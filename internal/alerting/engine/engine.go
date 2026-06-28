// Package engine routes bus events to alert channels (Discord, ntfy) and
// persists fired alerts to the database for the alert history view.
package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/silentmap/silentmap/internal/alerting/channels"
	"github.com/silentmap/silentmap/internal/bus"
	"github.com/silentmap/silentmap/internal/config"
	"github.com/silentmap/silentmap/internal/timeutil"
)

// Alert re-exports channels.Alert for callers that only import engine.
type Alert = channels.Alert

// newDeviceBatchWindow is how long the engine waits before flushing a batch of
// new_device events. Events that arrive within this window are grouped into a
// single "N neue Geräte entdeckt" notification instead of N individual ones.
const newDeviceBatchWindow = 60 * time.Second

// newDeviceBatch accumulates new_device alerts before dispatching them.
type newDeviceBatch struct {
	mu      sync.Mutex
	pending []Alert
	timer   *time.Timer
	engine  *Engine
}

func (b *newDeviceBatch) add(a Alert) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pending = append(b.pending, a)
	if b.timer == nil {
		b.timer = time.AfterFunc(newDeviceBatchWindow, b.flush)
	}
}

func (b *newDeviceBatch) flush() {
	b.mu.Lock()
	alerts := b.pending
	b.pending = nil
	b.timer = nil
	b.mu.Unlock()

	if len(alerts) == 0 {
		return
	}
	if len(alerts) == 1 {
		b.engine.dispatchToChannels(alerts[0])
		return
	}
	batch := Alert{
		ID:       uuid.NewString(),
		Type:     "new_device_batch",
		Severity: alerts[0].Severity,
		Title:    "alert.title.new_device_batch",
		Summary:  fmt.Sprintf("%d neue Geräte entdeckt", len(alerts)),
		FiredAt:  time.Now(),
	}
	b.engine.persist(batch)
	b.engine.dispatchToChannels(batch)
	slog.Info("alert batch flushed", "count", len(alerts), "severity", batch.Severity)
}

type Engine struct {
	cfg              config.AlertsCfg
	db               *sql.DB
	channels         []channels.Channel
	cooldown         map[string]time.Time
	mu               sync.Mutex
	maintenanceUntil int64 // Unix timestamp; 0 = disabled; accessed via sync/atomic
	newDevBatch      newDeviceBatch
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
	e := &Engine{
		cfg:      cfg,
		db:       db,
		cooldown: make(map[string]time.Time),
	}
	e.newDevBatch.engine = e
	go e.runCooldownCleanup()
	return e
}

// runCooldownCleanup periodically removes expired entries from the cooldown map
// so it does not grow unboundedly over long uptimes.
func (e *Engine) runCooldownCleanup() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		e.mu.Lock()
		for key, t := range e.cooldown {
			if now.Sub(t) > time.Hour {
				delete(e.cooldown, key)
			}
		}
		e.mu.Unlock()
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
	if v := atomic.LoadInt64(&e.maintenanceUntil); v > 0 && time.Now().Unix() < v {
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

	a := Alert{
		ID:       uuid.NewString(),
		Type:     "new_device",
		Severity: e.cfg.Rules.NewDevice.Severity,
		Title:    "alert.title.new_device",
		Summary:  summary,
		MAC:      ev.MAC,
		IP:       ev.IP,
		FiredAt:  time.Now(),
		Meta:     ev.Meta,
	}
	e.persist(a)
	// Route through batcher so boot-time floods become a single summary notification.
	e.newDevBatch.add(a)
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
	e.dispatchToChannels(a)
}

// dispatchToChannels sends a ready-to-send alert to all enabled channels
// that are configured for the alert's severity routing.
func (e *Engine) dispatchToChannels(a Alert) {
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
		a.FiredAt = timeutil.ParseSQLiteTime(firedAt)
		alerts = append(alerts, a)
	}
	return alerts, nil
}
