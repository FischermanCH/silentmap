package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/silentmap/silentmap/internal/alerting/channels"
	"github.com/silentmap/silentmap/internal/bus"
	"github.com/silentmap/silentmap/internal/config"
)

// Alert re-exports channels.Alert for callers that only import engine.
type Alert = channels.Alert

type Engine struct {
	cfg      config.AlertsCfg
	db       *sql.DB
	channels []channels.Channel
	cooldown map[string]time.Time
	mu       sync.Mutex
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
		Title:    "Neues Gerät erkannt",
		Summary:  summary,
		MAC:      ev.MAC,
		Meta:     ev.Meta,
	}, e.cfg.Rules.NewDevice.Cooldown)
}

func (e *Engine) onDeviceLost(ev bus.Event) {
	if !e.cfg.Rules.PriorityOffline.Enabled {
		return
	}
	priority, _ := ev.Meta["priority"].(bool)
	if !priority {
		return
	}
	label, _ := ev.Meta["label"].(string)
	display := label
	if display == "" {
		display = ev.MAC
	}

	e.fire(Alert{
		Type:     "priority_offline",
		Severity: e.cfg.Rules.PriorityOffline.Severity,
		Title:    "Prioritäts-Gerät offline",
		Summary:  display + " ist nicht mehr erreichbar (letzte IP: " + ev.IP + ")",
		MAC:      ev.MAC,
		Meta:     ev.Meta,
	}, e.cfg.Rules.PriorityOffline.Cooldown)
}

func (e *Engine) onDeviceBack(ev bus.Event) {
	if !e.cfg.Rules.DeviceBack.Enabled {
		return
	}
	label, _ := ev.Meta["label"].(string)
	display := label
	if display == "" {
		display = ev.MAC
	}

	e.fire(Alert{
		Type:     "device_back",
		Severity: e.cfg.Rules.DeviceBack.Severity,
		Title:    "Gerät wieder online",
		Summary:  display + " ist wieder erreichbar unter " + ev.IP,
		MAC:      ev.MAC,
		Meta:     ev.Meta,
	}, e.cfg.Rules.DeviceBack.Cooldown)
}

func (e *Engine) fire(a Alert, cooldownDur time.Duration) {
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
