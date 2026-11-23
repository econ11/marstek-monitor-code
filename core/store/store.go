// Package store persists stations, samples, outages, gaps and settings in SQLite.
//
// Times are stored as INTEGER unix milliseconds (UTC); unknown values as NULL.
package store

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/econ11/marstek-monitor-code/core/model"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaV1 string

//go:embed schema_v2.sql
var schemaV2 string

//go:embed schema_v3.sql
var schemaV3 string

//go:embed schema_v4.sql
var schemaV4 string

//go:embed schema_v5.sql
var schemaV5 string

// migrations[i] upgrades user_version i to i+1, each in its own transaction.
var migrations = []string{schemaV1, schemaV2, schemaV3, schemaV4, schemaV5}

// ErrAlreadyClosed: CloseOutage / CloseGap on a row that already has an end
// (the first end stays).
var ErrAlreadyClosed = errors.New("store: already closed")

type Station struct {
	ID, Name, Model, BLEMac, WifiMac, IP string
	Firmware                             int
	CreatedAt                            time.Time
}

type Outage struct {
	ID               int64
	StationID        string
	Start            time.Time
	End              *time.Time
	SOCStart, SOCEnd *float64
	DiedAt           *time.Time
	Partial          bool
}

type GapReason string

const (
	GapAppNotRunning GapReason = "app_not_running"
	GapPCSleep       GapReason = "pc_sleep"
	GapNoLink        GapReason = "no_link"
	GapLikelyDead    GapReason = "likely_dead"
	GapAppUpdate     GapReason = "app_update"
)

type Gap struct {
	ID        int64
	StationID string
	Start     time.Time
	End       *time.Time
	Reason    GapReason
}

type Event struct {
	ID        int64
	StationID string
	At        time.Time
	Level     string // info|warn|error
	Code      string
	Text      string
	Source    string
}

type MinutePoint struct {
	At                              time.Time
	SOCAvg, SOCMin, SOCMax, BatWAvg *float64
	GridOffSec                      int
}

type Store struct{ db *sql.DB }

