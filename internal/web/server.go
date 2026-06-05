package web

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/silentmap/silentmap/internal/alerting/channels/discord"
	"github.com/silentmap/silentmap/internal/alerting/channels/ntfy"
	"github.com/silentmap/silentmap/internal/alerting/engine"
	"github.com/silentmap/silentmap/internal/collectors/ping"
	"github.com/silentmap/silentmap/internal/config"
	"github.com/silentmap/silentmap/internal/i18n"
	"github.com/silentmap/silentmap/internal/registry"
	"github.com/silentmap/silentmap/internal/scanner"
)

type DiscordSettings struct {
	Enabled    bool   `json:"enabled"`
	WebhookURL string `json:"webhook_url"`
}

type NtfySettings struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
	Token   string `json:"token"`
}

type PingSettings struct {
	Enabled  bool `json:"enabled"`
	Interval int  `json:"interval_min"` // minutes
}

type AppSettings struct {
	Discord   DiscordSettings `json:"discord"`
	Ntfy      NtfySettings    `json:"ntfy"`
	Ping      PingSettings    `json:"ping"`
	Listening bool            `json:"listening"`
}

//go:embed templates/*
//go:embed static/*
var templateFS embed.FS

type Server struct {
	reg       *registry.Registry
	alertEng  *engine.Engine
	db        *sql.DB
	themes    *ThemeManager
	bundle    *i18n.Bundle
	funcMap   template.FuncMap
	nmapArgs  string
	dataDir   string
	discordCh *discord.Channel
	ntfyCh    *ntfy.Channel
	pingCol   *ping.Collector
	version   string
	buildTime string
	scanMu    sync.RWMutex
	scanning  map[string]bool // mac → scan in progress

	latestVersion   string
	latestVersionMu sync.RWMutex
}

func NewServer(reg *registry.Registry, alertEng *engine.Engine, db *sql.DB, dataDir string, nmapArgs string, discordCh *discord.Channel, ntfyCh *ntfy.Channel, pingCol *ping.Collector, version, buildTime string) *Server {
	bundle, err := i18n.New()
	if err != nil {
		slog.Warn("i18n: failed to load translations, using keys as fallback", "err", err)
		bundle, _ = i18n.New() // still returns usable empty bundle
	}
	s := &Server{
		reg:       reg,
		bundle:    bundle,
		alertEng:  alertEng,
		db:        db,
		themes:    NewThemeManager(dataDir),
		nmapArgs:  nmapArgs,
		dataDir:   dataDir,
		discordCh: discordCh,
		ntfyCh:    ntfyCh,
		pingCol:   pingCol,
		version:   version,
		scanning:  make(map[string]bool),
		buildTime: buildTime,
		funcMap: template.FuncMap{
			// t/tf/timeAgo are injected per-request in render() with lang baked in
			"severityClass":  severityClass,
			"eventIcon":      eventIcon,
			"eventColor":     eventColor,
			"friendlySource": friendlySource,
			"slice":          func(args ...string) []string { return args },
			"catIcon":        catIconSVG,
			"catColor":       catColorHex,
			"replace":        strings.ReplaceAll,
		},
	}
	// Apply persisted channel settings over yaml defaults
	stored := s.loadAppSettings()
	if stored.Discord.WebhookURL != "" || stored.Discord.Enabled {
		s.discordCh.Update(config.DiscordCfg{
			Enabled:    stored.Discord.Enabled,
			WebhookURL: stored.Discord.WebhookURL,
		})
	}
	if stored.Ntfy.URL != "" || stored.Ntfy.Enabled {
		s.ntfyCh.Update(config.NtfyCfg{
			Enabled: stored.Ntfy.Enabled,
			URL:     stored.Ntfy.URL,
			Token:   stored.Ntfy.Token,
		})
	}
	s.reg.SetListening(stored.Listening)
	// Only apply stored ping settings if the user has explicitly saved them (Interval > 0).
	// Otherwise keep the constructor default (enabled=true, 5min).
	if stored.Ping.Interval > 0 {
		s.pingCol.Update(stored.Ping.Enabled, time.Duration(stored.Ping.Interval)*time.Minute)
	}
	return s
}

func (s *Server) StartBackground(ctx context.Context) {
	go func() {
		s.checkLatestVersion()
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.checkLatestVersion()
			}
		}
	}()
}

func (s *Server) checkLatestVersion() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/repos/FischermanCH/silentmap/releases/latest", nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "silentmap/"+s.version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var result struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || result.TagName == "" {
		return
	}
	s.latestVersionMu.Lock()
	s.latestVersion = result.TagName
	s.latestVersionMu.Unlock()
}

