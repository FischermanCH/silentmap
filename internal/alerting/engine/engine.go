// Package engine routes bus events to alert channels (Discord, ntfy) and
// persists fired alerts to the database for the alert history view.
package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/silentmap/silentmap/internal/alerting/channels"
	"github.com/silentmap/silentmap/internal/bus"
	"github.com/silentmap/silentmap/internal/config"
)

// Alert re-exports channels.Alert for callers that only import engine.
type Alert = channels.Alert

type Engine struct {
	cfg              config.AlertsCfg
	db               *sql.DB
	channels         []channels.Channel
	cooldown         map[string]time.Time
	mu               sync.Mutex
	maintenanceUntil int64 // Unix timestamp; 0 = disabled; accessed via sync/atomic
}

// SetMaintenance puts the alert engine into maintenance mode until `until`.
// Pass the zero Time to cancel maintenance mode immediately.
func (e *Engine) SetMaintenance(until time.Time) {
	if until.IsZero() {
		atomic.StoreInt64(&e.maintenanceUntil, 0)
	} else {
		atomic.StoreInt64(&e.maintenanceUntil, until.Unix())
	}
}

// MaintenanceUntil returns the current maintenance-mode expiry (zero = inactive).
func (e *Engine) MaintenanceUntil() time.Time {
	v := atomic.LoadInt64(&e.maintenanceUntil)
	if v == 0 {
		return time.Time{}
	}
	return time.Unix(v, 0)
}

func New(cfg config.AlertsCfg, db *sql.DB) *Engine {
	return &Engine{
		cfg:      cfg,
		db:       db,
		cooldown: make(map[string]time.Time),
	}
}

func (e *Engine) Register(ch channels.Channel) {
	e.channels = append(e.channels, ch)
}

func (e *Engine) Subscribe(b *bus.Bus) {
	b.Subscribe(bus.EventDeviceNew, e.onDeviceNew)
	b.Subscribe(bus.EventDeviceLost, e.onDeviceLost)
	b.Subscribe(bus.EventDeviceBack, e.onDeviceBack)
}

func (e *Engine) onDeviceNew(ev bus.Event) {
	if !e.cfg.Rules.NewDevice.Enabled {
		return
	}
	vendor, _ := ev.Meta["vendor"].(string)
	hostname, _ := ev.Meta["hostname"].(string)

	display := ev.MAC
	if hostname != "" {
		display = hostname
	} else if vendor != "" {
		display = vendor + " device"
	}

	summary := display + " — " + ev.IP
	if vendor != "" && hostname != "" {
		summary = hostname + " (" + vendor + ") — " + ev.IP
	}

	e.fire(Alert{
		Type:     "new_device",
		Severity: e.cfg.Rules.NewDevice.Severity,
		Title:    "alert.title.new_device",
		Summary:  summary,
		MAC:      ev.MAC,
		IP:       ev.IP,
		Meta:     ev.Meta,
	}, e.cfg.Rules.NewDevice.Cooldown)
}

func (e *Engine) onDeviceLost(ev bus.Event) {
	priority, _ := ev.Meta["priority"].(bool)
	category, _ := ev.Meta["category"].(string)

	if e.cfg.Rules.PriorityOffline.Enabled && priority {
		e.fire(Alert{
			Type:     "priority_offline",
			Severity: e.cfg.Rules.PriorityOffline.Severity,
			Title:    "alert.title.priority_offline",
			Summary:  alertSummary(ev),
			MAC:      ev.MAC,
			IP:       ev.IP,
			Meta:     ev.Meta,
		}, e.cfg.Rules.PriorityOffline.Cooldown)
	}

	if e.cfg.Rules.ServiceDown.Enabled && category == "http-service" {
		e.fire(Alert{
			Type:     "service_down",
			Severity: e.cfg.Rules.ServiceDown.Severity,
			Title:    "alert.title.service_down",
			Summary:  alertSummary(ev),
			MAC:      ev.MAC,
			IP:       ev.IP,
			Meta:     ev.Meta,
		}, e.cfg.Rules.ServiceDown.Cooldown)
	}
}