// Open opens (creating if needed) the database at path and applies migrations.
func Open(path string) (*Store, error) {
	// synchronous=FULL: in WAL mode NORMAL does not fsync the WAL on commit, so an
	// abrupt power cut could drop the latest commits (outage/gap/died marks).
	// The write rate is one row per few seconds, so FULL costs nothing noticeable.
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)")
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	// One connection: serialises writers, avoids SQLITE_BUSY between our own goroutines.
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	var v int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return fmt.Errorf("store: read user_version: %w", err)
	}
	if v > len(migrations) {
		return fmt.Errorf("store: database version %d is newer than supported %d", v, len(migrations))
	}
	for ; v < len(migrations); v++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[v]); err != nil {
			tx.Rollback()
			return fmt.Errorf("store: migrate to v%d: %w", v+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, v+1)); err != nil {
			tx.Rollback()
			return fmt.Errorf("store: set user_version: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func ms(t time.Time) int64 { return t.UnixMilli() }

func fromMS(v int64) time.Time { return time.UnixMilli(v).UTC() }

func msPtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UnixMilli()
}

func timePtr(n sql.NullInt64) *time.Time {
	if !n.Valid {
		return nil
	}
	t := fromMS(n.Int64)
	return &t
}

func fltPtr(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	v := n.Float64
	return &v
}

func fltArg(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

// --- stations ---

func (s *Store) UpsertStation(st Station) error {
	_, err := s.db.Exec(`INSERT INTO stations(id,name,model,ble_mac,wifi_mac,ip,firmware,created_at)
VALUES(?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET name=excluded.name, model=excluded.model, ble_mac=excluded.ble_mac,
  wifi_mac=excluded.wifi_mac, ip=excluded.ip, firmware=excluded.firmware`,
		st.ID, st.Name, st.Model, st.BLEMac, st.WifiMac, st.IP, st.Firmware, ms(st.CreatedAt))
	return err
}

// RenameStation sets the station's display name; sql.ErrNoRows if id is unknown.
func (s *Store) RenameStation(id, name string) error {
	return s.mustUpdate(`UPDATE stations SET name=? WHERE id=?`, name, id)
}

const stationCols = `id,name,model,ble_mac,wifi_mac,ip,firmware,created_at`

func scanStation(sc interface{ Scan(...any) error }) (Station, error) {
	var st Station
	var created int64
	err := sc.Scan(&st.ID, &st.Name, &st.Model, &st.BLEMac, &st.WifiMac, &st.IP, &st.Firmware, &created)
	st.CreatedAt = fromMS(created)
	return st, err
}

func (s *Store) StationByBLEMac(mac string) (Station, bool, error) {
	st, err := scanStation(s.db.QueryRow(`SELECT `+stationCols+` FROM stations WHERE ble_mac=? LIMIT 1`, mac))
	if errors.Is(err, sql.ErrNoRows) {
		return Station{}, false, nil
	}
	return st, err == nil, err
}

func (s *Store) Stations() ([]Station, error) {
	rows, err := s.db.Query(`SELECT ` + stationCols + ` FROM stations ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Station
	for rows.Next() {
		st, err := scanStation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// --- samples ---

func (s *Store) InsertSample(x model.Sample) error {
	var raw any
	if len(x.Raw) > 0 {
		raw = string(x.Raw)
	}
	_, err := s.db.Exec(`INSERT OR REPLACE INTO samples(station_id,at,source,soc,battery_w,ongrid_w,offgrid_w,pv_w,grid,raw)
VALUES(?,?,?,?,?,?,?,?,?,?)`,
		x.StationID, ms(x.At), string(x.Source), fltArg(x.SOC), fltArg(x.BatteryW),
		fltArg(x.OngridW), fltArg(x.OffgridW), fltArg(x.PVW), int(x.Grid), raw)
	return err
}

const sampleCols = `station_id,at,source,soc,battery_w,ongrid_w,offgrid_w,pv_w,grid,raw`

func scanSample(sc interface{ Scan(...any) error }) (model.Sample, error) {
	var x model.Sample
	var at int64
	var src string
	var soc, bat, on, off, pv sql.NullFloat64
	var grid int
	var raw sql.NullString
	if err := sc.Scan(&x.StationID, &at, &src, &soc, &bat, &on, &off, &pv, &grid, &raw); err != nil {
		return model.Sample{}, err
	}
	x.At, x.Source = fromMS(at), model.Source(src)
	x.SOC, x.BatteryW, x.OngridW, x.OffgridW, x.PVW = fltPtr(soc), fltPtr(bat), fltPtr(on), fltPtr(off), fltPtr(pv)
	x.Grid = model.GridSignal(grid)
	if raw.Valid {
		x.Raw = []byte(raw.String)
	}
	return x, nil
}

// Samples returns raw samples with from <= At < to, ascending.
func (s *Store) Samples(stationID string, from, to time.Time) ([]model.Sample, error) {
	rows, err := s.db.Query(`SELECT `+sampleCols+` FROM samples WHERE station_id=? AND at>=? AND at<? ORDER BY at`,
		stationID, ms(from), ms(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Sample
	for rows.Next() {
		x, err := scanSample(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) LastSample(stationID string) (model.Sample, bool, error) {
	x, err := scanSample(s.db.QueryRow(`SELECT `+sampleCols+` FROM samples WHERE station_id=? ORDER BY at DESC LIMIT 1`, stationID))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Sample{}, false, nil
	}
	return x, err == nil, err
}

// ObservedSince returns the earliest time we hold data for the station (raw or aggregated).
func (s *Store) ObservedSince(stationID string) (time.Time, bool, error) {
	var n sql.NullInt64
	err := s.db.QueryRow(`SELECT MIN(t) FROM (
  SELECT MIN(at) AS t FROM samples WHERE station_id=?
  UNION ALL SELECT MIN(minute) FROM samples_minute WHERE station_id=?)`, stationID, stationID).Scan(&n)
	if err != nil || !n.Valid {
		return time.Time{}, false, err
	}
	return fromMS(n.Int64), true, nil
}

// MinSOC returns the lowest charge in [from, to): raw samples and the
// soc_min of aggregated minutes that overlap the range. nil without data.
func (s *Store) MinSOC(stationID string, from, to time.Time) (*float64, error) {
	var n sql.NullFloat64
	err := s.db.QueryRow(`SELECT MIN(v) FROM (
  SELECT MIN(soc) AS v FROM samples WHERE station_id=? AND at>=? AND at<?
  UNION ALL SELECT MIN(soc_min) FROM samples_minute WHERE station_id=? AND minute>? AND minute<?)`,
		stationID, ms(from), ms(to), stationID, ms(from)-60000, ms(to)).Scan(&n)
	if err != nil {
		return nil, err
	}
	return fltPtr(n), nil
}

// SOCBucket is the average SOC of the bucket starting At (UTC); nil when no
// value in it is known.
type SOCBucket struct {
	At  time.Time
	SOC *float64
}

// SOCBuckets averages the SOC per bucket over the raw samples and the
// aggregated minutes (one value each: soc_avg) with from <= at < to,
// ascending. It reads no raw JSON and returns one row per bucket.
func (s *Store) SOCBuckets(stationID string, from, to time.Time, bucket time.Duration) ([]SOCBucket, error) {
	b := bucket.Milliseconds()
	if b <= 0 {
		return nil, fmt.Errorf("store: SOC bucket %v", bucket)
	}
	rows, err := s.db.Query(`SELECT (t/?)*? AS k, AVG(v) FROM (
  SELECT at AS t, soc AS v FROM samples WHERE station_id=? AND at>=? AND at<?
  UNION ALL SELECT minute, soc_avg FROM samples_minute WHERE station_id=? AND minute>=? AND minute<?)
GROUP BY k ORDER BY k`, b, b, stationID, ms(from), ms(to), stationID, ms(from), ms(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SOCBucket
	for rows.Next() {
		var k int64
		var v sql.NullFloat64
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out = append(out, SOCBucket{At: fromMS(k), SOC: fltPtr(v)})
	}
	return out, rows.Err()
}

// --- outages ---

const outageCols = `id,station_id,start,"end",soc_start,soc_end,died_at,partial`

func scanOutage(sc interface{ Scan(...any) error }) (Outage, error) {
	var o Outage
	var start int64
	var end, died sql.NullInt64
	var s0, s1 sql.NullFloat64
	var partial int
	if err := sc.Scan(&o.ID, &o.StationID, &start, &end, &s0, &s1, &died, &partial); err != nil {
		return Outage{}, err
	}
	o.Start, o.End, o.DiedAt = fromMS(start), timePtr(end), timePtr(died)
	o.SOCStart, o.SOCEnd, o.Partial = fltPtr(s0), fltPtr(s1), partial != 0
	return o, nil
}

// OpenOutage inserts an outage; it fails if the station already has an open one.
func (s *Store) OpenOutage(o Outage) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO outages(station_id,start,"end",soc_start,soc_end,died_at,partial) VALUES(?,?,?,?,?,?,?)`,
		o.StationID, ms(o.Start), msPtr(o.End), fltArg(o.SOCStart), fltArg(o.SOCEnd), msPtr(o.DiedAt), o.Partial)
	if err != nil {
		return 0, fmt.Errorf("store: open outage: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) CurrentOutage(stationID string) (Outage, bool, error) {
	o, err := scanOutage(s.db.QueryRow(`SELECT `+outageCols+` FROM outages WHERE station_id=? AND "end" IS NULL`, stationID))
	if errors.Is(err, sql.ErrNoRows) {
		return Outage{}, false, nil
	}
	return o, err == nil, err
}

// CloseOutage sets the end of an open outage: ErrAlreadyClosed if it has
// one, sql.ErrNoRows if id is unknown.
func (s *Store) CloseOutage(id int64, end time.Time, socEnd *float64) error {
	return s.closeOnce("outages", id, `UPDATE outages SET "end"=?, soc_end=? WHERE id=? AND "end" IS NULL`, ms(end), fltArg(socEnd), id)
}

// closeOnce runs q, an update matching only the open row id of table, and
// tells a row already closed (ErrAlreadyClosed) from an unknown id
// (sql.ErrNoRows).
func (s *Store) closeOnce(table string, id int64, q string, args ...any) error {
	err := s.mustUpdate(q, args...)
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var one int
	if err := s.db.QueryRow(`SELECT 1 FROM `+table+` WHERE id=?`, id).Scan(&one); err != nil {
		return err // sql.ErrNoRows: unknown id
	}
	return ErrAlreadyClosed
}

func (s *Store) MarkDied(id int64, at time.Time) error {
	return s.mustUpdate(`UPDATE outages SET died_at=? WHERE id=?`, ms(at), id)
}

func (s *Store) mustUpdate(q string, args ...any) error {
	res, err := s.db.Exec(q, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Outages returns outages intersecting [from, to); open ones extend to infinity.
func (s *Store) Outages(stationID string, from, to time.Time) ([]Outage, error) {
	rows, err := s.db.Query(`SELECT `+outageCols+` FROM outages
WHERE station_id=? AND start<? AND ("end" IS NULL OR "end">?) ORDER BY start`, stationID, ms(to), ms(from))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Outage
	for rows.Next() {
		o, err := scanOutage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// --- gaps ---

// OpenGap inserts a gap; it fails if the station already has an open one
// (schema_v5.sql).
func (s *Store) OpenGap(g Gap) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO gaps(station_id,start,"end",reason) VALUES(?,?,?,?)`,
		g.StationID, ms(g.Start), msPtr(g.End), string(g.Reason))
	if err != nil {
		return 0, fmt.Errorf("store: open gap: %w", err)
	}
	return res.LastInsertId()
}

// CurrentGap returns the station's open gap, if any.
func (s *Store) CurrentGap(stationID string) (Gap, bool, error) {
	g := Gap{StationID: stationID}
	var start int64
	var reason string
	err := s.db.QueryRow(`SELECT id,start,reason FROM gaps WHERE station_id=? AND "end" IS NULL`, stationID).
		Scan(&g.ID, &start, &reason)
	if errors.Is(err, sql.ErrNoRows) {
		return Gap{}, false, nil
	}
	if err != nil {
		return Gap{}, false, err
	}
	g.Start, g.Reason = fromMS(start), GapReason(reason)
	return g, true, nil
}

// CloseGap sets the end of an open gap: ErrAlreadyClosed if it has one,
// sql.ErrNoRows if id is unknown.
func (s *Store) CloseGap(id int64, end time.Time) error {
	return s.closeOnce("gaps", id, `UPDATE gaps SET "end"=? WHERE id=? AND "end" IS NULL`, ms(end), id)
}

// SetGapReason updates a gap's reason (e.g. no_link becomes likely_dead once
// the station probably ran out), so one absence keeps one gap.
func (s *Store) SetGapReason(id int64, r GapReason) error {
	return s.mustUpdate(`UPDATE gaps SET reason=? WHERE id=?`, string(r), id)
}

// Gaps returns gaps intersecting [from, to); open ones extend to infinity.
func (s *Store) Gaps(stationID string, from, to time.Time) ([]Gap, error) {
	rows, err := s.db.Query(`SELECT id,station_id,start,"end",reason FROM gaps
WHERE station_id=? AND start<? AND ("end" IS NULL OR "end">?) ORDER BY start`, stationID, ms(to), ms(from))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Gap
	for rows.Next() {
		var g Gap
		var start int64
		var end sql.NullInt64
		var reason string
		if err := rows.Scan(&g.ID, &g.StationID, &start, &end, &reason); err != nil {
			return nil, err
		}
		g.Start, g.End, g.Reason = fromMS(start), timePtr(end), GapReason(reason)
		out = append(out, g)
	}
	return out, rows.Err()
}

// --- events, settings ---

func (s *Store) AddEvent(e Event) error {
	_, err := s.db.Exec(`INSERT INTO events(station_id,at,level,code,text,source) VALUES(?,?,?,?,?,?)`,
		e.StationID, ms(e.At), e.Level, e.Code, e.Text, e.Source)
	return err
}

// Events returns the events of one code with from <= At < to, ascending.
func (s *Store) Events(stationID, code string, from, to time.Time) ([]Event, error) {
	rows, err := s.db.Query(`SELECT id,station_id,at,level,code,text,source FROM events
WHERE station_id=? AND code=? AND at>=? AND at<? ORDER BY at, id`, stationID, code, ms(from), ms(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var at int64
		if err := rows.Scan(&e.ID, &e.StationID, &at, &e.Level, &e.Code, &e.Text, &e.Source); err != nil {
			return nil, err
		}
		e.At = fromMS(at)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) GetSetting(key string) (string, bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return v, err == nil, err
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// --- energy ---

// EnergyMinute is the battery energy of one minute (core/energy.Rollup) or,
// from EnergyHours, of one hour. At is its start (UTC). PeakDisW is the
// highest discharge power (W, positive) and PeakAt the sample it was read at;
// nil without discharge above the deadband.
type EnergyMinute struct {
	At                                 time.Time
	ChgWh, DisWh, DisOutageWh, PVChgWh float64
	Covered                            time.Duration
	PeakDisW                           *float64
	PeakAt                             *time.Time
}

// UpsertEnergyMinutes writes the minutes in one transaction, replacing any
// already stored for the same minute.
func (s *Store) UpsertEnergyMinutes(stationID string, mins []EnergyMinute) error {
	if len(mins) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := tx.Prepare(`INSERT OR REPLACE INTO energy_minute(station_id,minute,chg_wh,dis_wh,dis_out_wh,pv_chg_wh,covered_ms,peak_dis_w,peak_at)
VALUES(?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, m := range mins {
		if _, err := st.Exec(stationID, ms(m.At), m.ChgWh, m.DisWh, m.DisOutageWh, m.PVChgWh,
			m.Covered.Milliseconds(), fltArg(m.PeakDisW), msPtr(m.PeakAt)); err != nil {
			return fmt.Errorf("store: energy minute: %w", err)
		}
	}
	return tx.Commit()
}

// EnergySamples returns the samples with from <= At < to, ascending, with
// only At, SOC, BatteryW and PVW filled: the energy rollup and the capacity
// estimates read weeks of them and must not load the raw JSON.
func (s *Store) EnergySamples(stationID string, from, to time.Time) ([]model.Sample, error) {
	rows, err := s.db.Query(`SELECT at,soc,battery_w,pv_w FROM samples WHERE station_id=? AND at>=? AND at<? ORDER BY at`,
		stationID, ms(from), ms(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Sample
	for rows.Next() {
		var at int64
		var soc, bat, pv sql.NullFloat64
		if err := rows.Scan(&at, &soc, &bat, &pv); err != nil {
			return nil, err
		}
		out = append(out, model.Sample{StationID: stationID, At: fromMS(at), SOC: fltPtr(soc), BatteryW: fltPtr(bat), PVW: fltPtr(pv)})
	}
	return out, rows.Err()
}

// PlainSamples returns the samples with from <= At < to, ascending, without
// the raw JSON (Raw nil): the CSV export reads days of them.
func (s *Store) PlainSamples(stationID string, from, to time.Time) ([]model.Sample, error) {
	rows, err := s.db.Query(`SELECT station_id,at,source,soc,battery_w,ongrid_w,offgrid_w,pv_w,grid,NULL FROM samples
WHERE station_id=? AND at>=? AND at<? ORDER BY at`, stationID, ms(from), ms(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Sample
	for rows.Next() {
		x, err := scanSample(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// LastEnergyMinute returns the start of the newest stored energy minute.
func (s *Store) LastEnergyMinute(stationID string) (time.Time, bool, error) {
	var n sql.NullInt64
	if err := s.db.QueryRow(`SELECT MAX(minute) FROM energy_minute WHERE station_id=?`, stationID).Scan(&n); err != nil || !n.Valid {
		return time.Time{}, false, err
	}
	return fromMS(n.Int64), true, nil
}

// FirstEnergyMinute is the oldest stored energy minute of the station.
func (s *Store) FirstEnergyMinute(stationID string) (time.Time, bool, error) {
	var n sql.NullInt64
	if err := s.db.QueryRow(`SELECT MIN(minute) FROM energy_minute WHERE station_id=?`, stationID).Scan(&n); err != nil || !n.Valid {
		return time.Time{}, false, err
	}
	return fromMS(n.Int64), true, nil
}

// EnergyHours sums the minutes with from <= minute < to by UTC hour,
// ascending; each hour keeps its highest discharge peak with its time.
// Grouping by UTC hour is right for zones offset by whole hours, Kyiv with
// its DST included: every local day is a whole number of these hours. In a
// zone offset by half an hour an hour would straddle local midnight and be
// counted in one day only; v1 leaves that as is.
func (s *Store) EnergyHours(stationID string, from, to time.Time) ([]EnergyMinute, error) {
	// SQLite: with a single MAX(), the bare column peak_at comes from the row
	// holding the maximum.
	rows, err := s.db.Query(`SELECT (minute/3600000)*3600000 AS h, SUM(chg_wh), SUM(dis_wh), SUM(dis_out_wh),
  SUM(pv_chg_wh), SUM(covered_ms), MAX(peak_dis_w), peak_at
FROM energy_minute WHERE station_id=? AND minute>=? AND minute<? GROUP BY h ORDER BY h`, stationID, ms(from), ms(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EnergyMinute
	for rows.Next() {
		var m EnergyMinute
		var h, covered int64
		var peak sql.NullFloat64
		var peakAt sql.NullInt64
		if err := rows.Scan(&h, &m.ChgWh, &m.DisWh, &m.DisOutageWh, &m.PVChgWh, &covered, &peak, &peakAt); err != nil {
			return nil, err
		}
		m.At, m.Covered = fromMS(h), time.Duration(covered)*time.Millisecond
		if peak.Valid {
			m.PeakDisW, m.PeakAt = fltPtr(peak), timePtr(peakAt)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SettingDischargedPrunedWh holds the discharged energy of the energy
// minutes Prune deleted, per station: a JSON object {station id: Wh}.
const SettingDischargedPrunedWh = "health.discharged_pruned_wh"

// prunedWh parses SettingDischargedPrunedWh (sql.NullString: unset = none).
// An unreadable value is an error, never a silent 0: the lifetime cycles
// must not drop.
func prunedWh(v sql.NullString) (map[string]float64, error) {
	m := map[string]float64{}
	if !v.Valid || v.String == "" {
		return m, nil
	}
	if err := json.Unmarshal([]byte(v.String), &m); err != nil {
		return nil, fmt.Errorf("store: %s: %w", SettingDischargedPrunedWh, err)
	}
	return m, nil
}

// TotalDischargedWh is the energy discharged over the station's lifetime: the
// stored minutes plus the ones Prune deleted (SettingDischargedPrunedWh); 0
// without any. One statement, so a Prune cannot fall between the two parts.
func (s *Store) TotalDischargedWh(stationID string) (float64, error) {
	var v float64
	var pruned sql.NullString
	err := s.db.QueryRow(`SELECT COALESCE(SUM(dis_wh),0), (SELECT value FROM settings WHERE key=?)
FROM energy_minute WHERE station_id=?`, SettingDischargedPrunedWh, stationID).Scan(&v, &pruned)
	if err != nil {
		return 0, err
	}
	m, err := prunedWh(pruned)
	if err != nil {
		return 0, err
	}
	return v + m[stationID], nil
}

// gapSlack = core/history.BoundarySlack (that package imports this one): a
// gap ending this close before an outage start makes the start approximate.
const gapSlack = 2 * time.Minute

// Prune deletes the station's data older than before (the retention
// setting), in one transaction:
//   - samples, samples_minute, energy_minute and events older than before;
//     the discharged energy of the deleted energy minutes is first added to
//     SettingDischargedPrunedWh, so TotalDischargedWh (the lifetime cycles)
//     never drops;
//   - outages and gaps only when closed with end < before: open rows are
//     never touched, however old their start; a closed gap that a kept
//     outage (open, or ending at or after before) still needs is kept too:
//     one ending inside it or at most gapSlack before its start
//     (core/history.Annotate reads them for the outage's bounds);
//   - capacity estimates that start before before, except those of the
//     newest local month that has any (the battery health is read from it);
//   - rows of the bat_status table (schema_v4.sql) older than before: the
//     app no longer calls Bat.GetStatus (owner decision 2026-11-23) and
//     neither writes nor reads them; old rows still follow the retention.
//
// Pruning again with the same bound deletes and adds nothing.
func (s *Store) Prune(stationID string, before time.Time) error {
	b := ms(before)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var dis float64
	var pruned sql.NullString
	if err := tx.QueryRow(`SELECT COALESCE(SUM(dis_wh),0), (SELECT value FROM settings WHERE key=?)
FROM energy_minute WHERE station_id=? AND minute<?`, SettingDischargedPrunedWh, stationID, b).Scan(&dis, &pruned); err != nil {
		return err
	}
	counter, err := prunedWh(pruned)
	if err != nil {
		return err
	}
	if dis != 0 {
		counter[stationID] += dis
		v, err := json.Marshal(counter)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
			SettingDischargedPrunedWh, string(v)); err != nil {
			return err
		}
	}

	estCut := b
	var newest sql.NullInt64
	if err := tx.QueryRow(`SELECT MAX(start) FROM capacity_estimates WHERE station_id=?`, stationID).Scan(&newest); err != nil {
		return err
	}
	if newest.Valid {
		l := fromMS(newest.Int64).In(time.Local)
		estCut = min(b, ms(time.Date(l.Year(), l.Month(), 1, 0, 0, 0, 0, time.Local)))
	}

	for _, d := range []struct {
		q   string
		arg int64
	}{
		{`DELETE FROM samples WHERE station_id=? AND at<?`, b},
		{`DELETE FROM samples_minute WHERE station_id=? AND minute<?`, b},
		{`DELETE FROM energy_minute WHERE station_id=? AND minute<?`, b},
		{`DELETE FROM events WHERE station_id=? AND at<?`, b},
		{`DELETE FROM outages WHERE station_id=? AND "end" IS NOT NULL AND "end"<?`, b},
		{`DELETE FROM capacity_estimates WHERE station_id=? AND start<?`, estCut},
		{`DELETE FROM bat_status WHERE station_id=? AND at<?`, b},
	} {
		if _, err := tx.Exec(d.q, stationID, d.arg); err != nil {
			return fmt.Errorf("store: prune: %w", err)
		}
	}
	// After the outages: the ones left are the kept ones.
	if _, err := tx.Exec(`DELETE FROM gaps WHERE station_id=? AND "end" IS NOT NULL AND "end"<? AND NOT EXISTS (
  SELECT 1 FROM outages o WHERE o.station_id=gaps.station_id AND (o."end" IS NULL OR o."end">=?) AND o.start<=gaps."end"+?)`,
		stationID, b, b, gapSlack.Milliseconds()); err != nil {
		return fmt.Errorf("store: prune gaps: %w", err)
	}
	return tx.Commit()
}

// --- battery health ---

// CapacityEstimate is one deep discharge run (core/health.FindDischarges):
// its first and last sample (UTC), the capacity it implies (Wh) and its SOC
// drop (points).
type CapacityEstimate struct {
	Start, End   time.Time
	Wh, DeltaSOC float64
}

// UpsertCapacityEstimates writes the estimates in one transaction; one with
// the start of a stored estimate replaces it.
func (s *Store) UpsertCapacityEstimates(stationID string, es []CapacityEstimate) error {
	if len(es) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := tx.Prepare(`INSERT OR REPLACE INTO capacity_estimates(station_id,start,"end",wh,dsoc) VALUES(?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, e := range es {
		if _, err := st.Exec(stationID, ms(e.Start), ms(e.End), e.Wh, e.DeltaSOC); err != nil {
			return fmt.Errorf("store: capacity estimate: %w", err)
		}
	}
	return tx.Commit()
}

// CapacityEstimates returns all the station's estimates, ascending by start.
func (s *Store) CapacityEstimates(stationID string) ([]CapacityEstimate, error) {
	rows, err := s.db.Query(`SELECT start,"end",wh,dsoc FROM capacity_estimates WHERE station_id=? ORDER BY start`, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CapacityEstimate
	for rows.Next() {
		var e CapacityEstimate
		var start, end int64
		if err := rows.Scan(&start, &end, &e.Wh, &e.DeltaSOC); err != nil {
			return nil, err
		}
		e.Start, e.End = fromMS(start), fromMS(end)
		out = append(out, e)
	}
	return out, rows.Err()
}

// FirstSampleAt is the time of the station's oldest raw sample.
func (s *Store) FirstSampleAt(stationID string) (time.Time, bool, error) {
	var n sql.NullInt64
	if err := s.db.QueryRow(`SELECT MIN(at) FROM samples WHERE station_id=?`, stationID).Scan(&n); err != nil || !n.Valid {
		return time.Time{}, false, err
	}
	return fromMS(n.Int64), true, nil
}
