package mdns

import (
	"context"
	"log/slog"
	"net"
	"strings"

	"github.com/miekg/dns"
	"github.com/silentmap/silentmap/internal/bus"
	"golang.org/x/net/ipv4"
)

const mdnsAddr = "224.0.0.251:5353"

type Collector struct {
	iface string
	conn  *net.UDPConn
}

func New(iface string) *Collector {
	return &Collector{iface: iface}
}

func (c *Collector) Name() string { return "mdns" }

func (c *Collector) Start(ctx context.Context, b *bus.Bus) error {
	iface, err := net.InterfaceByName(c.iface)
	if err != nil {
		return err
	}

	addr, err := net.ResolveUDPAddr("udp4", mdnsAddr)
	if err != nil {
		return err
	}

	conn, err := net.ListenMulticastUDP("udp4", iface, addr)
	if err != nil {
		return err
	}
	c.conn = conn

	// Larger read buffer for busy networks
	if err := ipv4.NewPacketConn(conn).SetControlMessage(ipv4.FlagSrc, true); err != nil {
		slog.Debug("mdns: could not set control message", "err", err)
	}

	slog.Info("mdns collector started", "interface", c.iface)

	go func() {
		defer conn.Close()
		buf := make([]byte, 65536)
		for {
			if ctx.Err() != nil {
				return
			}
			n, src, err := conn.ReadFromUDP(buf)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				slog.Debug("mdns read error", "err", err)
				continue
			}
			msg := new(dns.Msg)
			if err := msg.Unpack(buf[:n]); err != nil {
				continue
			}
			c.process(msg, src.IP, b)
		}
	}()
	return nil
}

func (c *Collector) Stop() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Collector) process(msg *dns.Msg, srcIP net.IP, b *bus.Bus) {
	meta := map[string]any{}

	// Collect from all sections
	records := append(append(msg.Answer, msg.Ns...), msg.Extra...)

	var hostname string
	var services []string
	var ip string

	if srcIP != nil && !srcIP.IsLinkLocalMulticast() {
		ip = srcIP.String()
	}

	for _, rr := range records {
		switch r := rr.(type) {
		case *dns.A:
			if ip == "" {
				ip = r.A.String()
			}
			if hostname == "" {
				hostname = strings.TrimSuffix(r.Hdr.Name, ".")
			}

		case *dns.PTR:
			name := strings.TrimSuffix(r.Ptr, ".")
			// PTR für Hostnamen (z.B. _services._dns-sd)
			if !strings.HasPrefix(r.Hdr.Name, "_") {
				if hostname == "" {
					hostname = strings.TrimSuffix(r.Hdr.Name, ".")
				}
			}
			// Service Discovery (z.B. _airplay._tcp.local)
			if strings.Contains(r.Hdr.Name, "._tcp.") || strings.Contains(r.Hdr.Name, "._udp.") {
				svc := extractService(r.Hdr.Name)
				if svc != "" && !contains(services, svc) {
					services = append(services, svc)
				}
			}
			_ = name

		case *dns.SRV:
			svc := extractService(r.Hdr.Name)
			if svc != "" && !contains(services, svc) {
				services = append(services, svc)
			}
			if hostname == "" && r.Target != "" {
				hostname = strings.TrimSuffix(r.Target, ".")
			}

		case *dns.TXT:
			if len(r.Txt) > 0 {
				meta["txt"] = r.Txt
			}
		}
	}

	if ip == "" && hostname == "" {
		return
	}

	// Clean up .local suffix for display
	hostname = strings.TrimSuffix(hostname, ".local")

	if hostname != "" {
		meta["hostname"] = hostname
	}
	if len(services) > 0 {
		meta["services"] = services
	}

	b.Publish(bus.NewEvent(bus.EventDeviceSeen, "", ip, "mdns", meta))
}

func extractService(name string) string {
	// "_airplay._tcp.local." → "_airplay._tcp"
	name = strings.TrimSuffix(name, ".")
	parts := strings.Split(name, ".")
	for i, p := range parts {
		if p == "_tcp" || p == "_udp" {
			if i > 0 {
				return strings.Join(parts[i-1:i+1], ".")
			}
		}
	}
	return ""
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
