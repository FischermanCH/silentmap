// Package ntfy sends alerts to a ntfy.sh topic (or self-hosted ntfy server).
package ntfy

import (
	"context"
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
	cfg    config.NtfyCfg
	client *http.Client
}

func New(cfg config.NtfyCfg) *Channel {
	return &Channel{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Channel) Update(cfg config.NtfyCfg) {
	c.mu.Lock()
	c.cfg = cfg
	c.mu.Unlock()
}

func (c *Channel) Name() string { return "ntfy" }
func (c *Channel) Enabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cfg.Enabled && c.cfg.URL != ""
}

func (c *Channel) Send(ctx context.Context, a channels.Alert) error {
	c.mu.RLock()
	url := c.cfg.URL
	token := c.cfg.Token
	c.mu.RUnlock()

	body := a.Summary
	if a.MAC != "" {
		body += "\nMAC: " + a.MAC
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Title", a.Title)
	req.Header.Set("Priority", ntfyPriority(a.Severity))
	req.Header.Set("Tags", ntfyTags(a.Type))
	req.Header.Set("Content-Type", "text/plain")

	if token != "" {
		// Strip any CR/LF characters to prevent HTTP header injection.
		token = strings.Map(func(r rune) rune {
			if r == '\r' || r == '\n' {
				return -1
			}
			return r
		}, token)
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func ntfyPriority(severity string) string {
	switch severity {
	case "critical":
		return "5"
	case "high":
		return "4"
	case "medium":
		return "3"
	case "info":
		return "2"
	default:
		return "1"
	}
}

func ntfyTags(alertType string) string {
	switch alertType {
	case "new_device":
		return "new,computer"
	case "priority_offline":
		return "warning,rotating_light"
	case "device_back":
		return "white_check_mark"
	case "anomaly":
		return "eyes"
	default:
		return "bell"
	}
}
