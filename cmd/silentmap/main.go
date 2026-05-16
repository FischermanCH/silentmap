package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/silentmap/silentmap/internal/alerting/channels/ntfy"
	"github.com/silentmap/silentmap/internal/alerting/engine"
	"github.com/silentmap/silentmap/internal/bus"
	"github.com/silentmap/silentmap/internal/collectors/arp"
	"github.com/silentmap/silentmap/internal/collectors/mdns"
	"github.com/silentmap/silentmap/internal/config"
	"github.com/silentmap/silentmap/internal/registry"
	"github.com/silentmap/silentmap/internal/web"
	_ "modernc.org/sqlite"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	var (
		flagConfig    = flag.String("config", "", "path to config file (default: <data>/silentmap.yaml)")
		flagInterface = flag.String("interface", "", "network interface to listen on")
		flagData      = flag.String("data", "/data", "data directory for SQLite and models")
		flagDebug     = flag.Bool("debug", false, "enable debug logging")
		flagVersion   = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *flagVersion {
		fmt.Printf("silentmap %s (%s)\n", version, commit)
		os.Exit(0)
	}

	level := slog.LevelInfo
	if *flagDebug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	// Config
	cfgPath := *flagConfig
	if cfgPath == "" {
		cfgPath = filepath.Join(*flagData, "silentmap.yaml")
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		slog.Error("config load failed", "path", cfgPath, "err", err)
		os.Exit(1)
	}
	cfg.DataDir = *flagData
	if *flagInterface != "" {
		cfg.Interface = *flagInterface
	}

	// Auto-detect interface
	if cfg.Interface == "" {
		iface, err := detectInterface()
		if err != nil {
			slog.Error("could not detect network interface", "err", err)
			os.Exit(1)
		}
		cfg.Interface = iface
		slog.Info("auto-detected interface", "interface", iface)
	}

	// Data directory
	if err := os.MkdirAll(*flagData, 0750); err != nil {
		slog.Error("cannot create data dir", "path", *flagData, "err", err)
		os.Exit(1)
	}

	// SQLite
	dbPath := filepath.Join(*flagData, "silentmap.db")
	db, err := sql.Open("sqlite", dbPath+"?_journal=WAL&_timeout=5000")
	if err != nil {
		slog.Error("cannot open database", "path", dbPath, "err", err)
		os.Exit(1)
	}
	defer db.Close()

	// Core components
	b := bus.New()

	reg, err := registry.New(db, b, cfg.Collectors.ARP.OfflineTimeout)
	if err != nil {
		slog.Error("registry init failed", "err", err)
		os.Exit(1)
	}

	alertEngine := engine.New(cfg.Alerts, db)
	alertEngine.Register(ntfy.New(cfg.Alerts.Channels.Ntfy))
	alertEngine.Subscribe(b)

	webServer := web.NewServer(reg, alertEngine, db)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Start offline checker
	go reg.RunOfflineChecker(ctx)

	// Start ARP collector (non-fatal — needs CAP_NET_RAW or root)
	if cfg.Collectors.ARP.Enabled {
		arpCollector := arp.New(cfg.Interface)
		if err := arpCollector.Start(ctx, b); err != nil {
			slog.Warn("arp collector could not start — run as root or with CAP_NET_RAW for passive discovery",
				"err", err)
		} else {
			defer arpCollector.Stop()
		}
	}

	// Start mDNS collector (non-fatal)
	if cfg.Collectors.MDNS.Enabled {
		mdnsCollector := mdns.New(cfg.Interface)
		if err := mdnsCollector.Start(ctx, b); err != nil {
			slog.Warn("mdns collector could not start", "err", err)
		} else {
			defer mdnsCollector.Stop()
		}
	}

	// Start HTTP server
	srv := &http.Server{
		Addr:    cfg.Web.Listen,
		Handler: webServer.Handler(),
	}
	go func() {
		slog.Info("web UI listening", "addr", cfg.Web.Listen)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("web server error", "err", err)
		}
	}()

	slog.Info("silentmap started", "version", version, "interface", cfg.Interface)

	<-ctx.Done()
	slog.Info("shutting down...")
	srv.Close()
}

// detectInterface returns the first non-loopback interface with an IPv4 address.
func detectInterface() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.To4() != nil {
				return iface.Name, nil
			}
		}
	}
	return "", fmt.Errorf("no suitable network interface found")
}
