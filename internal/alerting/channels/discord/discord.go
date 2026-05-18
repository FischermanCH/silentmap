// Package discord sends alert notifications to a Discord channel via webhook.
package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/silentmap/silentmap/internal/alerting/channels"
	"github.com/silentmap/silentmap/internal/config"
)

type Channel struct {
	mu     sync.RWMutex
	cfg    config.DiscordCfg
	client *http.Client
}

func New(cfg config.DiscordCfg) *Channel {
	return &Channel{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Channel) Name() string { return "discord" }

func (c *Channel) Enabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cfg.Enabled && c.cfg.WebhookURL != ""
}

func (c *Channel) Update(cfg config.DiscordCfg) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg = cfg
}

func (c *Channel) Send(ctx context.Context, a channels.Alert) error {
	c.mu.RLock()
	webhookURL := c.cfg.WebhookURL
	c.mu.RUnlock()

	name := displayName(a)

	embed := map[string]any{
		"title":       buildTitle(a, name),
		"description": buildDescription(a, name),
		"color":       severityColor(a.Severity),
		"fields":      buildFields(a),
		"footer":      map[string]any{"text": "silentmap"},
		"timestamp":   a.FiredAt.UTC().Format(time.RFC3339),
	}

	payload, err := json.Marshal(map[string]any{
		"username": "silentmap",
		"embeds":   []any{embed},
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("discord: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// displayName returns the best human-readable name for the device in an alert.
// Priority: label → hostname (manual) → hostnameAuto → vendor → MAC
func displayName(a channels.Alert) string {
	str := func(key string) string {
		v, _ := a.Meta[key].(string)
		return v
	}
	if v := str("label"); v != "" {
		return v
	}
	if v := str("hostname"); v != "" {
		return v
	}
	if v := str("hostnameAuto"); v != "" {
		return v
	}
	if v := str("vendor"); v != "" {
		// Vendor can be long — take first two words max
		parts := strings.Fields(v)
		if len(parts) > 2 {
			parts = parts[:2]
		}
		return strings.Join(parts, " ") + " · " + a.IP
	}
	return a.MAC
}

func buildTitle(a channels.Alert, name string) string {
	named := name != a.MAC // only use name in title if it's more than a MAC
	switch a.Type {
	case "new_device":
		if named {
			return "🆕 Neues Gerät: " + name
		}
		return "🆕 Neues Gerät erkannt"
	case "priority_offline":
		if named {
			return "🔴 " + name + " ist offline"
		}
		return "🔴 Prioritäts-Gerät offline"
	case "device_back":
		if named {
			return "🟢 " + name + " ist wieder online"
		}
		return "🟢 Gerät wieder online"
	default:
		return "ℹ️ " + a.Title
	}
}

func buildDescription(a channels.Alert, name string) string {
	str := func(key string) string {
		v, _ := a.Meta[key].(string)
		return v
	}
	cat := str("category")
	groups := str("groups")

	var parts []string
	if a.IP != "" {
		parts = append(parts, "`"+a.IP+"`")
	}
	if cat != "" {
		parts = append(parts, cat)
	}
	if groups != "" {
		parts = append(parts, "📁 "+groups)
	}

	switch a.Type {
	case "priority_offline":
		if ls := str("lastSeen"); ls != "" {
			parts = append(parts, "zuletzt "+ls)
		}
	case "device_back":
		if ls := str("lastSeen"); ls != "" {
			parts = append(parts, "war offline seit "+ls)
		}
	case "new_device":
		if name == a.MAC {
			parts = append(parts, "noch kein Name vergeben")
		}
	}

	return strings.Join(parts, "  ·  ")
}

func buildFields(a channels.Alert) []map[string]any {
	str := func(key string) string {
		v, _ := a.Meta[key].(string)
		return v
	}

	hostname := str("hostname")
	if hostname == "" {
		hostname = str("hostnameAuto")
	}
	vendor := str("vendor")
	category := str("category")
	groups := str("groups")
	lastSeen := str("lastSeen")
	firstSeen := str("firstSeen")

	fields := []map[string]any{}
	field := func(name, value string, inline bool) {
		if value == "" {
			return
		}
		fields = append(fields, map[string]any{
			"name":   name,
			"value":  value,
			"inline": inline,
		})
	}

	// Row 1: Hostname + IP
	field("Hostname", hostname, true)
	field("IP", a.IP, true)

	// Row 2: Kategorie + Hersteller
	field("Kategorie", category, true)
	field("Hersteller", vendor, true)

	// Row 3: Gruppe + MAC
	field("Gruppe", groups, true)
	field("MAC", a.MAC, true)

	// Row 4: Zeitinfo
	switch a.Type {
	case "new_device":
		field("Erstmals gesehen", firstSeen, false)
	case "priority_offline":
		field("Zuletzt gesehen", lastSeen, false)
	case "device_back":
		field("Offline seit", lastSeen, false)
	}

	return fields
}

func severityColor(severity string) int {
	switch severity {
	case "critical":
		return 0xE74C3C
	case "high":
		return 0xE67E22
	case "medium":
		return 0xF1C40F
	case "info":
		return 0x3498DB
	default:
		return 0x95A5A6
	}
}
