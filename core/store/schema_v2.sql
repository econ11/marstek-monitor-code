-- v2: per-minute battery energy (core/energy.Rollup). Kept like outages: for
-- good when the retention is «always», else Prune deletes the rows older than
-- it (their discharged Wh first go to the pruned-Wh counter, so the cycle
-- total never drops). Times are unix ms UTC (minute = its start); NULL peak = no
-- discharge above the deadband in that minute. Rows are filled after start by
-- the monitor, not here: the migration itself is instant on any database.
CREATE TABLE IF NOT EXISTS energy_minute (
  station_id TEXT NOT NULL,
  minute     INTEGER NOT NULL,
  chg_wh     REAL NOT NULL,
  dis_wh     REAL NOT NULL,
  dis_out_wh REAL NOT NULL,
  pv_chg_wh  REAL NOT NULL,
  covered_ms INTEGER NOT NULL,
  peak_dis_w REAL,
  peak_at    INTEGER,
  PRIMARY KEY (station_id, minute)
) WITHOUT ROWID;
