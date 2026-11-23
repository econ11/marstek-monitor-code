-- v5: at most one open gap per station, as for outages (schema.sql
-- outages_one_open). A database from before may hold several: all but the
-- newest (by start, then id) are closed where that absence really ended, the
-- first measurement after its start (a raw sample or an aggregated minute of
-- the station), but no later than the newest one's start (which, still open,
-- covers the rest); without any measurement after it, at the newest one's
-- start. The newest stays open for the monitor to adopt. Instant on any
-- database: only open gaps are touched.
UPDATE gaps SET "end" = MIN(n.start, COALESCE((
  SELECT MIN(t) FROM (
    SELECT MIN(s.at) AS t FROM samples s WHERE s.station_id = gaps.station_id AND s.at > gaps.start
    UNION ALL
    SELECT MIN(m.minute) FROM samples_minute m WHERE m.station_id = gaps.station_id AND m.minute > gaps.start)
), n.start))
FROM (
  SELECT station_id, id, start FROM (
    SELECT station_id, id, start,
           ROW_NUMBER() OVER (PARTITION BY station_id ORDER BY start DESC, id DESC) AS rn
    FROM gaps WHERE "end" IS NULL)
  WHERE rn = 1
) AS n
WHERE gaps.station_id = n.station_id AND gaps."end" IS NULL AND gaps.id <> n.id;
CREATE UNIQUE INDEX gaps_one_open ON gaps(station_id) WHERE "end" IS NULL;
