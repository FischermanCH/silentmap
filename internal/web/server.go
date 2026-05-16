package web

import (
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/silentmap/silentmap/internal/alerting/engine"
	"github.com/silentmap/silentmap/internal/registry"
)

//go:embed templates/*
var templateFS embed.FS

type Server struct {
	reg      *registry.Registry
	alertEng *engine.Engine
	db       *sql.DB
	funcMap  template.FuncMap
}

func NewServer(reg *registry.Registry, alertEng *engine.Engine, db *sql.DB) *Server {
	return &Server{
		reg:      reg,
		alertEng: alertEng,
		db:       db,
		funcMap: template.FuncMap{
			"timeAgo":       timeAgo,
			"severityClass": severityClass,
			"eventIcon":     eventIcon,
			"slice": func(args ...string) []string { return args },
		},
	}
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
	r.Post("/devices/{mac}/category", s.setCategory)
	r.Get("/alerts", s.alertList)
	r.Get("/health", s.health)

	return r
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	devices, err := s.reg.List()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	online, offline := 0, 0
	for _, d := range devices {
		if d.Online {
			online++
		} else {
			offline++
		}
	}
	alerts, _ := s.alertEng.RecentAlerts(10)
	s.render(w, "dashboard.html", map[string]any{
		"Title":   "Dashboard",
		"Devices": devices,
		"Online":  online,
		"Offline": offline,
		"Alerts":  alerts,
	})
}

func (s *Server) deviceList(w http.ResponseWriter, r *http.Request) {
	devices, err := s.reg.List()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.render(w, "devices.html", map[string]any{
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
	title := dev.DisplayName()
	s.render(w, "device_detail.html", map[string]any{
		"Title":  title,
		"Device": dev,
		"Events": events,
	})
}

func (s *Server) setLabel(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	label := r.FormValue("label")
	if err := s.reg.SetLabel(mac, label); err != nil {
		slog.Error("set label failed", "mac", mac, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/devices/"+mac, http.StatusSeeOther)
}

func (s *Server) setHostname(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := s.reg.SetHostname(mac, r.FormValue("hostname")); err != nil {
		slog.Error("set hostname failed", "mac", mac, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/devices/"+mac, http.StatusSeeOther)
}

func (s *Server) setCategory(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := s.reg.SetCategory(mac, r.FormValue("category")); err != nil {
		slog.Error("set category failed", "mac", mac, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/devices/"+mac, http.StatusSeeOther)
}

func (s *Server) setPriority(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	priority := r.FormValue("priority") == "true"
	if err := s.reg.SetPriority(mac, priority); err != nil {
		slog.Error("set priority failed", "mac", mac, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/devices/"+mac, http.StatusSeeOther)
}

func (s *Server) alertList(w http.ResponseWriter, r *http.Request) {
	alerts, err := s.alertEng.RecentAlerts(50)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.render(w, "alerts.html", map[string]any{
		"Title":  "Alerts",
		"Alerts": alerts,
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// render builds a fresh template set per request (base + page) to avoid
// Go's "last define wins" issue when all pages share a template set.
func (s *Server) render(w http.ResponseWriter, name string, data any) {
	tmpl, err := template.New("").Funcs(s.funcMap).ParseFS(templateFS, "templates/base.html", "templates/"+name)
	if err != nil {
		slog.Error("template parse failed", "template", name, "err", err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
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
	default:
		return "•"
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