func (e *Engine) onDeviceBack(ev bus.Event) {
	priority, _ := ev.Meta["priority"].(bool)
	category, _ := ev.Meta["category"].(string)

	if e.cfg.Rules.DeviceBack.Enabled && priority {
		e.fire(Alert{
			Type:     "device_back",
			Severity: e.cfg.Rules.DeviceBack.Severity,
			Title:    "alert.title.device_back",
			Summary:  alertSummary(ev),
			MAC:      ev.MAC,
			IP:       ev.IP,
			Meta:     ev.Meta,
		}, e.cfg.Rules.DeviceBack.Cooldown)
	}

	if e.cfg.Rules.ServiceBack.Enabled && category == "http-service" {
		e.fire(Alert{
			Type:     "service_back",
			Severity: e.cfg.Rules.ServiceBack.Severity,
			Title:    "alert.title.service_back",
			Summary:  alertSummary(ev),
			MAC:      ev.MAC,
			IP:       ev.IP,
			Meta:     ev.Meta,
		}, e.cfg.Rules.ServiceBack.Cooldown)
	}
}

// bestName returns the most human-readable name available in the event metadata:
// label → hostname → hostnameAuto → vendor. Returns "" when nothing is known —
// callers should fall back to showing only the IP rather than repeating the MAC.
func bestName(ev bus.Event) string {
	for _, key := range []string{"label", "hostname", "hostnameAuto", "vendor"} {
		if v, _ := ev.Meta[key].(string); v != "" {
			return v
		}
	}
	return ""
}

// alertSummary builds the one-line summary for an alert:
// "Name · IP" when a human-readable name exists, just "IP" otherwise.
// The MAC is always shown separately as a link in the UI.
func alertSummary(ev bus.Event) string {
	if name := bestName(ev); name != "" {
		return name + " · " + ev.IP
	}
	return ev.IP
}

func (e *Engine) fire(a Alert, cooldownDur time.Duration) {
	if v := atomic.LoadInt64(&e.maintenanceUntil); v > 0 && time.Now().Unix() < v {
		slog.Debug("alert suppressed (maintenance mode)", "type", a.Type, "mac", a.MAC)
		return
	}

	e.mu.Lock()
	key := a.Type + ":" + a.MAC
	if cooldownDur > 0 {
		if t, ok := e.cooldown[key]; ok && time.Since(t) < cooldownDur {
			e.mu.Unlock()
			return
		}
	}
	e.cooldown[key] = time.Now()
	e.mu.Unlock()

	a.ID = uuid.NewString()
	a.FiredAt = time.Now()

	e.persist(a)

	slog.Info("alert fired", "type", a.Type, "severity", a.Severity, "summary", a.Summary)

	targets := e.routeChannels(a.Severity)
	for _, ch := range e.channels {
		for _, name := range targets {
			if ch.Name() == name && ch.Enabled() {
				go func(ch channels.Channel, a Alert) {
					if err := ch.Send(context.Background(), a); err != nil {
						slog.Error("alert channel failed", "channel", ch.Name(), "err", err)
					}
				}(ch, a)
			}
		}
	}
}

func (e *Engine) routeChannels(severity string) []string {
	switch severity {
	case "critical":
		return e.cfg.Routing.Critical
	case "high":
		return e.cfg.Routing.High
	case "medium":
		return e.cfg.Routing.Medium
	case "info":
		return e.cfg.Routing.Info
	default:
		return e.cfg.Routing.Low
	}
}

func (e *Engine) persist(a Alert) {
	meta, _ := json.Marshal(a.Meta)
	_, err := e.db.Exec(`
		INSERT INTO alerts(id, type, severity, title, summary, mac, fired_at)
		VALUES(?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Type, a.Severity, a.Title, a.Summary, a.MAC, a.FiredAt,
	)
	if err != nil {
		slog.Error("alert persist failed", "err", err, "meta", string(meta))
	}
}

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

// RecentAlerts returns the last N alerts for the web UI.
func (e *Engine) RecentAlerts(limit int) ([]Alert, error) {
	rows, err := e.db.Query(`
		SELECT id, type, severity, title, summary, mac, fired_at
		FROM alerts ORDER BY fired_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []Alert
	for rows.Next() {
		var a Alert
		var firedAt string
		if err := rows.Scan(&a.ID, &a.Type, &a.Severity, &a.Title, &a.Summary, &a.MAC, &firedAt); err != nil {
			continue
		}
		a.FiredAt = parseTime(firedAt)
		alerts = append(alerts, a)
	}
	return alerts, nil
}
