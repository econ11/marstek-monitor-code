-- v3: battery capacity estimates (core/health.FindDischarges), one per deep
-- discharge run, keyed by its first sample. They outlive the raw samples they
-- come from (Compact deletes those after 30 days); with a retention set, Prune
-- deletes those older than it, except the newest local month that has any
-- (the battery health is read from it). Times are
-- unix ms UTC; wh = the capacity the run implies, dsoc = its SOC drop
-- (points). Filled by the monitor after start, not here: the migration only
-- creates an empty table and is instant on any database.
CREATE TABLE IF NOT EXISTS capacity_estimates (
  station_id TEXT NOT NULL,
  start      INTEGER NOT NULL,
  "end"      INTEGER NOT NULL,
  wh         REAL NOT NULL,
  dsoc       REAL NOT NULL,
  PRIMARY KEY (station_id, start)
) WITHOUT ROWID;
