// Package ping implements an ARP poller that actively probes all known devices.
// ARP requests are layer-2 and bypass firewalls; replies are picked up by the
// passive ARP collector, which marks devices as seen. For devices outside the
// local subnet (force_ping=true), ICMP echo is used instead.
package ping

import (
	"context"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"

	"github.com/mdlayher/arp"
	"github.com/silentmap/silentmap/internal/bus"
	"github.com/silentmap/silentmap/internal/registry"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

type Collector struct {
	reg      *registry.Registry
	iface    string
	mu       sync.RWMutex
	enabled  bool
	interval time.Duration
	bus      *bus.Bus
}

func New(reg *registry.Registry, iface string, enabled bool, interval time.Duration) *Collector {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &Collector{
		reg:      reg,
		iface:    iface,
		enabled:  enabled,
		interval: interval,
	}
}

func (c *Collector) Name() string { return "arp-poller" }

func (c *Collector) Update(enabled bool, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	c.mu.Lock()
	c.enabled = enabled
	c.interval = interval
	c.mu.Unlock()
	slog.Info("arp poller updated", "enabled", enabled, "interval", interval)
}

func (c *Collector) IsEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.enabled
}

func (c *Collector) Settings() (enabled bool, interval time.Duration) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.enabled, c.interval
}

func (c *Collector) Start(ctx context.Context, b *bus.Bus) error {
	c.bus = b
	go c.run(ctx)
	slog.Info("arp poller started", "interface", c.iface)
	return nil
}

func (c *Collector) run(ctx context.Context) {
	c.mu.RLock()
	if c.enabled {
		c.mu.RUnlock()
		c.pollAll()
	} else {
		c.mu.RUnlock()
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	lastPoll := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.RLock()
			enabled := c.enabled
			interval := c.interval
			c.mu.RUnlock()
			if enabled && time.Since(lastPoll) >= interval {
				c.pollAll()
				lastPoll = time.Now()
			}
		}
	}
}

func (c *Collector) pollAll() {
	devices, err := c.reg.List()
	if err != nil || len(devices) == 0 {
		return
	}

	iface, err := net.InterfaceByName(c.iface)
	if err != nil {
		slog.Warn("arp poller: interface not found", "iface", c.iface, "err", err)
		return
	}
	arpClient, err := arp.Dial(iface)
	if err != nil {
		slog.Warn("arp poller: dial failed", "err", err)
		return
	}
	defer arpClient.Close()

	arpCount, pingCount := 0, 0
	for _, d := range devices {
		if d.IP == "" {
			continue
		}
		if d.ForcePing {
			// Device is outside the local subnet — use ICMP ping directly.
			// pingICMP publishes EventDeviceSeen on success.
			go c.pingICMP(d.MAC, d.IP)
			pingCount++
		} else {
			addr, err := netip.ParseAddr(d.IP)
			if err != nil || !addr.Is4() {
				continue
			}
			if err := arpClient.Request(addr); err != nil {
				slog.Debug("arp poller: request failed", "ip", d.IP, "err", err)
			}
			arpCount++
			time.Sleep(5 * time.Millisecond)
		}
	}
	slog.Debug("arp poller: done", "arp_requests", arpCount, "icmp_pings", pingCount)
}

// pingICMP sends an ICMP echo request and publishes EventDeviceSeen if the host replies.
func (c *Collector) pingICMP(mac, ip string) {
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		slog.Debug("icmp ping: listen failed", "ip", ip, "err", err)
		return
	}
	defer conn.Close()

	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID:   os.Getpid() & 0xffff,
			Seq:  1,
			Data: []byte("silentmap"),
		},
	}
	b, err := msg.Marshal(nil)
	if err != nil {
		return
	}

	dst := &net.IPAddr{IP: net.ParseIP(ip).To4()}
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.WriteTo(b, dst); err != nil {
		slog.Debug("icmp ping: write failed", "ip", ip, "err", err)
		return
	}

	rb := make([]byte, 1500)
	n, _, err := conn.ReadFrom(rb)
	if err != nil {
		return // timeout or no reply — device offline
	}

	rm, err := icmp.ParseMessage(1, rb[:n])
	if err != nil || rm.Type != ipv4.ICMPTypeEchoReply {
		return
	}

	// Host is reachable — publish seen event so registry marks it online.
	if c.bus != nil {
		c.bus.Publish(bus.NewEvent(bus.EventDeviceSeen, mac, ip, "icmp-ping", nil))
	}
}
