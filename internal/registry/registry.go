package registry

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/silentmap/silentmap/internal/bus"
	_ "modernc.org/sqlite"
)

type Device struct {
	MAC       string
	IP        string
	Hostname  string
	Vendor    string
	Label     string
	Category  string
	Priority  bool
	Online    bool
	FirstSeen time.Time
	LastSeen  time.Time
}

type Registry struct {
	db             *sql.DB
	oui            *ouiDB
	b              *bus.Bus
	offlineTimeout time.Duration
	mu             sync.Mutex
}

func New(db *sql.DB, b *bus.Bus, offlineTimeout time.Duration) (*Registry, error) {
	if err := migrate(db); err != nil {
		return nil, err
	}
	r := &Registry{
		db:             db,
		oui:            newOUIDB(db),
		b:              b,
		offlineTimeout: offlineTimeout,
	}
	b.Subscribe(bus.EventDeviceSeen, r.handleSeen)
	return r, nil
}

func (r *Registry) handleSeen(e bus.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	mac := normalizeMac(e.MAC)
	if mac == "" {
		return
	}

	existing, err := r.get(mac)
	now := time.Now()

	if err == sql.ErrNoRows {
		// Brand new device
		vendor := r.oui.Lookup(mac)
		hostname, _ := e.Meta["hostname"].(string)
		dev := Device{
			MAC:       mac,
			IP:        e.IP,
			Hostname:  hostname,
			Vendor:    vendor,
			Online:    true,
			FirstSeen: now,
			LastSeen:  now,
		}
		if err := r.insert(dev); err != nil {
			slog.Error("registry: insert failed", "mac", mac, "err", err)
			return
		}
		slog.Info("new device", "mac", mac, "ip", e.IP, "vendor", vendor)
		r.b.Publish(bus.NewEvent(bus.EventDeviceNew, mac, e.IP, "registry", map[string]any{
			"vendor":   vendor,
			"hostname": hostname,
		}))
		return
	}
	if err != nil {
		slog.Error("registry: get failed", "mac", mac, "err", err)
		return
	}

	wasOffline := !existing.Online
	updates := map[string]any{"last_seen": now, "online": true, "ip": e.IP}

	if hostname, ok := e.Meta["hostname"].(string); ok && hostname != "" && existing.Hostname == "" {
		updates["hostname"] = hostname
	}

	if err := r.update(mac, updates); err != nil {
		slog.Error("registry: update failed", "mac", mac, "err", err)
		return
	}

	if wasOffline {
		slog.Info("device back", "mac", mac, "ip", e.IP)
		r.b.Publish(bus.NewEvent(bus.EventDeviceBack, mac, e.IP, "registry", map[string]any{
			"label":  existing.Label,
			"vendor": existing.Vendor,
		}))
	}
}

// RunOfflineChecker periodically marks devices as offline when they haven't
// been seen within the configured timeout.
func (r *Registry) RunOfflineChecker(ctx context.Context) {
	ticker := time.NewTicker(r.offlineTimeout / 3)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.checkOffline()
		}
	}
}

func (r *Registry) checkOffline() {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := time.Now().Add(-r.offlineTimeout)
	rows, err := r.db.Query(
		`SELECT mac, ip, label, priority FROM devices WHERE online = 1 AND last_seen < ?`,
		cutoff,
	)
	if err != nil {
		slog.Error("registry: offline check query failed", "err", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var mac, ip, label string
		var priority bool
		if err := rows.Scan(&mac, &ip, &label, &priority); err != nil {
			continue
		}
		if _, err := r.db.Exec(`UPDATE devices SET online = 0 WHERE mac = ?`, mac); err != nil {
			continue
		}
		slog.Info("device lost", "mac", mac, "ip", ip)
		r.b.Publish(bus.NewEvent(bus.EventDeviceLost, mac, ip, "registry", map[string]any{
			"label":    label,
			"priority": priority,
		}))
	}
}

// SetLabel sets a human-readable label for a device.
func (r *Registry) SetLabel(mac, label string) error {
	_, err := r.db.Exec(`UPDATE devices SET label = ? WHERE mac = ?`, label, normalizeMac(mac))
	return err
}

// SetPriority marks a device as priority (triggers critical alerts when offline).
func (r *Registry) SetPriority(mac string, priority bool) error {
	_, err := r.db.Exec(`UPDATE devices SET priority = ? WHERE mac = ?`, priority, normalizeMac(mac))
	return err
}

// List returns all known devices, online first.
func (r *Registry) List() ([]Device, error) {
	rows, err := r.db.Query(`
		SELECT mac, ip, hostname, vendor, label, category, priority, online, first_seen, last_seen
		FROM devices
		ORDER BY online DESC, last_seen DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []Device
	for rows.Next() {
		var d Device
		err := rows.Scan(&d.MAC, &d.IP, &d.Hostname, &d.Vendor, &d.Label,
			&d.Category, &d.Priority, &d.Online, &d.FirstSeen, &d.LastSeen)
		if err != nil {
			continue
		}
		devices = append(devices, d)
	}
	return devices, nil
}

// Get returns a single device by MAC.
func (r *Registry) Get(mac string) (*Device, error) {
	d, err := r.get(normalizeMac(mac))
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// OnlineCount returns the number of currently online devices.
func (r *Registry) OnlineCount() (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM devices WHERE online = 1`).Scan(&n)
	return n, err
}

func (r *Registry) get(mac string) (Device, error) {
	var d Device
	err := r.db.QueryRow(`
		SELECT mac, ip, hostname, vendor, label, category, priority, online, first_seen, last_seen
		FROM devices WHERE mac = ?`, mac).
		Scan(&d.MAC, &d.IP, &d.Hostname, &d.Vendor, &d.Label,
			&d.Category, &d.Priority, &d.Online, &d.FirstSeen, &d.LastSeen)
	return d, err
}

func (r *Registry) insert(d Device) error {
	_, err := r.db.Exec(`
		INSERT INTO devices(mac, ip, hostname, vendor, label, category, priority, online, first_seen, last_seen)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.MAC, d.IP, d.Hostname, d.Vendor, d.Label, d.Category, d.Priority, d.Online, d.FirstSeen, d.LastSeen,
	)
	return err
}

func (r *Registry) update(mac string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	setClauses := make([]string, 0, len(fields))
	args := make([]any, 0, len(fields)+1)
	for k, v := range fields {
		setClauses = append(setClauses, k+" = ?")
		args = append(args, v)
	}
	args = append(args, mac)
	_, err := r.db.Exec(
		"UPDATE devices SET "+strings.Join(setClauses, ", ")+" WHERE mac = ?",
		args...,
	)
	return err
}

func normalizeMac(mac string) string {
	mac = strings.ToUpper(strings.TrimSpace(mac))
	// Accept both AA:BB:CC:DD:EE:FF and AABBCCDDEEFF
	if len(mac) == 12 {
		return mac[0:2] + ":" + mac[2:4] + ":" + mac[4:6] + ":" + mac[6:8] + ":" + mac[8:10] + ":" + mac[10:12]
	}
	return mac
}