func (s *Server) updateAvailable() (bool, string) {
	s.latestVersionMu.RLock()
	latest := s.latestVersion
	s.latestVersionMu.RUnlock()
	if latest == "" {
		return false, ""
	}
	current := s.version
	if !strings.HasPrefix(current, "v") {
		current = "v" + current
	}
	return latest != current, latest
}

func (s *Server) loadAppSettings() AppSettings {
	data, err := os.ReadFile(filepath.Join(s.dataDir, "settings.json"))
	if err != nil {
		return AppSettings{}
	}
	var cfg AppSettings
	if err := json.Unmarshal(data, &cfg); err != nil {
		slog.Warn("settings: failed to parse settings.json", "err", err)
	}
	return cfg
}

func (s *Server) saveAppSettings(cfg AppSettings) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dataDir, "settings.json"), data, 0644)
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/", s.dashboard)
	r.Get("/devices", s.deviceList)
	r.Get("/devices/{mac}", s.deviceDetail)
	r.Post("/devices/{mac}/label", s.setLabel)
	r.Post("/devices/{mac}/hostname", s.setHostname)
	r.Post("/devices/{mac}/priority", s.setPriority)
	r.Post("/devices/{mac}/force-ping", s.setForcePing)
	r.Post("/devices/{mac}/approve", s.approveDevice)
	r.Post("/devices/{mac}/category", s.setCategory)
	r.Get("/alerts", s.alertList)
	r.Get("/log", s.eventLog)
	r.Post("/devices/new", s.createDevice)
	r.Post("/devices/{mac}/delete", s.deleteDevice)
	r.Post("/devices/{mac}/connections", s.addConnection)
	r.Post("/devices/{mac}/connections/{id}/delete", s.removeConnection)
	r.Get("/groups", s.groupList)
	r.Post("/groups", s.createGroup)
	r.Post("/groups/{id}/update", s.updateGroup)
	r.Post("/groups/{id}/delete", s.deleteGroup)
	r.Post("/groups/{id}/move", s.moveGroup)
	r.Post("/devices/{mac}/groups", s.addDeviceToGroup)
	r.Post("/devices/{mac}/groups/{groupId}/remove", s.removeDeviceFromGroup)
	r.Get("/api/topology", s.apiTopology)
	r.Get("/api/export", s.exportDevices)
	r.Post("/api/import", s.importDevices)
	r.Post("/devices/{mac}/nmap", s.runNmap)
	r.Get("/devices/{mac}/nmap/status", s.nmapStatus)
	r.Get("/settings", s.settingsPage)
	r.Post("/settings/theme", s.setTheme)
	r.Post("/settings/lang", s.setLang)
	r.Post("/settings/discord", s.setDiscord)
	r.Post("/settings/ntfy", s.setNtfy)
	r.Post("/settings/listening", s.toggleListening)
	r.Post("/settings/ping", s.setPing)
	r.Get("/api/stats", s.apiStats)
	r.Get("/api/alerts", s.apiAlerts)
	r.Get("/api/version", s.apiVersion)
	r.Get("/api/settings/{key}", s.apiGetSetting)
	r.Post("/api/settings/{key}", s.apiSetSetting)
	r.Get("/health", s.health)
	r.Handle("/static/*", http.FileServer(http.FS(templateFS)))

	return r
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	devices, err := s.reg.List()
	if err != nil {
		slog.Error("dashboard: list failed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	online, offline := 0, 0
	for _, d := range devices {
		if d.Category == "virtual" {
			continue
		}
		if d.Online {
			online++
		} else {
			offline++
		}
	}
	alerts, _ := s.alertEng.RecentAlerts(10)
	updateAvail, latestVer := s.updateAvailable()
	s.render(w, r, "dashboard.html", map[string]any{
		"Title":           "Dashboard",
		"Devices":         devices,
		"Online":          online,
		"Offline":         offline,
		"Alerts":          alerts,
		"UpdateAvailable": updateAvail,
		"LatestVersion":   latestVer,
	})
}

func (s *Server) deviceList(w http.ResponseWriter, r *http.Request) {
	devices, err := s.reg.List()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.render(w, r, "devices.html", map[string]any{
		"Title":   "Geräte",
		"Devices": devices,
	})
}

func (s *Server) deviceDetail(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")
	dev, err := s.reg.Get(mac)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	events, _ := s.reg.DeviceEvents(mac, 20)
	connections, _ := s.reg.GetConnections(mac)
	allDevices, _ := s.reg.List()
	devGroups, _ := s.reg.GetDeviceGroups(mac)
	allGroups, _ := s.reg.ListGroups()
	title := dev.DisplayName()
	s.render(w, r, "device_detail.html", map[string]any{
		"Title":       title,
		"Device":      dev,
		"Events":      events,
		"Connections": connections,
		"AllDevices":  allDevices,
		"DevGroups":   devGroups,
		"AllGroups":   allGroups,
	})
}

// macRe matches a normalised MAC address (XX:XX:XX:XX:XX:XX, hex, any case).
var macRe = regexp.MustCompile(`(?i)^([0-9a-f]{2}:){5}[0-9a-f]{2}$`)

// validMAC returns true if mac looks like a well-formed MAC address.
// All handler inputs from URL params go through this before hitting the DB.
func validMAC(mac string) bool { return macRe.MatchString(mac) }

// requireMAC extracts and validates the {mac} URL param.
// Returns ("", false) and writes a 400 if the MAC is malformed.
func requireMAC(w http.ResponseWriter, r *http.Request) (string, bool) {
	mac := chi.URLParam(r, "mac")
	if !validMAC(mac) {
		http.Error(w, "ungültige MAC-Adresse", http.StatusBadRequest)
		return "", false
	}
	return mac, true
}

// requireHTTPS checks that rawURL uses https and is syntactically valid.
func requireHTTPS(rawURL string) error {
	if rawURL == "" {
		return nil // empty = disabled; callers check Enabled flag separately
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("URL muss mit https:// beginnen")
	}
	return nil
}

// isPrivateHost returns true if the host part of rawURL resolves to a
// private/loopback/link-local address — used to block SSRF via webhook URLs.
func isPrivateHost(rawURL string) bool {
	u, _ := url.Parse(rawURL)
	if u == nil {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil {
		return false // hostname — we can't pre-resolve; rely on HTTPS
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func (s *Server) deviceUpdate(w http.ResponseWriter, r *http.Request, field string, fn func(mac string) error) {
	mac, ok := requireMAC(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := fn(mac); err != nil {
		slog.Error("device update failed", "field", field, "mac", mac, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/devices/"+mac, http.StatusSeeOther)
}

func (s *Server) setLabel(w http.ResponseWriter, r *http.Request) {
	s.deviceUpdate(w, r, "label", func(mac string) error { return s.reg.SetLabel(mac, r.FormValue("label")) })
}
func (s *Server) setHostname(w http.ResponseWriter, r *http.Request) {
	s.deviceUpdate(w, r, "hostname", func(mac string) error { return s.reg.SetHostname(mac, r.FormValue("hostname")) })
}
func (s *Server) setCategory(w http.ResponseWriter, r *http.Request) {
	s.deviceUpdate(w, r, "category", func(mac string) error { return s.reg.SetCategory(mac, r.FormValue("category")) })
}
func (s *Server) setForcePing(w http.ResponseWriter, r *http.Request) {
	s.deviceUpdate(w, r, "force_ping", func(mac string) error { return s.reg.SetForcePing(mac, r.FormValue("force_ping") == "true") })
}
func (s *Server) setPriority(w http.ResponseWriter, r *http.Request) {
	s.deviceUpdate(w, r, "priority", func(mac string) error { return s.reg.SetPriority(mac, r.FormValue("priority") == "true") })
}
func (s *Server) approveDevice(w http.ResponseWriter, r *http.Request) {
	s.deviceUpdate(w, r, "approve", func(mac string) error { return s.reg.Approve(mac) })
}

func (s *Server) alertList(w http.ResponseWriter, r *http.Request) {
	alerts, err := s.alertEng.RecentAlerts(50)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.render(w, r, "alerts.html", map[string]any{
		"Title":  "Alerts",
		"Alerts": alerts,
	})
}

func (s *Server) eventLog(w http.ResponseWriter, r *http.Request) {
	events, err := s.reg.RecentEvents(200)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.render(w, r, "log.html", map[string]any{
		"Title":  "Log",
		"Events": events,
	})
}

func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")
	if err := s.reg.Delete(mac); err != nil {
		slog.Error("delete device failed", "mac", mac, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/devices", http.StatusSeeOther)
}

func (s *Server) createDevice(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	mac := r.FormValue("mac")
	ip := r.FormValue("ip")
	label := r.FormValue("label")
	category := r.FormValue("category")

	if ip != "" && net.ParseIP(ip) == nil {
		http.Error(w, "ungültige IP-Adresse", http.StatusBadRequest)
		return
	}

	dev, err := s.reg.AddManual(mac, ip, label, category)
	if err != nil {
		slog.Error("manual device create failed", "err", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/devices/"+dev.MAC, http.StatusSeeOther)
}

func (s *Server) addConnection(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")
	r.ParseForm()
	target := r.FormValue("target_mac")
	connType := r.FormValue("type")
	label := r.FormValue("label")
	if target == "" || connType == "" {
		http.Redirect(w, r, "/devices/"+mac, http.StatusSeeOther)
		return
	}
	if err := s.reg.AddConnection(mac, target, connType, label); err != nil {
		slog.Error("add connection failed", "err", err)
	}
	http.Redirect(w, r, "/devices/"+mac, http.StatusSeeOther)
}

func (s *Server) removeConnection(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")
	id := chi.URLParam(r, "id")
	s.reg.RemoveConnection(id)
	http.Redirect(w, r, "/devices/"+mac, http.StatusSeeOther)
}

func (s *Server) apiTopology(w http.ResponseWriter, r *http.Request) {
	topo, err := s.reg.Topology()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(topo)
}

func (s *Server) setTheme(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	s.themes.SetActive(name)
	http.Redirect(w, r, r.Referer(), http.StatusSeeOther)
}

func (s *Server) settingsPage(w http.ResponseWriter, r *http.Request) {
	settings := s.loadAppSettings()
	pingEnabled, pingInterval := s.pingCol.Settings()
	ping := PingSettings{
		Enabled:  pingEnabled,
		Interval: int(pingInterval.Minutes()),
	}
	if ping.Interval <= 0 {
		ping.Interval = 5
	}
	s.render(w, r, "settings.html", map[string]any{
		"Title":   "Settings",
		"Discord": settings.Discord,
		"Ntfy":    settings.Ntfy,
		"Ping":    ping,
		"Saved":   r.URL.Query().Get("saved") == "1",
	})
}

func (s *Server) setDiscord(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	webhookURL := strings.TrimSpace(r.FormValue("webhook_url"))
	if err := requireHTTPS(webhookURL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if isPrivateHost(webhookURL) {
		http.Error(w, "Webhook-URL darf nicht auf interne Adressen zeigen", http.StatusBadRequest)
		return
	}
	cfg := s.loadAppSettings()
	cfg.Discord = DiscordSettings{
		Enabled:    r.FormValue("enabled") == "on",
		WebhookURL: webhookURL,
	}
	if err := s.saveAppSettings(cfg); err != nil {
		slog.Error("save discord settings failed", "err", err)
	}
	s.discordCh.Update(config.DiscordCfg{
		Enabled:    cfg.Discord.Enabled,
		WebhookURL: cfg.Discord.WebhookURL,
	})
	http.Redirect(w, r, "/settings?saved=1", http.StatusSeeOther)
}

func (s *Server) setNtfy(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	ntfyURL := strings.TrimSpace(r.FormValue("url"))
	if err := requireHTTPS(ntfyURL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if isPrivateHost(ntfyURL) {
		http.Error(w, "Ntfy-URL darf nicht auf interne Adressen zeigen", http.StatusBadRequest)
		return
	}
	cfg := s.loadAppSettings()
	cfg.Ntfy = NtfySettings{
		Enabled: r.FormValue("enabled") == "on",
		URL:     ntfyURL,
		Token:   strings.TrimSpace(r.FormValue("token")),
	}
	if err := s.saveAppSettings(cfg); err != nil {
		slog.Error("save ntfy settings failed", "err", err)
	}
	s.ntfyCh.Update(config.NtfyCfg{
		Enabled: cfg.Ntfy.Enabled,
		URL:     cfg.Ntfy.URL,
		Token:   cfg.Ntfy.Token,
	})
	http.Redirect(w, r, "/settings?saved=1", http.StatusSeeOther)
}

func (s *Server) setPing(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	intervalMin, _ := strconv.Atoi(r.FormValue("interval_min"))
	if intervalMin <= 0 {
		intervalMin = 5
	}
	cfg := s.loadAppSettings()
	cfg.Ping = PingSettings{
		Enabled:  r.FormValue("enabled") == "on",
		Interval: intervalMin,
	}
	if err := s.saveAppSettings(cfg); err != nil {
		slog.Error("save settings failed", "err", err)
	}
	s.pingCol.Update(cfg.Ping.Enabled, time.Duration(intervalMin)*time.Minute)
	http.Redirect(w, r, "/settings?saved=1", http.StatusSeeOther)
}

func (s *Server) toggleListening(w http.ResponseWriter, r *http.Request) {
	cfg := s.loadAppSettings()
	cfg.Listening = !s.reg.IsListening()
	if err := s.saveAppSettings(cfg); err != nil {
		slog.Error("save settings failed", "err", err)
	}
	s.reg.SetListening(cfg.Listening)
	w.Header().Set("Content-Type", "application/json")
	if cfg.Listening {
		w.Write([]byte(`{"listening":true}`))
	} else {
		w.Write([]byte(`{"listening":false}`))
	}
}

func (s *Server) apiStats(w http.ResponseWriter, r *http.Request) {
	devices, err := s.reg.List()
	if err != nil {
		http.Error(w, `{"error":"failed"}`, http.StatusInternalServerError)
		return
	}
	online, offline, total, newDev := 0, 0, 0, 0
	for _, d := range devices {
		if d.Category != "virtual" {
			total++
			if d.Online {
				online++
			} else {
				offline++
			}
		}
		if !d.Approved {
			newDev++
		}
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"online":%d,"offline":%d,"total":%d,"new":%d,"listening":%t}`, online, offline, total, newDev, s.reg.IsListening())
}

func (s *Server) apiAlerts(w http.ResponseWriter, r *http.Request) {
	lang := s.detectLang(r)
	alerts, err := s.alertEng.RecentAlerts(10)
	if err != nil {
		http.Error(w, `{"error":"failed"}`, http.StatusInternalServerError)
		return
	}
	type alertJSON struct {
		Severity string `json:"severity"`
		Title    string `json:"title"`
		Summary  string `json:"summary"`
		MAC      string `json:"mac"`
		TimeAgo  string `json:"timeAgo"`
		Time     string `json:"time"`
	}
	out := make([]alertJSON, 0, len(alerts))
	for _, a := range alerts {
		out = append(out, alertJSON{
			Severity: a.Severity,
			Title:    s.bundle.T(lang, "alert.title."+a.Type),
			Summary:  a.Summary,
			MAC:      a.MAC,
			TimeAgo:  s.bundle.TimeAgo(lang, a.FiredAt),
			Time:     a.FiredAt.Format("15:04:05"),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (s *Server) apiVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"version":%q,"buildTime":%q}`, s.version, s.buildTime)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) apiGetSetting(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	val, err := s.reg.GetSetting(key)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(val))
}

func (s *Server) apiSetSetting(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	body, err := io.ReadAll(io.LimitReader(r.Body, 8192))
	if err != nil || len(body) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var js json.RawMessage
	if err := json.Unmarshal(body, &js); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := s.reg.SetSetting(key, string(body)); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// detectLang returns the active language for a request.
// Priority: cookie "lang" → Accept-Language header → "de".
func (s *Server) detectLang(r *http.Request) string {
	available := s.bundle.Languages()
	isValid := func(l string) bool {
		for _, a := range available {
			if a == l {
				return true
			}
		}
		return false
	}
	if c, err := r.Cookie("lang"); err == nil && isValid(c.Value) {
		return c.Value
	}
	if al := r.Header.Get("Accept-Language"); al != "" {
		for _, part := range strings.Split(al, ",") {
			tag := strings.ToLower(strings.TrimSpace(strings.Split(part, ";")[0]))
			lang := strings.Split(tag, "-")[0]
			if isValid(lang) {
				return lang
			}
		}
	}
	return "de"
}

// setLang stores the chosen language in a cookie.
func (s *Server) setLang(w http.ResponseWriter, r *http.Request) {
	lang := r.FormValue("lang")
	for _, l := range s.bundle.Languages() {
		if l == lang {
			http.SetCookie(w, &http.Cookie{
				Name: "lang", Value: lang,
				Path: "/", MaxAge: 365 * 24 * 3600,
			})
			break
		}
	}
	http.Redirect(w, r, r.Referer(), http.StatusSeeOther)
}

// render builds a fresh template set per request, injecting theme + i18n functions.
func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, data any) {
	lang := s.detectLang(r)

	// Build per-request funcMap with language baked into t/tf/timeAgo
	fm := make(template.FuncMap, len(s.funcMap)+3)
	for k, v := range s.funcMap {
		fm[k] = v
	}
	fm["t"] = func(key string) string { return s.bundle.T(lang, key) }
	fm["tf"] = func(key string, args ...any) string { return s.bundle.Tf(lang, key, args...) }
	fm["timeAgo"] = func(t time.Time) string { return s.bundle.TimeAgo(lang, t) }

	tmpl, err := template.New("").Funcs(fm).ParseFS(templateFS, "templates/base.html", "templates/"+name)
	if err != nil {
		slog.Error("template parse failed", "template", name, "err", err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	wrapped := map[string]any{
		"Theme":     s.themes.Active(),
		"Themes":    s.themes.All(),
		"Lang":      lang,
		"Langs":     s.bundle.Languages(),
		"Version":   s.version,
		"BuildTime": s.buildTime,
	}
	if m, ok := data.(map[string]any); ok {
		for k, v := range m {
			wrapped[k] = v
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "base", wrapped); err != nil {
		slog.Error("template render failed", "template", name, "err", err)
	}
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "gerade eben"
	case d < time.Hour:
		mins := int(d.Minutes())
		return fmt.Sprintf("%dm", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		return fmt.Sprintf("%dh", hours)
	default:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%dd", days)
	}
}

func eventIcon(typ string) string {
	switch typ {
	case "seen":
		return "👋"
	case "online":
		return "🟢"
	case "offline":
		return "🔴"
	case "hostname", "hostname_manual":
		return "✏️"
	case "label":
		return "🏷️"
	case "services":
		return "🔌"
	case "nmap":
		return "🔍"
	case "ip_change":
		return "🔀"
	default:
		return "•"
	}
}

func eventColor(typ string) string {
	switch typ {
	case "online":
		return "#4ade80"
	case "offline":
		return "#ef4444"
	default:
		return "var(--sm-text-primary)"
	}
}

func friendlySource(src string) string {
	switch src {
	case "registry":
		return "Poller"
	case "web":
		return "Manual"
	case "import":
		return "Import"
	case "mdns":
		return "mDNS"
	case "arp":
		return "ARP"
	case "dhcp":
		return "DHCP"
	case "manual":
		return "Manual"
	case "ping":
		return "Ping"
	case "nmap":
		return "nmap"
	default:
		return src
	}
}

func severityClass(severity string) string {
	switch severity {
	case "critical":
		return "bg-red-100 text-red-800"
	case "high":
		return "bg-orange-100 text-orange-800"
	case "medium":
		return "bg-yellow-100 text-yellow-800"
	case "info":
		return "bg-blue-100 text-blue-800"
	default:
		return "bg-gray-100 text-gray-800"
	}
}

func sortDevicesByIP(devs []registry.Device) {
	sort.Slice(devs, func(i, j int) bool {
		a := net.ParseIP(devs[i].IP).To4()
		b := net.ParseIP(devs[j].IP).To4()
		if a == nil || b == nil {
			return devs[i].IP < devs[j].IP
		}
		for k := 0; k < 4; k++ {
			if a[k] != b[k] {
				return a[k] < b[k]
			}
		}
		return false
	})
}

func (s *Server) groupList(w http.ResponseWriter, r *http.Request) {
	groups, _ := s.reg.ListGroups()
	type groupView struct {
		registry.Group
		Devices []registry.Device
	}
	var views []groupView
	for _, g := range groups {
		devs, _ := s.reg.GetGroupDevices(g.ID)
		sortDevicesByIP(devs)
		views = append(views, groupView{g, devs})
	}
	allDevices, _ := s.reg.List()
	grouped := map[string]bool{}
	for _, v := range views {
		for _, d := range v.Devices {
			grouped[d.MAC] = true
		}
	}
	var ungrouped []registry.Device
	for _, d := range allDevices {
		if !grouped[d.MAC] {
			ungrouped = append(ungrouped, d)
		}
	}
	s.render(w, r, "groups.html", map[string]any{
		"Title":      "Gruppen",
		"Groups":     views,
		"AllDevices": allDevices,
		"Ungrouped":  ungrouped,
	})
}

func (s *Server) createGroup(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	name := strings.TrimSpace(r.FormValue("name"))
	color := r.FormValue("color")
	if name == "" {
		http.Redirect(w, r, "/groups", http.StatusSeeOther)
		return
	}
	if color == "" {
		color = "#38bdf8"
	}
	if _, err := s.reg.CreateGroup(name, color); err != nil {
		slog.Error("create group failed", "err", err)
	}
	http.Redirect(w, r, "/groups", http.StatusSeeOther)
}

func (s *Server) updateGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	r.ParseForm()
	name := strings.TrimSpace(r.FormValue("name"))
	color := r.FormValue("color")
	if name != "" {
		s.reg.UpdateGroup(id, name, color)
	}
	http.Redirect(w, r, "/groups", http.StatusSeeOther)
}

func (s *Server) deleteGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.reg.DeleteGroup(id); err != nil {
		slog.Error("delete group failed", "id", id, "err", err)
	}
	http.Redirect(w, r, "/groups", http.StatusSeeOther)
}

func (s *Server) moveGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	r.ParseForm()
	direction := r.FormValue("direction")
	if direction != "up" && direction != "down" {
		http.Redirect(w, r, "/groups", http.StatusSeeOther)
		return
	}
	s.reg.MoveGroup(id, direction)
	http.Redirect(w, r, "/groups", http.StatusSeeOther)
}

func (s *Server) addDeviceToGroup(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")
	r.ParseForm()
	groupID := r.FormValue("group_id")
	if groupID != "" {
		s.reg.AddDeviceToGroup(mac, groupID)
	}
	http.Redirect(w, r, "/devices/"+mac, http.StatusSeeOther)
}

func (s *Server) removeDeviceFromGroup(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")
	groupID := chi.URLParam(r, "groupId")
	s.reg.RemoveDeviceFromGroup(mac, groupID)
	http.Redirect(w, r, "/devices/"+mac, http.StatusSeeOther)
}

func (s *Server) exportDevices(w http.ResponseWriter, r *http.Request) {
	payload, err := s.reg.Export()
	if err != nil {
		slog.Error("export failed", "err", err)
		http.Error(w, "export failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="silentmap-export.json"`)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(payload)
}

func (s *Server) importDevices(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Redirect(w, r, "/devices", http.StatusSeeOther)
		return
	}
	defer file.Close()

	var payload registry.ExportPayload
	if err := json.NewDecoder(io.LimitReader(file, 4<<20)).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if payload.Version != 1 {
		http.Error(w, "unsupported export version", http.StatusBadRequest)
		return
	}

	result, err := s.reg.Import(&payload)
	if err != nil {
		slog.Error("import failed", "err", err)
		http.Error(w, "import failed", http.StatusInternalServerError)
		return
	}
	slog.Info("import complete", "created", result.Created, "updated", result.Updated)
	http.Redirect(w, r, "/devices", http.StatusSeeOther)
}

func (s *Server) runNmap(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")
	dev, err := s.reg.Get(mac)
	if err != nil || dev.IP == "" {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}

	s.scanMu.Lock()
	if s.scanning[mac] {
		s.scanMu.Unlock()
		w.WriteHeader(http.StatusConflict)
		return
	}
	s.scanning[mac] = true
	s.scanMu.Unlock()

	args := s.nmapArgs
	if args == "" {
		args = "-sV --top-ports 20 -T3"
	}

	go func() {
		defer func() {
			s.scanMu.Lock()
			delete(s.scanning, mac)
			s.scanMu.Unlock()
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		result, err := scanner.RunNmap(ctx, dev.IP, args)
		if err != nil {
			slog.Warn("nmap scan failed", "mac", mac, "ip", dev.IP, "err", err)
			s.reg.AddEvent(mac, dev.IP, "nmap", "web", "Scan fehlgeschlagen: "+err.Error())
			return
		}

		if result.OsInfo != "" {
			s.reg.SetOsInfo(mac, result.OsInfo)
		}
		if len(result.Ports) > 0 {
			s.reg.SetNmapPorts(mac, result.Ports)
		}
		s.reg.AddEvent(mac, dev.IP, "nmap", "web", result.Summary())
		slog.Info("nmap scan complete", "mac", mac, "ip", dev.IP, "os", result.OsInfo, "ports", len(result.Ports))
	}()

	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) nmapStatus(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")
	s.scanMu.RLock()
	running := s.scanning[mac]
	s.scanMu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	if running {
		fmt.Fprint(w, `{"scanning":true}`)
	} else {
		fmt.Fprint(w, `{"scanning":false}`)
	}
}

var mdiPaths = map[string]string{
	"netzwerk":  "M12 2C6.5 2 2 6.5 2 12C2 17.5 6.5 22 12 22C17.5 22 22 17.5 22 12C22 6.5 17.5 2 12 2M12 20C7.58 20 4 16.42 4 12C4 7.58 7.58 4 12 4C16.42 4 20 7.58 20 12C20 16.42 16.42 20 12 20M13 13V16H15L12 19L9 16H11V13M5 13H8V15L11 12L8 9V11H5M11 11V8H9L12 5L15 8H13V11M19 11H16V9L13 12L16 15V13H19",
	"server":    "M4,1H20A1,1 0 0,1 21,2V6A1,1 0 0,1 20,7H4A1,1 0 0,1 3,6V2A1,1 0 0,1 4,1M4,9H20A1,1 0 0,1 21,10V14A1,1 0 0,1 20,15H4A1,1 0 0,1 3,14V10A1,1 0 0,1 4,9M4,17H20A1,1 0 0,1 21,18V22A1,1 0 0,1 20,23H4A1,1 0 0,1 3,22V18A1,1 0 0,1 4,17M9,5H10V3H9V5M9,13H10V11H9V13M9,21H10V19H9V21M5,3V5H7V3H5M5,11V13H7V11H5M5,19V21H7V19H5Z",
	"desktop":   "M21,16H3V4H21M21,2H3C1.89,2 1,2.89 1,4V16A2,2 0 0,0 3,18H10V20H8V22H16V20H14V18H21A2,2 0 0,0 23,16V4C23,2.89 22.1,2 21,2Z",
	"mobile":    "M17,19H7V5H17M17,1H7C5.89,1 5,1.89 5,3V21A2,2 0 0,0 7,23H17A2,2 0 0,0 19,21V3C19,1.89 18.1,1 17,1Z",
	"smarthome": "M12,3L2,12H5V20H19V12H22L12,3M12,8.5C14.34,8.5 16.46,9.43 18,10.94L16.8,12.12C15.58,10.91 13.88,10.17 12,10.17C10.12,10.17 8.42,10.91 7.2,12.12L6,10.94C7.54,9.43 9.66,8.5 12,8.5M12,11.83C13.4,11.83 14.67,12.39 15.6,13.3L14.4,14.47C13.79,13.87 12.94,13.5 12,13.5C11.06,13.5 10.21,13.87 9.6,14.47L8.4,13.3C9.33,12.39 10.6,11.83 12,11.83M12,15.17C12.94,15.17 13.7,15.91 13.7,16.83C13.7,17.75 12.94,18.5 12,18.5C11.06,18.5 10.3,17.75 10.3,16.83C10.3,15.91 11.06,15.17 12,15.17Z",
	"iot":       "M6,4H18V5H21V7H18V9H21V11H18V13H21V15H18V17H21V19H18V20H6V19H3V17H6V15H3V13H6V11H3V9H6V7H3V5H6V4M11,15V18H12V15H11M13,15V18H14V15H13M15,15V18H16V15H15Z",
	"media":     "M21,17H3V5H21M21,3H3A2,2 0 0,0 1,5V17A2,2 0 0,0 3,19H8V21H16V19H21A2,2 0 0,0 23,17V5A2,2 0 0,0 21,3Z",
	"drucker":   "M18,3H6V7H18M19,12A1,1 0 0,1 18,11A1,1 0 0,1 19,10A1,1 0 0,1 20,11A1,1 0 0,1 19,12M16,19H8V14H16M19,8H5A3,3 0 0,0 2,11V17H6V21H18V17H22V11A3,3 0 0,0 19,8Z",
	"virtual":   "M21,16.5C21,16.88 20.79,17.21 20.47,17.38L12.57,21.82C12.41,21.94 12.21,22 12,22C11.79,22 11.59,21.94 11.43,21.82L3.53,17.38C3.21,17.21 3,16.88 3,16.5V7.5C3,7.12 3.21,6.79 3.53,6.62L11.43,2.18C11.59,2.06 11.79,2 12,2C12.21,2 12.41,2.06 12.57,2.18L20.47,6.62C20.79,6.79 21,7.12 21,7.5V16.5M12,4.15L6.04,7.5L12,10.85L17.96,7.5L12,4.15M5,15.91L11,19.29V12.58L5,9.21V15.91M19,15.91V9.21L13,12.58V19.29L19,15.91Z",
	"unbekannt": "M15.07,11.25L14.17,12.17C13.45,12.89 13,13.5 13,15H11V14.5C11,13.39 11.45,12.39 12.17,11.67L13.41,10.41C13.78,10.05 14,9.55 14,9C14,7.89 13.1,7 12,7A2,2 0 0,0 10,9H8A4,4 0 0,1 12,5A4,4 0 0,1 16,9C16,9.88 15.64,10.67 15.07,11.25M13,19H11V17H13M12,2A10,10 0 0,0 2,12A10,10 0 0,0 12,22A10,10 0 0,0 22,12C22,6.47 17.5,2 12,2Z",
}

var catColors = map[string]string{
	"netzwerk":  "#38bdf8",
	"server":    "#a78bfa",
	"desktop":   "#818cf8",
	"mobile":    "#f472b6",
	"smarthome": "#fb923c",
	"iot":       "#fbbf24",
	"media":     "#22d3ee",
	"drucker":   "#94a3b8",
	"virtual":   "#c084fc",
	"unbekannt": "#6b7280",
}

func catColorHex(cat string) string {
	if c, ok := catColors[cat]; ok {
		return c
	}
	return "#6b7280"
}

func catIconSVG(cat string) template.HTML {
	p, ok := mdiPaths[cat]
	if !ok {
		return ""
	}
	return template.HTML(`<svg viewBox="0 0 24 24" width="13" height="13" style="display:inline-block;vertical-align:middle;margin-right:2px" fill="currentColor"><path d="` + p + `"/></svg>`)
}
