package registry

import "database/sql"

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS devices (
			mac        TEXT PRIMARY KEY,
			ip         TEXT NOT NULL DEFAULT '',
			hostname   TEXT NOT NULL DEFAULT '',
			vendor     TEXT NOT NULL DEFAULT '',
			label      TEXT NOT NULL DEFAULT '',
			category   TEXT NOT NULL DEFAULT '',
			priority   INTEGER NOT NULL DEFAULT 0,
			online     INTEGER NOT NULL DEFAULT 0,
			first_seen DATETIME NOT NULL,
			last_seen  DATETIME NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_devices_online ON devices(online);
		CREATE INDEX IF NOT EXISTS idx_devices_last_seen ON devices(last_seen);

		CREATE TABLE IF NOT EXISTS events (
			id         TEXT PRIMARY KEY,
			type       TEXT NOT NULL,
			mac        TEXT NOT NULL DEFAULT '',
			ip         TEXT NOT NULL DEFAULT '',
			source     TEXT NOT NULL DEFAULT '',
			meta       TEXT NOT NULL DEFAULT '{}',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_events_mac ON events(mac);
		CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
		CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at);

		CREATE TABLE IF NOT EXISTS alerts (
			id         TEXT PRIMARY KEY,
			type       TEXT NOT NULL,
			severity   TEXT NOT NULL,
			title      TEXT NOT NULL,
			summary    TEXT NOT NULL,
			mac        TEXT NOT NULL DEFAULT '',
			fired_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_alerts_fired_at ON alerts(fired_at);
	`)
	if err != nil {
		return err
	}
	return createOUITable(db)
}
