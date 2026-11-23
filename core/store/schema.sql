CREATE TABLE stations (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL DEFAULT '',
  model      TEXT NOT NULL DEFAULT '',
  ble_mac    TEXT NOT NULL DEFAULT '',
  wifi_mac   TEXT NOT NULL DEFAULT '',
  ip         TEXT NOT NULL DEFAULT '',
  firmware   INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE INDEX stations_ble_mac ON stations(ble_mac);

-- Times are unix milliseconds UTC; NULL = unknown (never 0).
CREATE TABLE samples (
  station_id TEXT NOT NULL,
  at         INTEGER NOT NULL,
  source     TEXT NOT NULL,
  soc        REAL,
  battery_w  REAL,
  ongrid_w   REAL,
  offgrid_w  REAL,
  pv_w       REAL,
  grid       INTEGER NOT NULL,
  raw        TEXT,
  PRIMARY KEY (station_id, at)
) WITHOUT ROWID;

CREATE TABLE samples_minute (
  station_id   TEXT NOT NULL,
  minute       INTEGER NOT NULL,
  soc_avg      REAL,
  soc_min      REAL,
  soc_max      REAL,
  bat_w_avg    REAL,
  grid_off_sec INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (station_id, minute)
) WITHOUT ROWID;

CREATE TABLE outages (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  station_id TEXT NOT NULL,
  start      INTEGER NOT NULL,
  "end"      INTEGER,
  soc_start  REAL,
  soc_end    REAL,
  died_at    INTEGER,
  partial    INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX outages_one_open ON outages(station_id) WHERE "end" IS NULL;
CREATE INDEX outages_range ON outages(station_id, start);

CREATE TABLE gaps (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  station_id TEXT NOT NULL,
  start      INTEGER NOT NULL,
  "end"      INTEGER,
  reason     TEXT NOT NULL
);
CREATE INDEX gaps_range ON gaps(station_id, start);

CREATE TABLE events (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  station_id TEXT NOT NULL,
  at         INTEGER NOT NULL,
  level      TEXT NOT NULL,
  code       TEXT NOT NULL,
  text       TEXT NOT NULL,
  source     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX events_at ON events(station_id, at);

CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
