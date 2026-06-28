// Package webhook sends alert notifications to an arbitrary HTTP endpoint.
// The payload is a JSON object with the full alert data; the URL and optional
// headers are configurable in Settings → Webhook.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/silentmap/silentmap/internal/alerting/channels"
	"github.com/silentmap/silentmap/internal/config"
)

type Channel struct {
	mu     sync.RWMutex
	cfg    config.WebhookCfg
	client *http.Client
}

func New(cfg config.WebhookCfg) *Channel {
	return &Channel{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Channel) Name() string { return "webhook" }

func (c *Channel) Enabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cfg.Enabled && c.cfg.URL != ""
}

func (c *Channel) Update(cfg config.WebhookCfg) {
	c.mu.Lock()
	c.cfg = cfg
	c.mu.Unlock()
}

// payload is the JSON body posted to the webhook endpoint.
type payload struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Severity string         `json:"severity"`
	Title    string         `json:"title"`
	Summary  string         `json:"summary"`
	MAC      string         `json:"mac"`
	IP       string         `json:"ip"`
	FiredAt  string         `json:"fired_at"`
	Meta     map[string]any `json:"meta,omitempty"`
}

func (c *Channel) Send(ctx context.Context, a channels.Alert) error {
	c.mu.RLock()
	url := c.cfg.URL
	method := c.cfg.Method
	headers := c.cfg.Headers
	c.mu.RUnlock()

	if method == "" {
		method = http.MethodPost
	}

	body, err := json.Marshal(payload{
		ID:       a.ID,
		Type:     a.Type,
		Severity: a.Severity,
		Title:    a.Title,
		Summary:  a.Summary,
		MAC:      a.MAC,
		IP:       a.IP,
		FiredAt:  a.FiredAt.UTC().Format(time.RFC3339),
		Meta:     a.Meta,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook: unexpected status %d", resp.StatusCode)
	}
	return nil
}
