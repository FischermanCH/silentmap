// Package email sends alert notifications via SMTP with bilingual HTML emails.
package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"github.com/silentmap/silentmap/internal/alerting/channels"
)

// Config holds the runtime-editable email channel settings (decrypted plaintext).
type Config struct {
	Enabled  bool
	SMTPHost string
	SMTPPort int
	SMTPUser string
	SMTPPass string
	From     string
	To       string
	TLSMode  string // "starttls" | "tls" | "none"
	Lang     string // "de" | "en"
}

type Channel struct {
	mu      sync.RWMutex
	cfg     Config
	logoB64 string
	tmpl    *template.Template
}

func New(cfg Config, logoBytes []byte) *Channel {
	return &Channel{
		cfg:     cfg,
		logoB64: base64.StdEncoding.EncodeToString(logoBytes),
		tmpl:    template.Must(template.New("email").Parse(htmlTmpl)),
	}
}

func (c *Channel) Name() string { return "email" }

func (c *Channel) Enabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cfg.Enabled && c.cfg.SMTPHost != "" && c.cfg.From != "" && c.cfg.To != ""
}

func (c *Channel) Update(cfg Config) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg = cfg
}

func (c *Channel) Send(_ context.Context, a channels.Alert) error {
	c.mu.RLock()
	cfg := c.cfg
	logoB64 := c.logoB64
	c.mu.RUnlock()

	lang := cfg.Lang
	if lang != "de" {
		lang = "en"
	}
	str := i18n[lang]

	subject := str[a.Type]
	if subject == "" {
		subject = a.Title
	}

	data := emailData{
		LogoSrc:    template.URL("data:image/png;base64," + logoB64),
		AlertColor: alertColor(a.Type),
		AlertIcon:  alertIcon(a.Type),
		AlertTitle: subject,
		FiredAt:    a.FiredAt.Format("2006-01-02 15:04:05"),
		FooterText: str["footer"],
		Fields:     buildFields(a, str),
	}

	var buf bytes.Buffer
	if err := c.tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("email template: %w", err)
	}

	return sendSMTP(cfg, subject, buf.String())
}

type emailData struct {
	LogoSrc    template.URL
	AlertColor string
	AlertIcon  string
	AlertTitle string
	FiredAt    string
	FooterText string
	Fields     []emailField
}

type emailField struct {
	Label string
	Value string
}

func buildFields(a channels.Alert, str map[string]string) []emailField {
	meta := func(k string) string { v, _ := a.Meta[k].(string); return v }
	var out []emailField
	add := func(label, value string) {
		if value != "" {
			out = append(out, emailField{label, value})
		}
	}

	name := meta("label")
	if name == "" {
		name = meta("hostname")
	}
	if name == "" {
		name = meta("hostnameAuto")
	}
	if name != "" {
		add(str["name"], name)
	}

	add(str["ip"], a.IP)
	add(str["mac"], a.MAC)

	hostname := meta("hostname")
	if hostname == "" {
		hostname = meta("hostnameAuto")
	}
	if hostname != name {
		add(str["hostname"], hostname)
	}
	add(str["vendor"], meta("vendor"))
	add(str["category"], meta("category"))
	add(str["group"], meta("groups"))

	switch a.Type {
	case "new_device":
		add(str["first_seen"], meta("firstSeen"))
	case "priority_offline":
		add(str["last_seen"], meta("lastSeen"))
	case "device_back":
		add(str["last_seen"], meta("lastSeen"))
	case "service_down", "service_back":
		add(str["url"], meta("httpURL"))
		add(str["last_seen"], meta("lastSeen"))
	}

	return out
}

func sendSMTP(cfg Config, subject, htmlBody string) error {
	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)

	// Build RFC 2822 message
	var raw strings.Builder
	for k, v := range map[string]string{
		"From":         cfg.From,
		"To":           cfg.To,
		"Subject":      subject,
		"MIME-Version": "1.0",
		"Content-Type": "text/html; charset=UTF-8",
		"Date":         time.Now().Format("Mon, 02 Jan 2006 15:04:05 -0700"),
	} {
		raw.WriteString(k + ": " + v + "\r\n")
	}
	raw.WriteString("\r\n")
	raw.WriteString(htmlBody)
	msg := []byte(raw.String())

	switch cfg.TLSMode {
	case "tls":
		tlsCfg := &tls.Config{ServerName: cfg.SMTPHost, MinVersion: tls.VersionTLS12}
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("email tls dial: %w", err)
		}
		defer conn.Close()
		client, err := smtp.NewClient(conn, cfg.SMTPHost)
		if err != nil {
			return fmt.Errorf("email smtp client: %w", err)
		}
		defer client.Close()
		if cfg.SMTPUser != "" {
			if err := client.Auth(smtp.PlainAuth("", cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPHost)); err != nil {
				return fmt.Errorf("email auth: %w", err)
			}
		}
		return sendData(client, cfg.From, cfg.To, msg)

	case "starttls":
		client, err := smtp.Dial(addr)
		if err != nil {
			return fmt.Errorf("email starttls dial: %w", err)
		}
		defer client.Close()
		tlsCfg := &tls.Config{ServerName: cfg.SMTPHost, MinVersion: tls.VersionTLS12}
		if err := client.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("email starttls upgrade: %w", err)
		}
		if cfg.SMTPUser != "" {
			if err := client.Auth(smtp.PlainAuth("", cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPHost)); err != nil {
				return fmt.Errorf("email auth: %w", err)
			}
		}
		return sendData(client, cfg.From, cfg.To, msg)

	default: // "none" — plain SMTP, no TLS
		var auth smtp.Auth
		if cfg.SMTPUser != "" {
			auth = smtp.PlainAuth("", cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPHost)
		}
		return smtp.SendMail(addr, auth, cfg.From, []string{cfg.To}, msg)
	}
}

