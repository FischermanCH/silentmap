package registry

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/silentmap/silentmap/internal/bus"
	_ "modernc.org/sqlite"
)

// parseTime handles SQLite datetime strings in multiple formats.
func parseTime(s string) time.Time {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	}
	s = strings.TrimSuffix(s, "Z")
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}

// DeviceEvent is a single activity entry shown on the device detail page.
type DeviceEvent struct {
	ID        string
	MAC       string
	Type      string
	IP        string
	Source    string
	Note      string
	CreatedAt time.Time
}

// Device is the core model stored in SQLite.
type Device struct {
	MAC          string
	IP           string
	Hostname     string   // user-set override (editable)
	HostnameAuto string   // discovered via mDNS/DHCP
	Vendor       string
	Label        string
	Category     string
	Services     []string // mDNS service types, e.g. ["_airplay._tcp","_smb._tcp"]
	Priority     bool
	Online       bool
	FirstSeen    time.Time
	LastSeen     time.Time
}

// DisplayName returns the best available name for the device.
func (d Device) DisplayName() string {
	if d.Label != "" {
		return d.Label
	}
	if d.Hostname != "" {
		return d.Hostname
	}
	if d.HostnameAuto != "" {
		return d.HostnameAuto
	}
	return d.MAC
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
	// Sync: ARP-Bursts sollen sequenziell verarbeitet werden, nicht parallel
	b.SubscribeSync(bus.EventDeviceSeen, r.handleSeen)
	return r, nil
}

func (r *Registry) handleSeen(e bus.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// mDNS events may not have a MAC — match by IP
	mac := normalizeMac(e.MAC)
	if mac == "" {
		if e.IP == "" {
			return
		}
		mac = r.macByIP(e.IP)
		if mac == "" {
			return // unknown device, wait for ARP
		}
	}

	existing, err := r.get(mac)
	now := time.Now()

	if err == sql.ErrNoRows {
		vendor := r.oui.Lookup(mac)
		hostnameAuto, _ := e.Meta["hostname"].(string)
		svcs, _ := e.Meta["services"].([]string)
		dev := Device{
			MAC:          mac,
			IP:           e.IP,
			HostnameAuto: hostnameAuto,
			Vendor:       vendor,
			Services:     svcs,
			Online:       true,
			FirstSeen:    now,
			LastSeen:     now,
		}
		if err := r.insert(dev); err != nil {
			slog.Error("registry: insert failed", "mac", mac, "err", err)
			return
		}
		r.logEvent(mac, e.IP, "seen", e.Source, "Erstmals gesehen")
		slog.Info("new device", "mac", mac, "ip", e.IP, "vendor", vendor)
		r.b.Publish(bus.NewEvent(bus.EventDeviceNew, mac, e.IP, "registry", map[string]any{
			"vendor":   vendor,
			"hostname": hostnameAuto,
		}))
		return
	}
	if err != nil {
		slog.Error("registry: get failed", "mac", mac, "err", err)
		return
	}

	wasOffline := !existing.Online
	updates := map[string]any{"last_seen": now, "online": true}
	if e.IP != "" {
		updates["ip"] = e.IP
	}

	// Merge mDNS hostname (only if not user-overridden)
	if hostnameAuto, ok := e.Meta["hostname"].(string); ok && hostnameAuto != "" {
		if existing.HostnameAuto != hostnameAuto {
			updates["hostname_auto"] = hostnameAuto
			r.logEvent(mac, e.IP, "hostname", e.Source, hostnameAuto)
		}
	}

	// Merge services
	if svcs, ok := e.Meta["services"].([]string); ok && len(svcs) > 0 {
		merged := mergeServices(existing.Services, svcs)
		if len(merged) > len(existing.Services) {
			updates["services"] = marshalServices(merged)
			r.logEvent(mac, e.IP, "services", e.Source, strings.Join(svcs, ", "))
		}
	}

	if err := r.update(mac, updates); err != nil {
		slog.Error("registry: update failed", "mac", mac, "err", err)
		return
	}

	if wasOffline {
		r.logEvent(mac, e.IP, "online", e.Source, "Gerät wieder online")
		slog.Info("device back", "mac", mac, "ip", e.IP)
		r.b.Publish(bus.NewEvent(bus.EventDeviceBack, mac, e.IP, "registry", map[string]any{
			"label":  existing.Label,
			"vendor": existing.Vendor,
		}))
	}
}

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
		`SELECT mac, ip, label, priority FROM devices WHERE online = 1 AND last_seen < ?`, cutoff,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var mac, ip, label string
		var priority bool
		if err := rows.Scan(&mac, &ip, &label, &priority); err != nil {
			continue
		}
		r.db.Exec(`UPDATE devices SET online = 0 WHERE mac = ?`, mac)
		r.logEvent(mac, ip, "offline", "registry", "Nicht mehr erreichbar")
		slog.Info("device lost", "mac", mac, "ip", ip)
		r.b.Publish(bus.NewEvent(bus.EventDeviceLost, mac, ip, "registry", map[string]any{
			"label":    label,
			"priority": priority,
		}))
	}
}

// --- Public API ---

func (r *Registry) SetLabel(mac, label string) error {
	_, err := r.db.Exec(`UPDATE devices SET label = ? WHERE mac = ?`, label, normalizeMac(mac))
	if err == nil {
		r.logEvent(normalizeMac(mac), "", "label", "web", label)
	}
	return err
}

