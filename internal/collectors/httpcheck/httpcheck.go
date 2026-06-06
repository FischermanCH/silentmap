// Package httpcheck implements a periodic HTTP/HTTPS availability checker.
// It is opt-in: the collector only probes devices that have an http_url set.
// Any HTTP response (including 4xx/5xx) counts as "online"; only timeouts and
// connection failures are treated as unavailable.
package httpcheck

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/silentmap/silentmap/internal/bus"
	"github.com/silentmap/silentmap/internal/registry"
)

// maxConsecutiveFailures is the number of failed HTTP checks before a device
// is explicitly marked offline (independent of the ARP offline timeout).
const maxConsecutiveFailures = 3

type Collector struct {
	reg      *registry.Registry
	mu       sync.RWMutex
	enabled  bool
	interval time.Duration
	b        *bus.Bus
	failMu   sync.Mutex
	failures map[string]int // mac → consecutive failure count
}

func New(reg *registry.Registry, enabled bool, interval time.Duration) *Collector {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &Collector{
		reg:      reg,
		enabled:  enabled,
		interval: interval,
		failures: make(map[string]int),
	}
}

func (c *Collector) Name() string { return "http-checker" }

func (c *Collector) Update(enabled bool, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	c.mu.Lock()
	c.enabled = enabled
	c.interval = interval
	c.mu.Unlock()
	slog.Info("http checker updated", "enabled", enabled, "interval", interval)
}

func (c *Collector) Settings() (enabled bool, interval time.Duration) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.enabled, c.interval
}

func (c *Collector) Start(ctx context.Context, b *bus.Bus) error {
	c.b = b
	go c.run(ctx)
	slog.Info("http checker started")
	return nil
}

func (c *Collector) run(ctx context.Context) {
	c.mu.RLock()
	if c.enabled {
		c.mu.RUnlock()
		c.checkAll()
	} else {
		c.mu.RUnlock()
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	lastCheck := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.RLock()
			enabled := c.enabled
			interval := c.interval
			c.mu.RUnlock()
			if enabled && time.Since(lastCheck) >= interval {
				c.checkAll()
				lastCheck = time.Now()
			}
		}
	}
}

func (c *Collector) checkAll() {
	devices := c.reg.HttpServiceDevices()
	if len(devices) == 0 {
		return
	}

	// Reusable HTTP client: 10 s timeout, follows redirects, ignores TLS errors
	// (self-signed certs are common on home networks).
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}

	ok, fail := 0, 0
	for _, d := range devices {
		if c.probe(client, d.MAC, d.IP, d.HttpURL) {
			ok++
		} else {
			fail++
		}
	}
	slog.Debug("http checker: done", "ok", ok, "fail", fail)
}

// probe performs a single HTTP GET. Returns true if the server responded.
// Tracks consecutive failures; after maxConsecutiveFailures the device is
// explicitly marked offline so the alert fires regardless of the ARP timeout.
func (c *Collector) probe(client *http.Client, mac, ip, rawURL string) bool {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		slog.Debug("http checker: bad url", "url", rawURL, "err", err)
		return false
	}
	req.Header.Set("User-Agent", "silentmap-healthcheck/1.0")

	resp, err := client.Do(req)
	if err != nil {
		slog.Debug("http checker: no response", "mac", mac, "url", rawURL, "err", err)
		c.failMu.Lock()
		c.failures[mac]++
		fails := c.failures[mac]
		c.failMu.Unlock()
		if fails >= maxConsecutiveFailures {
			c.reg.MarkHttpServiceOffline(mac, ip)
		}
		return false
	}
	resp.Body.Close()

	// Any HTTP response (including 4xx/5xx) means the service is reachable.
	c.failMu.Lock()
	c.failures[mac] = 0
	c.failMu.Unlock()

	if c.b != nil {
		c.b.Publish(bus.NewEvent(bus.EventDeviceSeen, mac, ip, "http-check", nil))
	}
	slog.Debug("http checker: ok", "mac", mac, "url", rawURL, "status", resp.StatusCode)
	return true
}
