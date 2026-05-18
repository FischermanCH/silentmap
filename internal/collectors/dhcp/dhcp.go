package dhcp

import (
	"context"
	"encoding/binary"
	"log/slog"
	"net"
	"strings"

	"github.com/silentmap/silentmap/internal/bus"
)

// Collector passively sniffs DHCP Discover/Request/Inform packets
// using a raw IPv4 socket (requires CAP_NET_RAW).
type Collector struct {
	iface string
	conn  net.PacketConn
}

func New(iface string) *Collector {
	return &Collector{iface: iface}
}

func (c *Collector) Name() string { return "dhcp" }

func (c *Collector) Start(ctx context.Context, b *bus.Bus) error {
	// Raw IPv4 socket — captures all UDP, we filter port 67 in software
	conn, err := net.ListenPacket("ip4:udp", "0.0.0.0")
	if err != nil {
		return err
	}
	c.conn = conn
	slog.Info("dhcp collector started", "interface", c.iface)

	go func() {
		defer conn.Close()
		buf := make([]byte, 65536)
		for {
			if ctx.Err() != nil {
				return
			}
			n, _, err := conn.ReadFrom(buf)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				continue
			}
			c.process(buf[:n], b)
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

func (c *Collector) process(pkt []byte, b *bus.Bus) {
	// Parse IP header to get to UDP payload
	if len(pkt) < 20 {
		return
	}
	ihl := int(pkt[0]&0x0f) * 4
	if len(pkt) < ihl+8 {
		return
	}

	// UDP header
	udp := pkt[ihl:]
	srcPort := binary.BigEndian.Uint16(udp[0:2])
	dstPort := binary.BigEndian.Uint16(udp[2:4])

	// Only DHCP client→server (68→67) or server→client (67→68)
	if !((srcPort == 68 && dstPort == 67) || (srcPort == 67 && dstPort == 68)) {
		return
	}

	// DHCP payload starts after UDP header (8 bytes)
	if len(udp) < 8+240 {
		return
	}
	dhcp := udp[8:]

	// op=1 means client request (DISCOVER, REQUEST, INFORM)
	if dhcp[0] != 1 {
		return
	}
	// htype=1 ethernet, hlen=6
	if dhcp[1] != 1 || dhcp[2] != 6 {
		return
	}

	// chaddr: client hardware address (offset 28, 6 bytes for ethernet)
	mac := net.HardwareAddr(dhcp[28:34]).String()
	if mac == "00:00:00:00:00:00" {
		return
	}

	// ciaddr: client IP (offset 12)
	ip := net.IP(dhcp[12:16]).String()
	if ip == "0.0.0.0" {
		ip = ""
	}

	// Parse DHCP options (magic cookie at offset 236, options start at 240)
	if binary.BigEndian.Uint32(dhcp[236:240]) != 0x63825363 {
		return // not a valid DHCP packet
	}

	hostname, msgType := parseDHCPOptions(dhcp[240:])

	// Only care about client-to-server messages (not offers/acks)
	if msgType != 0 && msgType != 1 && msgType != 3 && msgType != 8 {
		// 1=DISCOVER, 3=REQUEST, 8=INFORM
		return
	}

	meta := map[string]any{"dhcp_type": msgType}
	if hostname != "" {
		meta["hostname"] = strings.TrimSuffix(hostname, ".")
	}

	b.Publish(bus.NewEvent(bus.EventDeviceSeen, mac, ip, "dhcp", meta))
}

// parseDHCPOptions extracts hostname (option 12) and message type (option 53).
func parseDHCPOptions(opts []byte) (hostname string, msgType byte) {
	i := 0
	for i < len(opts) {
		code := opts[i]
		if code == 255 { // END
			break
		}
		if code == 0 { // PAD
			i++
			continue
		}
		if i+1 >= len(opts) {
			break
		}
		length := int(opts[i+1])
		if i+2+length > len(opts) {
			break
		}
		val := opts[i+2 : i+2+length]
		switch code {
		case 12: // Hostname
			hostname = string(val)
		case 53: // DHCP Message Type
			if length == 1 {
				msgType = val[0]
			}
		}
		i += 2 + length
	}
	return
}
