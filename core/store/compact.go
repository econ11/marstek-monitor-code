package store

import (
	"database/sql"
	"github.com/econ11/marstek-monitor-code/core/model"
	"time"
)

const (
	rawKeep     = 30 * 24 * time.Hour // raw samples older than this are aggregated per minute
	rawJSONKeep = 7 * 24 * time.Hour  // Raw JSON older than this is nulled
	// maxSampleSpanMS caps how long one sample's grid state is assumed to hold,
	// so holes in the data are not counted as grid-off time. It is
	// core/energy.MaxStep (that package imports this one): samples come every
	// 20 s in Calm, every 5 s otherwise, and a longer step is a hole.
	maxSampleSpanMS = 60000
	// lastSampleSpanMS is assumed for a sample with no successor (one poll period).
	lastSampleSpanMS = 5000
)

// Compact aggregates raw samples older than 30 days into samples_minute and
// deletes them, and nulls Raw JSON older than 7 days. It is idempotent.
func (s *Store) Compact(now time.Time) error {
	// Align to a minute boundary so a minute is never split between raw and aggregate.
	cutoff := ms(now.Add(-rawKeep).Truncate(time.Minute))
	rawCutoff := ms(now.Add(-rawJSONKeep))

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE samples SET raw=NULL WHERE raw IS NOT NULL AND at<?`, rawCutoff); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR REPLACE INTO samples_minute(station_id,minute,soc_avg,soc_min,soc_max,bat_w_avg,grid_off_sec)
SELECT station_id, (at/60000)*60000, AVG(soc), MIN(soc), MAX(soc), AVG(battery_w),
       CAST(ROUND(SUM(CASE WHEN grid=? THEN span ELSE 0 END)/1000.0) AS INTEGER)
FROM (
  SELECT station_id, at, soc, battery_w, grid,
         MIN(COALESCE(LEAD(at) OVER (PARTITION BY station_id ORDER BY at) - at, ?), ?) AS span
  FROM samples WHERE at<?
)
GROUP BY station_id, at/60000`, int(model.GridAbsent), lastSampleSpanMS, maxSampleSpanMS, cutoff); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM samples WHERE at<?`, cutoff); err != nil {
		return err
	}
	return tx.Commit()
}

// MinuteSeries returns aggregated per-minute points with from <= At < to, ascending.
func (s *Store) MinuteSeries(stationID string, from, to time.Time) ([]MinutePoint, error) {
	rows, err := s.db.Query(`SELECT minute,soc_avg,soc_min,soc_max,bat_w_avg,grid_off_sec FROM samples_minute
WHERE station_id=? AND minute>=? AND minute<? ORDER BY minute`, stationID, ms(from), ms(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MinutePoint
	for rows.Next() {
		var p MinutePoint
		var m int64
		var a, lo, hi, w sql.NullFloat64
		if err := rows.Scan(&m, &a, &lo, &hi, &w, &p.GridOffSec); err != nil {
			return nil, err
		}
		p.At, p.SOCAvg, p.SOCMin, p.SOCMax, p.BatWAvg = fromMS(m), fltPtr(a), fltPtr(lo), fltPtr(hi), fltPtr(w)
		out = append(out, p)
	}
	return out, rows.Err()
}
