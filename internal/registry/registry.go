package registry

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sort"
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
	OsInfo       string   // from nmap OS detection
	NmapPorts    []string // open ports from last nmap scan, e.g. ["22/tcp open ssh OpenSSH 8.4"]
	ForcePing    bool   // use ICMP ping instead of ARP (for devices outside local subnet)
	Priority     bool
	Approved     bool
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
	paused         bool
	// lastWrite tracks the last time each device was actually written to DB.
	// Debounces high-frequency ARP traffic to reduce SQLite write contention.
	lastWrite map[string]time.Time
}

func (r *Registry) SetListening(enabled bool) {
	r.mu.Lock()
	r.paused = !enabled
	r.mu.Unlock()
}

func (r *Registry) IsListening() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.paused
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
		lastWrite:      make(map[string]time.Time),
	}
	// Sync: ARP-Bursts sollen sequenziell verarbeitet werden, nicht parallel
	b.SubscribeSync(bus.EventDeviceSeen, r.handleSeen)
	return r, nil
}

func (r *Registry) handleSeen(e bus.Event) {
	r.mu.Lock()
	paused := r.paused
	r.mu.Unlock()
	if paused {
		return
	}
	// Resolve MAC from IP for mDNS events — needs a quick DB read, no heavy lock.
	mac := normalizeMac(e.MAC)
	if mac == "" {
		if e.IP == "" {
			return
		}
		mac = r.macByIP(e.IP)
		if mac == "" {
			return
		}
	}

	now := time.Now()

	r.mu.Lock()
	existing, err := r.get(mac)
	r.mu.Unlock()

	if err == sql.ErrNoRows {
		// New device: do expensive lookups outside any lock.
		vendor := r.oui.Lookup(mac)
		hostnameAuto, _ := e.Meta["hostname"].(string)
		if hostnameAuto == "" && e.IP != "" {
			hostnameAuto = reverseDNS(e.IP) // may take up to 2s — outside lock
		}
		svcs, _ := e.Meta["services"].([]string)
		dev := Device{
			MAC:          mac,
			IP:           e.IP,
			HostnameAuto: hostnameAuto,
			Vendor:       vendor,
			Services:     svcs,
			Approved:     false,
			Online:       true,
			FirstSeen:    now,
			LastSeen:     now,
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		// Re-check under lock to avoid double-insert from concurrent events.
		if _, err2 := r.get(mac); err2 != sql.ErrNoRows {
			return
		}
		if err := r.insert(dev); err != nil {
			slog.Error("registry: insert failed", "mac", mac, "err", err)
			return
		}
		r.lastWrite[mac] = now
		r.logEvent(mac, e.IP, "seen", e.Source, "")
		slog.Info("new device", "mac", mac, "ip", e.IP, "vendor", vendor)
		r.b.Publish(bus.NewEvent(bus.EventDeviceNew, mac, e.IP, "registry", map[string]any{
			"vendor":   vendor,
			"hostname": hostnameAuto,
			"firstSeen": time.Now().Format("2006-01-02 15:04"),
		}))
		return
	}
	if err != nil {
		slog.Error("registry: get failed", "mac", mac, "err", err)
		return
	}

	wasOffline := !existing.Online
	hostnameAuto, _ := e.Meta["hostname"].(string)
	svcs, _ := e.Meta["services"].([]string)

	ipChanged := e.IP != "" && e.IP != existing.IP
	hostnameChanged := hostnameAuto != "" && hostnameAuto != existing.HostnameAuto
	svcsChanged := len(svcs) > 0 && len(mergeServices(existing.Services, svcs)) > len(existing.Services)
	// debounceWindow suppresses redundant DB writes for stable online devices.
	// We use 1/3 of the offline timeout so a device gets written at least 3×
	// per timeout period — enough to keep last_seen fresh without hammering SQLite.
	debounceWindow := r.offlineTimeout / 3

	// Skip DB write if device is already online, nothing changed, and written recently.
	if !wasOffline && !ipChanged && !hostnameChanged && !svcsChanged {
		r.mu.Lock()
		recent := time.Since(r.lastWrite[mac]) < debounceWindow
		r.mu.Unlock()
		if recent {
			return
		}
	}

	updates := map[string]any{"last_seen": now, "online": true}
	if ipChanged {
		updates["ip"] = e.IP
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if hostnameChanged {
		updates["hostname_auto"] = hostnameAuto
		r.logEvent(mac, e.IP, "hostname", e.Source, hostnameAuto)
	}
	if svcsChanged {
		merged := mergeServices(existing.Services, svcs)
		updates["services"] = marshalServices(merged)
		r.logEvent(mac, e.IP, "services", e.Source, strings.Join(svcs, ", "))
	}
	if err := r.update(mac, updates); err != nil {
		slog.Error("registry: update failed", "mac", mac, "err", err)
		return
	}
	r.lastWrite[mac] = now

	if wasOffline {
		r.logEvent(mac, e.IP, "online", e.Source, "")
		slog.Info("device back", "mac", mac, "ip", e.IP)
		r.b.Publish(bus.NewEvent(bus.EventDeviceBack, mac, e.IP, "registry", map[string]any{
			"label":        existing.Label,
			"hostname":     existing.Hostname,
			"hostnameAuto": existing.HostnameAuto,
			"vendor":       existing.Vendor,
			"category":     existing.Category,
			"groups":       r.deviceGroupNames(mac),
			"lastSeen":     existing.LastSeen.Format("02.01.2006 15:04"),
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

type offlineEntry struct {
	mac, ip, label, hostname, hostnameAuto, category, vendor, groups string
	priority                                                          bool
	lastSeen                                                          string
}

func (r *Registry) checkOffline() {
	cutoff := time.Now().Add(-r.offlineTimeout)

	// Collect candidates first, then close the cursor before writing.
	rows, err := r.db.Query(
		`SELECT mac, ip, label, priority, hostname, hostname_auto, category, vendor, last_seen
		 FROM devices WHERE online = 1 AND last_seen < ? AND category != 'virtual'`, cutoff,
	)
	if err != nil {
		return
	}
	var candidates []offlineEntry
	for rows.Next() {
		var e offlineEntry
		var ls string
		if err := rows.Scan(&e.mac, &e.ip, &e.label, &e.priority, &e.hostname, &e.hostnameAuto, &e.category, &e.vendor, &ls); err != nil {
			continue
		}
		e.lastSeen = parseTimestamp(ls)
		candidates = append(candidates, e)
	}
	rows.Close()

	if len(candidates) == 0 {
		return
	}

	// Fetch group names before acquiring the write lock.
	for i := range candidates {
		candidates[i].groups = r.deviceGroupNames(candidates[i].mac)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range candidates {
		r.db.Exec(`UPDATE devices SET online = 0 WHERE mac = ?`, e.mac)
		r.logEvent(e.mac, e.ip, "offline", "registry", "")
		slog.Info("device lost", "mac", e.mac, "ip", e.ip)
		r.b.Publish(bus.NewEvent(bus.EventDeviceLost, e.mac, e.ip, "registry", map[string]any{
			"label":        e.label,
			"hostname":     e.hostname,
			"hostnameAuto": e.hostnameAuto,
			"category":     e.category,
			"vendor":       e.vendor,
			"groups":       e.groups,
			"lastSeen":     e.lastSeen,
			"priority":     e.priority,
		}))
	}
}

// deviceGroupNames returns a comma-separated list of group names for a device.
func (r *Registry) deviceGroupNames(mac string) string {
	rows, err := r.db.Query(
		`SELECT g.name FROM device_groups g
		 JOIN device_group_members m ON m.group_id = g.id
		 WHERE m.mac = ? ORDER BY g.name`, mac,
	)
	if err != nil {
		return ""
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil {
			names = append(names, n)
		}
	}
	return strings.Join(names, ", ")
}

// parseTimestamp parses a SQLite timestamp string into a human-readable format.
func parseTimestamp(s string) string {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("02.01.2006 15:04")
		}
	}
	return s
}

// --- Public API ---

// AddManual creates a device that was not auto-discovered.
// If mac is empty a synthetic one is generated from the IP.
func (r *Registry) AddManual(mac, ip, label, category string) (*Device, error) {
	if mac == "" && ip != "" {
		mac = "00:00:00:00:00:00" // placeholder, will be keyed by synthetic value
		// use IP bytes as synthetic MAC so it's unique
		parts := strings.SplitN(ip, ".", 4)
		if len(parts) == 4 {
			mac = fmt.Sprintf("02:00:%02x:%02x:%02x:%02x",
				atoi(parts[0]), atoi(parts[1]), atoi(parts[2]), atoi(parts[3]))
		}
	}
	mac = normalizeMac(mac)
	if mac == "" {
		return nil, fmt.Errorf("MAC oder IP erforderlich")
	}

	// Check if already exists
	existing, err := r.get(mac)
	if err == nil {
		return &existing, nil
	}

	vendor := r.oui.Lookup(mac)
	hostname := reverseDNS(ip)
	now := time.Now()
	dev := Device{
		MAC:          mac,
		IP:           ip,
		Label:        label,
		HostnameAuto: hostname,
		Vendor:       vendor,
		Category:     category,
		Approved:     true,
		Online:       false,
		FirstSeen:    now,
		LastSeen:     now,
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.insert(dev); err != nil {
		return nil, err
	}
	r.logEvent(mac, ip, "seen", "manual", "")
	return &dev, nil
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

func (r *Registry) GetSetting(key string) (string, error) {
	var val string
	err := r.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&val)
	return val, err
}

func (r *Registry) SetSetting(key, value string) error {
	_, err := r.db.Exec(
		`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		key, value,
	)
	return err
}

func (r *Registry) Delete(mac string) error {
	mac = normalizeMac(mac)
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.lastWrite, mac)
	r.db.Exec(`DELETE FROM device_events      WHERE mac = ?`, mac)
	r.db.Exec(`DELETE FROM device_connections WHERE mac_a = ? OR mac_b = ?`, mac, mac)
	_, err := r.db.Exec(`DELETE FROM devices WHERE mac = ?`, mac)
	return err
}

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

func (r *Registry) Approve(mac string) error {
	_, err := r.db.Exec(`UPDATE devices SET approved = 1 WHERE mac = ?`, normalizeMac(mac))
	return err
}

func (r *Registry) SetCategory(mac, category string) error {
	_, err := r.db.Exec(`UPDATE devices SET category = ? WHERE mac = ?`, category, normalizeMac(mac))
	return err
}

func (r *Registry) List() ([]Device, error) {
	rows, err := r.db.Query(`
		SELECT mac, ip, hostname, hostname_auto, vendor, label, category, services, priority, approved, online, first_seen, last_seen, os_info, force_ping, nmap_ports
		FROM devices
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	devices, err := r.scanDevices(rows)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(devices, func(i, j int) bool {
		if devices[i].Online != devices[j].Online {
			return devices[i].Online
		}
		return ipLess(devices[i].IP, devices[j].IP)
	})
	return devices, nil
}

// ipLess compares two IPv4 addresses numerically.
func ipLess(a, b string) bool {
	ia := net.ParseIP(a).To4()
	ib := net.ParseIP(b).To4()
	if ia == nil || ib == nil {
		return a < b
	}
	for i := 0; i < 4; i++ {
		if ia[i] != ib[i] {
			return ia[i] < ib[i]
		}
	}
	return false
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

// --- Connections ---

type Connection struct {
	ID    string
	MACa  string
	MACb  string
	Type  string // "physical" | "logical"
	Label string
}

func (r *Registry) AddConnection(macA, macB, connType, label string) error {
	macA, macB = normalizeMac(macA), normalizeMac(macB)
	// Avoid duplicates (order-independent)
	var exists int
	r.db.QueryRow(`SELECT COUNT(*) FROM device_connections
		WHERE (mac_a=? AND mac_b=?) OR (mac_a=? AND mac_b=?)`,
		macA, macB, macB, macA).Scan(&exists)
	if exists > 0 {
		return nil
	}
	_, err := r.db.Exec(`INSERT INTO device_connections(id,mac_a,mac_b,type,label,created_at)
		VALUES(?,?,?,?,?,?)`, uuid.NewString(), macA, macB, connType, label, time.Now())
	return err
}

func (r *Registry) RemoveConnection(id string) error {
	_, err := r.db.Exec(`DELETE FROM device_connections WHERE id=?`, id)
	return err
}

func (r *Registry) GetConnections(mac string) ([]Connection, error) {
	mac = normalizeMac(mac)
	rows, err := r.db.Query(`SELECT id,mac_a,mac_b,type,label FROM device_connections
		WHERE mac_a=? OR mac_b=? ORDER BY created_at`, mac, mac)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Connection
	for rows.Next() {
		var c Connection
		rows.Scan(&c.ID, &c.MACa, &c.MACb, &c.Type, &c.Label)
		out = append(out, c)
	}
	return out, nil
}

// --- Topology for map ---

type TopoNode struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	IP       string   `json:"ip"`
	Vendor   string   `json:"vendor"`
	Category string   `json:"category"`
	Groups   []string `json:"groups"`
	Online   bool     `json:"online"`
	Priority bool     `json:"priority"`
	Approved bool     `json:"approved"`
	OsInfo    string   `json:"os,omitempty"`
	Services  []string `json:"services,omitempty"`
	NmapPorts []string `json:"ports,omitempty"`
}

type TopoGroup struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type TopoLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`  // "physical" | "logical" | "auto"
	Label  string `json:"label"`
}

type Topology struct {
	Nodes  []TopoNode  `json:"nodes"`
	Links  []TopoLink  `json:"links"`
	Groups []TopoGroup `json:"groups"`
}

func (r *Registry) Topology() (*Topology, error) {
	devices, err := r.List()
	if err != nil {
		return nil, err
	}

	// Load all group memberships
	groupRows, err := r.db.Query(`SELECT mac, group_id FROM device_group_members`)
	if err != nil {
		return nil, err
	}
	groupsOf := make(map[string][]string)
	for groupRows.Next() {
		var mac, gid string
		groupRows.Scan(&mac, &gid)
		groupsOf[mac] = append(groupsOf[mac], gid)
	}
	groupRows.Close()

	// Load group metadata
	groups, _ := r.ListGroups()

	topo := &Topology{}
	for _, g := range groups {
		topo.Groups = append(topo.Groups, TopoGroup{ID: g.ID, Name: g.Name, Color: g.Color})
	}

	// Build nodes
	for _, d := range devices {
		grps := groupsOf[d.MAC]
		if grps == nil {
			grps = []string{}
		}
		topo.Nodes = append(topo.Nodes, TopoNode{
			ID:        d.MAC,
			Name:      d.DisplayName(),
			IP:        d.IP,
			Vendor:    d.Vendor,
			Category:  d.Category,
			Groups:    grps,
			Online:    d.Online,
			Priority:  d.Priority,
			Approved:  d.Approved,
			OsInfo:    d.OsInfo,
			Services:  d.Services,
			NmapPorts: d.NmapPorts,
		})
	}

	// Build node ID set for dangling-link filtering
	nodeIDs := make(map[string]bool, len(topo.Nodes))
	for _, n := range topo.Nodes {
		nodeIDs[n.ID] = true
	}

	// Manual connections — skip any where source or target no longer exists
	rows, err := r.db.Query(`SELECT mac_a, mac_b, type, label FROM device_connections`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var l TopoLink
		rows.Scan(&l.Source, &l.Target, &l.Type, &l.Label)
		if nodeIDs[l.Source] && nodeIDs[l.Target] {
			topo.Links = append(topo.Links, l)
		}
	}

	// Auto-links: connect devices in same /24 subnet to the gateway (.1)
	type subnet struct{ prefix, gateway string }
	gatewayOf := make(map[string]string) // subnet prefix → gateway MAC
	macOfIP := make(map[string]string)   // ip → mac

	for _, d := range devices {
		if d.IP == "" {
			continue
		}
		macOfIP[d.IP] = d.MAC
		parts := strings.SplitN(d.IP, ".", 4)
		if len(parts) != 4 {
			continue
		}
		prefix := parts[0] + "." + parts[1] + "." + parts[2]
		if parts[3] == "1" || parts[3] == "254" {
			gatewayOf[prefix] = d.MAC
		}
	}

	added := make(map[string]bool)
	for _, d := range devices {
		if d.IP == "" {
			continue
		}
		parts := strings.SplitN(d.IP, ".", 4)
		if len(parts) != 4 {
			continue
		}
		prefix := parts[0] + "." + parts[1] + "." + parts[2]
		gw, ok := gatewayOf[prefix]
		if !ok || gw == d.MAC {
			continue
		}
		key := gw + "↔" + d.MAC
		if added[key] {
			continue
		}
		added[key] = true
		topo.Links = append(topo.Links, TopoLink{
			Source: gw,
			Target: d.MAC,
			Type:   "auto",
		})
	}

	return topo, nil
}

func (r *Registry) OnlineCount() (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM devices WHERE online = 1`).Scan(&n)
	return n, err
}

// RecentEvents returns the latest device_events across all devices for the log page.
func (r *Registry) RecentEvents(limit int) ([]DeviceEvent, error) {
	rows, err := r.db.Query(`
		SELECT id, mac, type, ip, source, note, created_at
		FROM device_events
		ORDER BY created_at DESC LIMIT ?`, limit)
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

// BackfillVendors sets vendor for existing devices that have none.
func (r *Registry) BackfillVendors() {
	r.mu.Lock()
	defer r.mu.Unlock()

	rows, err := r.db.Query(`SELECT mac FROM devices WHERE vendor = ''`)
	if err != nil {
		return
	}
	var macs []string
	for rows.Next() {
		var mac string
		rows.Scan(&mac)
		macs = append(macs, mac)
	}
	rows.Close()

	for _, mac := range macs {
		if v := r.oui.Lookup(mac); v != "" {
			r.db.Exec(`UPDATE devices SET vendor = ? WHERE mac = ?`, v, mac)
		}
	}
}

// BackfillReverseDNS sets hostname_auto via PTR lookup for devices with no hostname.
// DNS lookups run without any lock; results are written in one batch at the end.
func (r *Registry) BackfillReverseDNS() {
	r.mu.Lock()
	rows, err := r.db.Query(`SELECT mac, ip FROM devices WHERE hostname_auto = '' AND ip != ''`)
	if err != nil {
		r.mu.Unlock()
		return
	}
	type entry struct{ mac, ip string }
	var entries []entry
	for rows.Next() {
		var e entry
		rows.Scan(&e.mac, &e.ip)
		entries = append(entries, e)
	}
	rows.Close()
	r.mu.Unlock()

	// DNS lookups outside any lock — can take a while
	results := make(map[string]string, len(entries))
	for _, e := range entries {
		if name := reverseDNS(e.ip); name != "" {
			results[e.mac] = name
			slog.Debug("rdns backfill", "mac", e.mac, "ip", e.ip, "name", name)
		}
	}

	if len(results) == 0 {
		return
	}

	// Single locked batch write
	r.mu.Lock()
	defer r.mu.Unlock()
	for mac, name := range results {
		r.db.Exec(`UPDATE devices SET hostname_auto = ? WHERE mac = ? AND hostname_auto = ''`, name, mac)
	}
}

// reverseDNS does a PTR lookup for the given IP using the system resolver.
// Returns only the hostname part (strips domain), ignores IPv6 arpa results.
func reverseDNS(ip string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	names, err := net.DefaultResolver.LookupAddr(ctx, ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	for _, name := range names {
		name = strings.TrimSuffix(name, ".")
		if strings.Contains(name, ".ip6.arpa") || strings.Contains(name, ".in-addr.arpa") {
			continue
		}
		if idx := strings.Index(name, "."); idx != -1 {
			return name[:idx]
		}
		return name
	}
	return ""
}

func (r *Registry) macByIP(ip string) string {
	var mac string
	r.db.QueryRow(`SELECT mac FROM devices WHERE ip = ? AND online = 1 LIMIT 1`, ip).Scan(&mac)
	return mac
}

// --- Internal ---

func (r *Registry) get(mac string) (Device, error) {
	row := r.db.QueryRow(`
		SELECT mac, ip, hostname, hostname_auto, vendor, label, category, services, priority, approved, online, first_seen, last_seen, os_info, force_ping, nmap_ports
		FROM devices WHERE mac = ?`, mac)
	return r.scanDevice(row)
}

func (r *Registry) scanDevice(row *sql.Row) (Device, error) {
	var d Device
	var firstSeen, lastSeen, servicesJSON, nmapPortsJSON string
	err := row.Scan(&d.MAC, &d.IP, &d.Hostname, &d.HostnameAuto, &d.Vendor, &d.Label,
		&d.Category, &servicesJSON, &d.Priority, &d.Approved, &d.Online, &firstSeen, &lastSeen, &d.OsInfo, &d.ForcePing, &nmapPortsJSON)
	if err != nil {
		return d, err
	}
	d.FirstSeen = parseTime(firstSeen)
	d.LastSeen = parseTime(lastSeen)
	d.Services = unmarshalServices(servicesJSON)
	d.NmapPorts = unmarshalServices(nmapPortsJSON)
	return d, nil
}

func (r *Registry) scanDevices(rows *sql.Rows) ([]Device, error) {
	var devices []Device
	for rows.Next() {
		var d Device
		var firstSeen, lastSeen, servicesJSON, nmapPortsJSON string
		err := rows.Scan(&d.MAC, &d.IP, &d.Hostname, &d.HostnameAuto, &d.Vendor, &d.Label,
			&d.Category, &servicesJSON, &d.Priority, &d.Approved, &d.Online, &firstSeen, &lastSeen, &d.OsInfo, &d.ForcePing, &nmapPortsJSON)
		if err != nil {
			continue
		}
		d.FirstSeen = parseTime(firstSeen)
		d.LastSeen = parseTime(lastSeen)
		d.Services = unmarshalServices(servicesJSON)
		d.NmapPorts = unmarshalServices(nmapPortsJSON)
		devices = append(devices, d)
	}
	return devices, nil
}

func (r *Registry) insert(d Device) error {
	_, err := r.db.Exec(`
		INSERT INTO devices(mac, ip, hostname, hostname_auto, vendor, label, category, services, priority, approved, online, first_seen, last_seen, os_info, force_ping)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		d.MAC, d.IP, d.Hostname, d.HostnameAuto, d.Vendor, d.Label, d.Category,
		marshalServices(d.Services), d.Priority, d.Approved, d.Online, d.FirstSeen, d.LastSeen, d.OsInfo, d.ForcePing,
	)
	return err
}


// update applies a partial UPDATE to a device row. Field names come from
// internal callers only (never from user input), so concatenating them into
// the query is safe. Values are always passed as parameters to prevent
// SQL injection regardless of their source.
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

// --- Groups ---

type Group struct {
	ID        string
	Name      string
	Color     string
	SortOrder int
}

func (r *Registry) CreateGroup(name, color string) (*Group, error) {
	id := uuid.NewString()
	var maxOrder int
	r.db.QueryRow(`SELECT COALESCE(MAX(sort_order),0) FROM device_groups`).Scan(&maxOrder)
	_, err := r.db.Exec(`INSERT INTO device_groups(id,name,color,sort_order) VALUES(?,?,?,?)`, id, name, color, maxOrder+1)
	if err != nil {
		return nil, err
	}
	return &Group{ID: id, Name: name, Color: color, SortOrder: maxOrder + 1}, nil
}

func (r *Registry) ListGroups() ([]Group, error) {
	rows, err := r.db.Query(`SELECT id,name,color,sort_order FROM device_groups ORDER BY sort_order, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		rows.Scan(&g.ID, &g.Name, &g.Color, &g.SortOrder)
		out = append(out, g)
	}
	return out, nil
}

func (r *Registry) MoveGroup(id, direction string) error {
	var curOrder int
	if err := r.db.QueryRow(`SELECT sort_order FROM device_groups WHERE id=?`, id).Scan(&curOrder); err != nil {
		return err
	}
	var adjID string
	var adjOrder int
	var err error
	if direction == "up" {
		err = r.db.QueryRow(`SELECT id,sort_order FROM device_groups WHERE sort_order < ? ORDER BY sort_order DESC LIMIT 1`, curOrder).Scan(&adjID, &adjOrder)
	} else {
		err = r.db.QueryRow(`SELECT id,sort_order FROM device_groups WHERE sort_order > ? ORDER BY sort_order ASC LIMIT 1`, curOrder).Scan(&adjID, &adjOrder)
	}
	if err != nil {
		return nil // already first or last
	}
	r.db.Exec(`UPDATE device_groups SET sort_order=? WHERE id=?`, adjOrder, id)
	r.db.Exec(`UPDATE device_groups SET sort_order=? WHERE id=?`, curOrder, adjID)
	return nil
}

func (r *Registry) UpdateGroup(id, name, color string) error {
	_, err := r.db.Exec(`UPDATE device_groups SET name=?,color=? WHERE id=?`, name, color, id)
	return err
}

func (r *Registry) DeleteGroup(id string) error {
	r.db.Exec(`DELETE FROM device_group_members WHERE group_id=?`, id)
	_, err := r.db.Exec(`DELETE FROM device_groups WHERE id=?`, id)
	return err
}

func (r *Registry) AddDeviceToGroup(mac, groupID string) error {
	_, err := r.db.Exec(`INSERT OR IGNORE INTO device_group_members(group_id,mac) VALUES(?,?)`, groupID, normalizeMac(mac))
	return err
}

func (r *Registry) RemoveDeviceFromGroup(mac, groupID string) error {
	_, err := r.db.Exec(`DELETE FROM device_group_members WHERE group_id=? AND mac=?`, groupID, normalizeMac(mac))
	return err
}

func (r *Registry) GetDeviceGroups(mac string) ([]Group, error) {
	rows, err := r.db.Query(`
		SELECT g.id,g.name,g.color FROM device_groups g
		JOIN device_group_members m ON m.group_id=g.id
		WHERE m.mac=? ORDER BY g.name`, normalizeMac(mac))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		rows.Scan(&g.ID, &g.Name, &g.Color)
		out = append(out, g)
	}
	return out, nil
}

func (r *Registry) GetGroupDevices(groupID string) ([]Device, error) {
	rows, err := r.db.Query(`
		SELECT d.mac,d.ip,d.hostname,d.hostname_auto,d.vendor,d.label,d.category,
		       d.services,d.priority,d.approved,d.online,d.first_seen,d.last_seen,d.os_info,d.force_ping
		FROM devices d JOIN device_group_members m ON m.mac=d.mac
		WHERE m.group_id=? ORDER BY d.ip`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanDevices(rows)
}

// --- Export / Import ---

// ExportRecord is one device entry in an export/import file.
type ExportRecord struct {
	MAC          string    `json:"mac"`
	Label        string    `json:"label,omitempty"`
	Hostname     string    `json:"hostname,omitempty"`
	Category     string    `json:"category,omitempty"`
	OsInfo       string    `json:"os_info,omitempty"`
	Priority     bool      `json:"priority"`
	Approved     bool      `json:"approved"`
	IP           string    `json:"ip,omitempty"`
	HostnameAuto string    `json:"hostname_auto,omitempty"`
	Vendor       string    `json:"vendor,omitempty"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
	Groups       []string  `json:"groups,omitempty"` // group IDs
}

type ExportGroup struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type ExportConnection struct {
	MACa  string `json:"mac_a"`
	MACb  string `json:"mac_b"`
	Type  string `json:"type"`
	Label string `json:"label,omitempty"`
}

// ExportPayload is the root object of an export file.
type ExportPayload struct {
	Version     int                `json:"version"`
	ExportedAt  time.Time          `json:"exported_at"`
	Devices     []ExportRecord     `json:"devices"`
	Groups      []ExportGroup      `json:"groups,omitempty"`
	Connections []ExportConnection `json:"connections,omitempty"`
}

// ImportResult counts how many devices were created and updated during import.
type ImportResult struct {
	Created int
	Updated int
}

func (r *Registry) Export() (*ExportPayload, error) {
	devices, err := r.List()
	if err != nil {
		return nil, err
	}

	records := make([]ExportRecord, len(devices))
	for i, d := range devices {
		groups, _ := r.GetDeviceGroups(d.MAC)
		groupIDs := make([]string, len(groups))
		for j, g := range groups {
			groupIDs[j] = g.ID
		}
		records[i] = ExportRecord{
			MAC:          d.MAC,
			Label:        d.Label,
			Hostname:     d.Hostname,
			Category:     d.Category,
			OsInfo:       d.OsInfo,
			Priority:     d.Priority,
			Approved:     d.Approved,
			IP:           d.IP,
			HostnameAuto: d.HostnameAuto,
			Vendor:       d.Vendor,
			FirstSeen:    d.FirstSeen,
			LastSeen:     d.LastSeen,
			Groups:       groupIDs,
		}
	}

	// Groups
	allGroups, _ := r.ListGroups()
	exportGroups := make([]ExportGroup, len(allGroups))
	for i, g := range allGroups {
		exportGroups[i] = ExportGroup{ID: g.ID, Name: g.Name, Color: g.Color}
	}

	// Connections (deduplicated — each connection appears once)
	seen := map[string]bool{}
	var exportConns []ExportConnection
	for _, d := range devices {
		conns, _ := r.GetConnections(d.MAC)
		for _, c := range conns {
			key := c.MACa + "→" + c.MACb
			if seen[key] {
				continue
			}
			seen[key] = true
			seen[c.MACb+"→"+c.MACa] = true
			exportConns = append(exportConns, ExportConnection{
				MACa:  c.MACa,
				MACb:  c.MACb,
				Type:  c.Type,
				Label: c.Label,
			})
		}
	}

	return &ExportPayload{
		Version:     1,
		ExportedAt:  time.Now(),
		Devices:     records,
		Groups:      exportGroups,
		Connections: exportConns,
	}, nil
}

func (r *Registry) Import(payload *ExportPayload) (ImportResult, error) {
	var res ImportResult
	now := time.Now()

	for _, rec := range payload.Devices {
		mac := normalizeMac(rec.MAC)
		if mac == "" {
			continue
		}

		r.mu.Lock()
		_, err := r.get(mac)
		r.mu.Unlock()

		if err == sql.ErrNoRows {
			firstSeen := rec.FirstSeen
			if firstSeen.IsZero() {
				firstSeen = now
			}
			dev := Device{
				MAC:          mac,
				IP:           rec.IP,
				Label:        rec.Label,
				Hostname:     rec.Hostname,
				HostnameAuto: rec.HostnameAuto,
				Vendor:       rec.Vendor,
				Category:     rec.Category,
				OsInfo:       rec.OsInfo,
				Priority:     rec.Priority,
				Approved:     rec.Approved,
				Online:       false,
				FirstSeen:    firstSeen,
				LastSeen:     now,
			}
			r.mu.Lock()
			if err := r.insert(dev); err == nil {
				r.logEvent(mac, rec.IP, "seen", "import", "")
				res.Created++
			}
			r.mu.Unlock()
		} else if err == nil {
			updates := map[string]any{
				"priority": rec.Priority,
				"approved": rec.Approved,
			}
			if rec.Label != "" {
				updates["label"] = rec.Label
			}
			if rec.Hostname != "" {
				updates["hostname"] = rec.Hostname
			}
			if rec.Category != "" {
				updates["category"] = rec.Category
			}
			if rec.OsInfo != "" {
				updates["os_info"] = rec.OsInfo
			}
			r.mu.Lock()
			if err := r.update(mac, updates); err == nil {
				// no event logged for import updates — would be noise
				res.Updated++
			}
			r.mu.Unlock()
		}
	}
	// Restore groups (create if ID doesn't exist yet, update name/color if it does)
	groupIDMap := map[string]string{} // exported ID → local ID
	for _, eg := range payload.Groups {
		existing, _ := r.ListGroups()
		found := false
		for _, lg := range existing {
			if lg.ID == eg.ID {
				r.UpdateGroup(lg.ID, eg.Name, eg.Color)
				groupIDMap[eg.ID] = lg.ID
				found = true
				break
			}
		}
		if !found {
			// Insert with the original ID so foreign keys match
			r.db.Exec(`INSERT OR IGNORE INTO device_groups (id, name, color) VALUES (?, ?, ?)`,
				eg.ID, eg.Name, eg.Color)
			groupIDMap[eg.ID] = eg.ID
		}
	}

	// Restore group memberships per device
	for _, rec := range payload.Devices {
		mac := normalizeMac(rec.MAC)
		if mac == "" {
			continue
		}
		for _, gid := range rec.Groups {
			if localID, ok := groupIDMap[gid]; ok {
				r.AddDeviceToGroup(mac, localID)
			}
		}
	}

	// Restore connections
	for _, ec := range payload.Connections {
		macA := normalizeMac(ec.MACa)
		macB := normalizeMac(ec.MACb)
		if macA != "" && macB != "" {
			r.AddConnection(macA, macB, ec.Type, ec.Label)
		}
	}

	return res, nil
}

// PriorityDevices returns all priority devices that have an IP address.
func (r *Registry) PriorityDevices() []Device {
	rows, err := r.db.Query(`
		SELECT mac, ip, hostname, hostname_auto, vendor, label, category, services, priority, approved, online, first_seen, last_seen, os_info, force_ping, nmap_ports
		FROM devices WHERE priority = 1 AND ip != ''`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	devices, _ := r.scanDevices(rows)
	return devices
}

// SetForcePing enables/disables ICMP-ping fallback for a device (used when ARP doesn't reach it).
func (r *Registry) SetForcePing(mac string, v bool) error {
	_, err := r.db.Exec(`UPDATE devices SET force_ping = ? WHERE mac = ?`, v, normalizeMac(mac))
	return err
}

// SetOsInfo stores nmap OS detection results for a device.
func (r *Registry) SetOsInfo(mac, osInfo string) error {
	_, err := r.db.Exec(`UPDATE devices SET os_info = ? WHERE mac = ?`, osInfo, normalizeMac(mac))
	return err
}

// SetNmapPorts stores the open ports from the last nmap scan for a device.
func (r *Registry) SetNmapPorts(mac string, ports []string) error {
	_, err := r.db.Exec(`UPDATE devices SET nmap_ports = ? WHERE mac = ?`, marshalServices(ports), normalizeMac(mac))
	return err
}

// AddEvent logs a device event (public wrapper for internal logEvent).
func (r *Registry) AddEvent(mac, ip, typ, source, note string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logEvent(normalizeMac(mac), ip, typ, source, note)
}

// PruneOldLogs deletes device_events and alerts older than maxAge.
func (r *Registry) PruneOldLogs(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	res, _ := r.db.Exec(`DELETE FROM device_events WHERE created_at < ?`, cutoff)
	if n, _ := res.RowsAffected(); n > 0 {
		slog.Info("pruned old events", "count", n, "older_than", cutoff.Format("2006-01-02"))
	}
	res, _ = r.db.Exec(`DELETE FROM alerts WHERE fired_at < ?`, cutoff)
	if n, _ := res.RowsAffected(); n > 0 {
		slog.Info("pruned old alerts", "count", n)
	}
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
