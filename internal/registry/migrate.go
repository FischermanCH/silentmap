package registry

import "database/sql"

func migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS devices (
			mac           TEXT PRIMARY KEY,
			ip            TEXT NOT NULL DEFAULT '',
			hostname      TEXT NOT NULL DEFAULT '',
			hostname_auto TEXT NOT NULL DEFAULT '',
			vendor        TEXT NOT NULL DEFAULT '',
			label         TEXT NOT NULL DEFAULT '',
			category      TEXT NOT NULL DEFAULT '',
			services      TEXT NOT NULL DEFAULT '[]',
			priority      INTEGER NOT NULL DEFAULT 0,
			online        INTEGER NOT NULL DEFAULT 0,
			first_seen    DATETIME NOT NULL,
			last_seen     DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_devices_online    ON devices(online)`,
		`CREATE INDEX IF NOT EXISTS idx_devices_last_seen ON devices(last_seen)`,
		`CREATE TABLE IF NOT EXISTS device_events (
			id         TEXT PRIMARY KEY,
			mac        TEXT NOT NULL,
			type       TEXT NOT NULL,
			ip         TEXT NOT NULL DEFAULT '',
			source     TEXT NOT NULL DEFAULT '',
			note       TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_devevents_mac        ON device_events(mac)`,
		`CREATE INDEX IF NOT EXISTS idx_devevents_created_at ON device_events(created_at)`,
		`CREATE TABLE IF NOT EXISTS alerts (
			id         TEXT PRIMARY KEY,
			type       TEXT NOT NULL,
			severity   TEXT NOT NULL,
			title      TEXT NOT NULL,
			summary    TEXT NOT NULL,
			mac        TEXT NOT NULL DEFAULT '',
			fired_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_fired_at ON alerts(fired_at)`,
		`CREATE TABLE IF NOT EXISTS device_connections (
			id         TEXT PRIMARY KEY,
			mac_a      TEXT NOT NULL,
			mac_b      TEXT NOT NULL,
			type       TEXT NOT NULL DEFAULT 'physical',
			label      TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_conn_mac_a ON device_connections(mac_a)`,
		`CREATE INDEX IF NOT EXISTS idx_conn_mac_b ON device_connections(mac_b)`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	// Add parent_mac column to existing databases (ignore "duplicate column" error).
	db.Exec(`ALTER TABLE devices ADD COLUMN parent_mac TEXT NOT NULL DEFAULT ''`)
	// approved=1 default so existing devices aren't flagged NEW after migration.
	db.Exec(`ALTER TABLE devices ADD COLUMN approved INTEGER NOT NULL DEFAULT 1`)
	db.Exec(`CREATE TABLE IF NOT EXISTS device_groups (
		id    TEXT PRIMARY KEY,
		name  TEXT NOT NULL,
		color TEXT NOT NULL DEFAULT '#888888'
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS device_group_members (
		group_id TEXT NOT NULL,
		mac      TEXT NOT NULL,
		PRIMARY KEY (group_id, mac)
	)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_grp_members_mac      ON device_group_members(mac)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_grp_members_group_id ON device_group_members(group_id)`)

	// Multi-parent table (replaces single parent_mac for topology).
	db.Exec(`CREATE TABLE IF NOT EXISTS device_parents (
		mac        TEXT NOT NULL,
		parent_mac TEXT NOT NULL,
		PRIMARY KEY (mac, parent_mac)
	)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_parents_mac ON device_parents(mac)`)
	db.Exec(`ALTER TABLE devices ADD COLUMN os_info TEXT NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE devices ADD COLUMN force_ping INTEGER NOT NULL DEFAULT 0`)
	return createOUITable(db)
}
