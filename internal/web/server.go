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
	reg     *registry.Registry
	alertEng *engine.Engine
	db      *sql.DB
	tmpl    *template.Template
}

func NewServer(reg *registry.Registry, alertEng *engine.Engine, db *sql.DB) *Server {
	tmpl := template.Must(template.New("").Funcs(template.FuncMap{
		"timeAgo": timeAgo,
		"severityClass": severityClass,
	}).ParseFS(templateFS, "templates/*.html"))

	return &Server{
		reg:      reg,
		alertEng: alertEng,
		db:       db,
		tmpl:     tmpl,
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
	r.Post("/devices/{mac}/priority", s.setPriority)
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
	s.render(w, "device_detail.html", map[string]any{
		"Title":  "Gerät: " + dev.MAC,
		"Device": dev,
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

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
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
