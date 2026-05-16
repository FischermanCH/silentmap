package arp

import (
	"context"
	"log/slog"
	"net"
	"time"

	"github.com/mdlayher/arp"
	"github.com/silentmap/silentmap/internal/bus"
)

type Collector struct {
	iface  string
	client *arp.Client
}

func New(iface string) *Collector {
	return &Collector{iface: iface}
}

func (c *Collector) Name() string { return "arp" }

func (c *Collector) Start(ctx context.Context, b *bus.Bus) error {
	iface, err := net.InterfaceByName(c.iface)
	if err != nil {
		return err
	}

	client, err := arp.Dial(iface)
	if err != nil {
		return err
	}
	c.client = client

	slog.Info("arp collector started", "interface", c.iface)

	go func() {
		defer client.Close()
		for {
			if ctx.Err() != nil {
				return
			}
			// Unblock every second so we can check ctx.Done
			client.SetReadDeadline(time.Now().Add(time.Second))

			pkt, _, err := client.Read()
			if err != nil {
				// Timeout is expected — just loop
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				if ctx.Err() != nil {
					return
				}
				slog.Debug("arp read error", "err", err)
				continue
			}

			mac := pkt.SenderHardwareAddr.String()
			ip := pkt.SenderIP.String()

			// Ignore zero/broadcast
			if mac == "00:00:00:00:00:00" || mac == "ff:ff:ff:ff:ff:ff" {
				continue
			}
			if ip == "0.0.0.0" {
				continue
			}

			b.Publish(bus.NewEvent(bus.EventDeviceSeen, mac, ip, "arp", map[string]any{
				"target_ip": pkt.TargetIP.String(),
			}))
		}
	}()
	return nil
}

func (c *Collector) Stop() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}