// TestConnection dials the SMTP server, authenticates, sends a test email, and returns any error.
// It is safe to call concurrently and does not affect the running channel state.
func TestConnection(cfg Config) error {
	return sendSMTP(cfg, "silentmap — SMTP test",
		"<p>This is a test message from <b>silentmap</b> to verify your SMTP configuration. You can ignore this email.</p>")
}

func sendData(client *smtp.Client, from, to string, msg []byte) error {
	if err := client.Mail(from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	wc, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := wc.Write(msg); err != nil {
		wc.Close()
		return err
	}
	return wc.Close()
}

func alertColor(alertType string) string {
	switch alertType {
	case "new_device":
		return "#3b82f6"
	case "priority_offline", "service_down":
		return "#ef4444"
	case "device_back", "service_back":
		return "#22c55e"
	default:
		return "#6b7280"
	}
}

func alertIcon(alertType string) string {
	switch alertType {
	case "new_device":
		return "🆕"
	case "priority_offline":
		return "🔴"
	case "device_back":
		return "🟢"
	case "service_down":
		return "🔶"
	case "service_back":
		return "🟢"
	default:
		return "ℹ️"
	}
}

var i18n = map[string]map[string]string{
	"de": {
		"new_device":       "Neues Gerät erkannt",
		"priority_offline": "Prioritäts-Gerät offline",
		"device_back":      "Gerät wieder online",
		"service_down":     "HTTP-Service nicht erreichbar",
		"service_back":     "HTTP-Service wieder online",
		"name":             "Name",
		"ip":               "IP-Adresse",
		"mac":              "MAC-Adresse",
		"hostname":         "Hostname",
		"vendor":           "Hersteller",
		"category":         "Kategorie",
		"group":            "Gruppe",
		"first_seen":       "Erstmals gesehen",
		"last_seen":        "Zuletzt gesehen",
		"url":              "URL",
		"footer":           "silentmap — Netzwerk-Monitoring",
	},
	"en": {
		"new_device":       "New device detected",
		"priority_offline": "Priority device offline",
		"device_back":      "Device back online",
		"service_down":     "HTTP service unreachable",
		"service_back":     "HTTP service back online",
		"name":             "Name",
		"ip":               "IP Address",
		"mac":              "MAC Address",
		"hostname":         "Hostname",
		"vendor":           "Vendor",
		"category":         "Category",
		"group":            "Group",
		"first_seen":       "First seen",
		"last_seen":        "Last seen",
		"url":              "URL",
		"footer":           "silentmap — Network Monitor",
	},
}

const htmlTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>silentmap</title>
</head>
<body style="margin:0;padding:0;background-color:#f1f5f9;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color:#f1f5f9;padding:32px 16px">
<tr><td align="center">
<table role="presentation" width="580" cellpadding="0" cellspacing="0" border="0" style="max-width:580px;width:100%">

  <!-- Header -->
  <tr>
    <td style="background-color:#0f172a;border-radius:12px 12px 0 0;padding:20px 24px">
      <table role="presentation" cellpadding="0" cellspacing="0" border="0">
        <tr>
          <td style="vertical-align:middle;padding-right:12px">
            <img src="{{.LogoSrc}}" width="40" height="40" alt="silentmap" style="border-radius:8px;display:block">
          </td>
          <td style="vertical-align:middle">
            <span style="color:#f8fafc;font-size:20px;font-weight:700;letter-spacing:-0.3px">silentmap</span>
          </td>
        </tr>
      </table>
    </td>
  </tr>

  <!-- Alert banner -->
  <tr>
    <td style="background-color:{{.AlertColor}};padding:14px 24px">
      <p style="margin:0;color:#ffffff;font-size:15px;font-weight:600;line-height:1.4">{{.AlertIcon}} {{.AlertTitle}}</p>
    </td>
  </tr>

  <!-- Body -->
  <tr>
    <td style="background-color:#ffffff;padding:24px 24px 20px;border-radius:0 0 12px 12px">

      <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0">
        {{range .Fields}}
        <tr>
          <td style="padding:9px 0;border-bottom:1px solid #f1f5f9;color:#6b7280;font-size:13px;width:38%;vertical-align:top">{{.Label}}</td>
          <td style="padding:9px 0 9px 16px;border-bottom:1px solid #f1f5f9;color:#111827;font-size:13px;font-weight:500;font-family:monospace;vertical-align:top;word-break:break-all">{{.Value}}</td>
        </tr>
        {{end}}
      </table>

      <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="margin-top:20px">
        <tr>
          <td style="padding-top:16px;border-top:1px solid #e5e7eb;text-align:center">
            <span style="color:#9ca3af;font-size:12px">{{.FooterText}}</span>
            <span style="color:#d1d5db;font-size:12px">&nbsp;&middot;&nbsp;</span>
            <a href="https://github.com/FischermanCH/silentmap" style="color:#3b82f6;font-size:12px;text-decoration:none">GitHub</a>
            <br>
            <span style="color:#d1d5db;font-size:11px;margin-top:4px;display:inline-block">{{.FiredAt}}</span>
          </td>
        </tr>
      </table>

    </td>
  </tr>

</table>
</td></tr>
</table>
</body>
</html>`
