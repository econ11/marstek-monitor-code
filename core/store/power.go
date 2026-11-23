package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// MaxPowerSpan bounds one PowerBuckets window: 31 local days, one of them
// 25 h long (the autumn DST change). The battery power chart never asks for
// more (core/energy.MaxPowerDays clips a longer period).
const MaxPowerSpan = 31*24*time.Hour + time.Hour

// ErrPowerSpan: the PowerBuckets window is longer than MaxPowerSpan.
var ErrPowerSpan = errors.New("store: power window longer than 31 days")

// PowerBucket is the battery power of the bucket starting At (UTC), W
// (+ charge, − discharge): nil when no value in it is known — never 0.
type PowerBucket struct {
	At time.Time
	W  *float64
}

// PowerBuckets returns the battery power per minute or per hour with
// from <= minute < to, ascending; buckets without data are left out (the
// caller breaks the line there). The window is bounded by MaxPowerSpan.
//
// A minute is the average of its raw samples (kept 30 days), or
// samples_minute.bat_w_avg for older data. An hour comes from the energy
// rollup instead (energy_minute, one row per covered minute, at most 44 640
// rows for 31 days rather than half a million raw samples): its net energy
// over its covered time, (chg_wh − dis_wh) / covered. Hours are UTC hours, as
// in EnergyHours; an hour with no covered time has no row.
func (s *Store) PowerBuckets(stationID string, from, to time.Time, bucket time.Duration) ([]PowerBucket, error) {
	if bucket != time.Minute && bucket != time.Hour {
		return nil, fmt.Errorf("store: power bucket %v", bucket)
	}
	if to.Before(from) {
		return nil, fmt.Errorf("store: power window %v..%v", from, to)
	}
	if to.Sub(from) > MaxPowerSpan {
		return nil, ErrPowerSpan
	}
	q := `SELECT minute, w FROM (
  SELECT (at/60000)*60000 AS minute, AVG(battery_w) AS w FROM samples WHERE station_id=? AND at>=? AND at<? GROUP BY minute
  UNION ALL SELECT minute, bat_w_avg FROM samples_minute WHERE station_id=? AND minute>=? AND minute<?)
ORDER BY minute`
	args := []any{stationID, ms(from), ms(to), stationID, ms(from), ms(to)}
	if bucket == time.Hour {
		q = `SELECT (minute/3600000)*3600000 AS h, (SUM(chg_wh)-SUM(dis_wh))*3600000.0/SUM(covered_ms) FROM energy_minute
WHERE station_id=? AND minute>=? AND minute<? GROUP BY h HAVING SUM(covered_ms)>0 ORDER BY h`
		args = args[:3]
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PowerBucket
	for rows.Next() {
		var k int64
		var w sql.NullFloat64
		if err := rows.Scan(&k, &w); err != nil {
			return nil, err
		}
		out = append(out, PowerBucket{At: fromMS(k), W: fltPtr(w)})
	}
	return out, rows.Err()
}

// PeakDischarge is the highest discharge peak (energy_minute.peak_dis_w) with
// from <= minute < to and its time; nil, nil without discharge.
func (s *Store) PeakDischarge(stationID string, from, to time.Time) (*float64, *time.Time, error) {
	// SQLite: with a single MAX(), the bare column peak_at comes from the row
	// holding the maximum.
	var w sql.NullFloat64
	var at sql.NullInt64
	err := s.db.QueryRow(`SELECT MAX(peak_dis_w), peak_at FROM energy_minute
WHERE station_id=? AND minute>=? AND minute<? AND peak_dis_w IS NOT NULL`, stationID, ms(from), ms(to)).Scan(&w, &at)
	if err != nil || !w.Valid {
		return nil, nil, err
	}
	return fltPtr(w), timePtr(at), nil
}
