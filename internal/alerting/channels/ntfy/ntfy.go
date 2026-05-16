package ntfy

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/silentmap/silentmap/internal/alerting/channels"
	"github.com/silentmap/silentmap/internal/config"
)

type Channel struct {
	cfg    config.NtfyCfg
	client *http.Client
}

func New(cfg config.NtfyCfg) *Channel {
	return &Channel{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Channel) Name() string    { return "ntfy" }
func (c *Channel) Enabled() bool   { return c.cfg.Enabled && c.cfg.URL != "" }

func (c *Channel) Send(ctx context.Context, a channels.Alert) error {
	body := a.Summary
	if a.MAC != "" {
		body += "\nMAC: " + a.MAC
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.URL, strings.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Title", a.Title)
	req.Header.Set("Priority", ntfyPriority(a.Severity))
	req.Header.Set("Tags", ntfyTags(a.Type))
	req.Header.Set("Content-Type", "text/plain")

	if c.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
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