func (r *Registry) SetHostname(mac, hostname string) error {
	_, err := r.db.Exec(`UPDATE devices SET hostname = ? WHERE mac = ?`, hostname, normalizeMac(mac))
	if err == nil {
		r.logEvent(normalizeMac(mac), "", "hostname_manual", "web", hostname)
	}
	return err
}

func (r *Registry) SetPriority(mac string, priority bool) error {
	_, err := r.db.Exec(`UPDATE devices SET priority = ? WHERE mac = ?`, priority, normalizeMac(mac))
	return err
}

func (r *Registry) SetCategory(mac, category string) error {
	_, err := r.db.Exec(`UPDATE devices SET category = ? WHERE mac = ?`, category, normalizeMac(mac))
	return err
}

func (r *Registry) List() ([]Device, error) {
	rows, err := r.db.Query(`
		SELECT mac, ip, hostname, hostname_auto, vendor, label, category, services, priority, online, first_seen, last_seen
		FROM devices
		ORDER BY online DESC, last_seen DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanDevices(rows)
}

func (r *Registry) Get(mac string) (*Device, error) {
	d, err := r.get(normalizeMac(mac))
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *Registry) DeviceEvents(mac string, limit int) ([]DeviceEvent, error) {
	rows, err := r.db.Query(`
		SELECT id, mac, type, ip, source, note, created_at
		FROM device_events WHERE mac = ?
		ORDER BY created_at DESC LIMIT ?`, normalizeMac(mac), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []DeviceEvent
	for rows.Next() {
		var ev DeviceEvent
		var createdAt string
		if err := rows.Scan(&ev.ID, &ev.MAC, &ev.Type, &ev.IP, &ev.Source, &ev.Note, &createdAt); err != nil {
			continue
		}
		ev.CreatedAt = parseTime(createdAt)
		events = append(events, ev)
	}
	return events, nil
}

func (r *Registry) OnlineCount() (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM devices WHERE online = 1`).Scan(&n)
	return n, err
}

func (r *Registry) macByIP(ip string) string {
	var mac string
	r.db.QueryRow(`SELECT mac FROM devices WHERE ip = ? AND online = 1 LIMIT 1`, ip).Scan(&mac)
	return mac
}

// --- Internal ---

func (r *Registry) get(mac string) (Device, error) {
	row := r.db.QueryRow(`
		SELECT mac, ip, hostname, hostname_auto, vendor, label, category, services, priority, online, first_seen, last_seen
		FROM devices WHERE mac = ?`, mac)
	return r.scanDevice(row)
}

func (r *Registry) scanDevice(row *sql.Row) (Device, error) {
	var d Device
	var firstSeen, lastSeen, servicesJSON string
	err := row.Scan(&d.MAC, &d.IP, &d.Hostname, &d.HostnameAuto, &d.Vendor, &d.Label,
		&d.Category, &servicesJSON, &d.Priority, &d.Online, &firstSeen, &lastSeen)
	if err != nil {
		return d, err
	}
	d.FirstSeen = parseTime(firstSeen)
	d.LastSeen = parseTime(lastSeen)
	d.Services = unmarshalServices(servicesJSON)
	return d, nil
}

func (r *Registry) scanDevices(rows *sql.Rows) ([]Device, error) {
	var devices []Device
	for rows.Next() {
		var d Device
		var firstSeen, lastSeen, servicesJSON string
		err := rows.Scan(&d.MAC, &d.IP, &d.Hostname, &d.HostnameAuto, &d.Vendor, &d.Label,
			&d.Category, &servicesJSON, &d.Priority, &d.Online, &firstSeen, &lastSeen)
		if err != nil {
			continue
		}
		d.FirstSeen = parseTime(firstSeen)
		d.LastSeen = parseTime(lastSeen)
		d.Services = unmarshalServices(servicesJSON)
		devices = append(devices, d)
	}
	return devices, nil
}

func (r *Registry) insert(d Device) error {
	_, err := r.db.Exec(`
		INSERT INTO devices(mac, ip, hostname, hostname_auto, vendor, label, category, services, priority, online, first_seen, last_seen)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		d.MAC, d.IP, d.Hostname, d.HostnameAuto, d.Vendor, d.Label, d.Category,
		marshalServices(d.Services), d.Priority, d.Online, d.FirstSeen, d.LastSeen,
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

func (r *Registry) logEvent(mac, ip, typ, source, note string) {
	r.db.Exec(`
		INSERT INTO device_events(id, mac, type, ip, source, note, created_at)
		VALUES(?,?,?,?,?,?,?)`,
		uuid.NewString(), mac, typ, ip, source, note, time.Now(),
	)
}

func normalizeMac(mac string) string {
	mac = strings.ToUpper(strings.TrimSpace(mac))
	if len(mac) == 12 {
		return mac[0:2] + ":" + mac[2:4] + ":" + mac[4:6] + ":" + mac[6:8] + ":" + mac[8:10] + ":" + mac[10:12]
	}
	return mac
}

func marshalServices(s []string) string {
	if len(s) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(s)
	return string(b)
}

func unmarshalServices(s string) []string {
	if s == "" || s == "[]" {
		return nil
	}
	var out []string
	json.Unmarshal([]byte(s), &out)
	return out
}

func mergeServices(existing, newSvcs []string) []string {
	set := make(map[string]struct{})
	for _, s := range existing {
		set[s] = struct{}{}
	}
	result := append([]string{}, existing...)
	for _, s := range newSvcs {
		if _, ok := set[s]; !ok {
			result = append(result, s)
		}
	}
	return result
}
