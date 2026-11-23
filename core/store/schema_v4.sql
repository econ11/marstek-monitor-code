-- v4: Bat.GetStatus readings. The table is no longer written or read (owner decision 2026-11-23); Prune deletes rows past the retention.
CREATE TABLE IF NOT EXISTS bat_status (
  station_id   TEXT NOT NULL,
  at           INTEGER NOT NULL,
  soc          REAL,
  temp_c       REAL,
  remaining_wh REAL,
  rated_wh     REAL,
  chg_ok       INTEGER,
  dis_ok       INTEGER,
  PRIMARY KEY (station_id, at)
) WITHOUT ROWID;
