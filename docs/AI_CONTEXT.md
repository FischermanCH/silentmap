# AI Context — silentmap

Dieses Dokument ist für eine zukünftige KI-Instanz (Claude oder ähnliches) gedacht,
die an diesem Projekt weiterarbeitet. Es ersetzt das mühsame Erklären von Null.

---

## Was ist silentmap?

Ein selbst-gehostetes Netzwerk-Monitoring-Tool, das Geräte im lokalen Netzwerk passiv
erkennt (ARP, mDNS, DHCP) und aktiv überwacht (Ping, nmap). Single binary, läuft als
Docker-Container auf einem Heimserver (Proxmox/Portainer). Web UI auf Port 8080.

**Docker Hub:** `fischermanch/silentmap`
**GitHub:** `FischermanCH/silentmap`

---

## Tech-Stack

| Was | Wie |
|---|---|
| Sprache | Go (kein CGO, single binary) |
| Datenbank | SQLite via `modernc.org/sqlite` (CGO-frei), WAL-Modus |
| Web | chi Router + `html/template` (SSR, kein Framework) |
| Frontend | HTMX + Tailwind CSS (CDN, kein Build-Step) |
| Container | Alpine Linux, ~25MB Image |
| Deployment | Docker / Portainer auf Heimserver |

---

## Projektstruktur

```
cmd/silentmap/main.go          — Einstiegspunkt, Flag-Parsing, Komponenten-Verdrahtung
internal/
  registry/registry.go         — Kernlogik: Device-Verwaltung, Groups, SQLite-Zugriff
  registry/migrate.go          — Schema-Migrationen (additive ALTER TABLE, idempotent)
  web/server.go                — HTTP-Handler, Routing, Template-Rendering
  web/templates/               — HTML-Templates (groups.html, devices.html, ...)
  web/static/                  — Statische Assets (d3.min.js)
  bus/bus.go                   — Synchroner Event-Bus (SubscribeSync für Sequenz)
  collectors/arp/              — Passiver ARP-Sniffer (braucht CAP_NET_RAW)
  collectors/mdns/             — Passiver mDNS-Listener
  collectors/dhcp/             — Passiver DHCP-Listener
  collectors/ping/             — Aktiver ICMP-Ping (für Priority-Devices)
  collectors/httpcheck/        — HTTP/HTTPS-Verfügbarkeitscheck (opt-in, Kategorie http-service)
  scanner/nmap.go              — nmap-Integration (on-demand, pro Device)
  alerting/engine/             — Alert-Engine: Regeln, Dedup, Cooldown
  alerting/channels/discord/   — Discord-Webhook-Channel
  alerting/channels/ntfy/      — ntfy-Push-Channel
  alerting/channels/email/     — E-Mail-Channel (SMTP, bilingual HTML)
  crypto/secrets.go            — AES-256-GCM Verschlüsselung für Secrets at rest
  config/config.go             — YAML-Config + Defaults
  i18n/i18n.go                 — DE/EN Übersetzungen
VERSION                        — Aktuelle Version (z.B. "1.0.15"), wird in Build eingebettet
CHANGELOG.md                   — Alle Änderungen chronologisch
```

---

## Datenmodell (SQLite)

### `devices`
| Spalte | Typ | Bedeutung |
|---|---|---|
| `mac` | TEXT PK | Normalisiert: `AA:BB:CC:DD:EE:FF` |
| `ip` | TEXT | Letzte bekannte IP |
| `hostname` | TEXT | Manuell gesetzt (UI) |
| `hostname_auto` | TEXT | Automatisch via mDNS/DHCP/PTR |
| `vendor` | TEXT | Aus OUI-DB |
| `label` | TEXT | Freitext-Label (UI) |
| `category` | TEXT | z.B. "smartphone", "nas", "router" |
| `services` | TEXT | JSON-Array mDNS-Dienste |
| `nmap_ports` | TEXT | JSON-Array offener Ports (z.B. `["22/tcp open ssh"]`) |
| `os_info` | TEXT | nmap OS-Erkennung |
| `http_url` | TEXT | URL für HTTP-Verfügbarkeitscheck (opt-in, leer = deaktiviert) |
| `notes` | TEXT | Freitext-Notizen (nur auf Device-Detailseite sichtbar) |
| `priority` | INTEGER | 0/1 — löst kritische Alerts aus |
| `approved` | INTEGER | 0/1 — neue Geräte starten mit 0 |
| `online` | INTEGER | 0/1 — aktuell erreichbar |
| `force_ping` | INTEGER | 0/1 — ICMP statt ARP (für Geräte ausserhalb Subnet) |
| `first_seen` | DATETIME | |
| `last_seen` | DATETIME | |

### `device_groups`
| Spalte | Typ | Bedeutung |
|---|---|---|
| `id` | TEXT PK | UUID |
| `name` | TEXT | Gruppenname |
| `color` | TEXT | Hex-Farbe (#RRGGBB) |
| `sort_order` | INTEGER | Reihenfolge auf /groups-Seite |

### `device_group_members`
| Spalten | Bedeutung |
|---|---|
| `group_id`, `mac` | Composite PK, many-to-many |

### Weitere Tabellen
- `device_events` — Aktivitätslog pro Device (seen, online, offline, label, hostname...)
- `device_connections` — Manuelle physische/logische Verbindungen für die Topologie-Map
- `alerts` — Gefeuerte Alerts (type, severity, mac, fired_at)
- `settings` — Key-Value-Store für UI-Einstellungen (theme, Discord, ntfy, Ping...)

---

## Wichtige Patterns & Fallen

### `EventDeviceBack` / `EventDeviceLost` müssen `priority` in Meta haben
`onDeviceBack` und `onDeviceLost` in der Alert-Engine prüfen `ev.Meta["priority"].(bool)`
bevor sie einen Alert feuern. Wenn `priority` fehlt, ist der Wert `false` → Alert wird
stillschweigend verworfen. Beim Publish von `EventDeviceBack` in `registry.go` (handleSeen)
**muss** `"priority": existing.Priority` in der Meta-Map stehen.

### `scanDevices` vs. `scanDevice` — Spaltenanzahl
Zwei unterschiedliche Scanner für zwei unterschiedliche Abfragen:

**`scanDevices()` — 17 Spalten (Bulk-Abfragen):**
```
mac, ip, hostname, hostname_auto, vendor, label, category,
services, priority, approved, online, first_seen, last_seen,
os_info, force_ping, nmap_ports, http_url
```
Betroffene Funktionen: `List()`, `GetGroupDevices()`, `PriorityDevices()`, `HttpServiceDevices()`

**`scanDevice()` — 18 Spalten (Single-Row-Abfragen):**
Wie oben + `notes` am Ende. Nur verwendet von `get()` (→ Device-Detailseite).

Fehlt eine Spalte → `rows.Scan()` schlägt stillschweigend fehl (`continue`) →
leere Ergebnisliste ohne Fehlermeldung. **Bug-Quelle wenn neue Spalten hinzukommen.**
`notes` ist bewusst **nicht** in den Bulk-Queries — so bleibt der bestehende Code unberührt.

### Device Notes (seit v1.0.20)
Freitext-Notizfeld pro Gerät. Gespeichert in `devices.notes`. Nur auf der Device-Detailseite
angezeigt und editierbar — **nicht** in der Geräteliste, den Map-Tooltips oder Bulk-Queries.
`SetNotes(mac, notes string) error` in `registry.go`.

### Approve All (seit v1.0.20)
`ApproveAll() (int, error)` setzt `approved=1` für alle Geräte mit `approved=0`.
In `devices.html` erscheint ein "Alle bestätigen"-Button wenn `NewCount > 0` (berechnet
in `deviceList` Handler). Route: `POST /devices/approve-all`.

### Maintenance Mode / Alarmpause (seit v1.0.20)
Globale Alertunterdrückung via `sync/atomic` int64 im Alert-Engine-Struct:
```go
type Engine struct {
    maintenanceUntil int64  // atomic, Unix timestamp; 0 = inaktiv
    ...
}
```
`SetMaintenance(until time.Time)` — setzt oder löscht (Zero-Time → 0).
`MaintenanceUntil() time.Time` — liest zurück.
`fire()` prüft vor jedem Alert: wenn `time.Now().Unix() < maintenanceUntil` → Alarm verworfen.
Zustand wird in `AppSettings.MaintenanceUntil int64` (Unix-Timestamp) persistiert.
Beim Server-Start wird der gespeicherte Wert wiederhergestellt: `alertEng.SetMaintenance(time.Unix(...))`.
Route: `POST /settings/maintenance` mit Form-Field `duration` (z.B. "30m", "1h", "2h", "8h", "off").

### HTTP-Service-Alerts (seit v1.0.20)
Neue Alert-Typen `service_down` und `service_back` für Devices mit `category == "http-service"`.
`onDeviceLost` feuert **beide** `priority_offline` (wenn priority=true) **und** `service_down` (wenn category=http-service).
Konfigurierbar in `config.yaml` unter `alerting.rules.service_down` / `service_back`.
Defaults: enabled=true, severity="high", ServiceDown cooldown=15min, ServiceBack cooldown=5min.

### Auto nmap bei neuen Geräten (seit v1.0.20)
Opt-in Toggle in Settings → Netzwerk → "Auto nmap bei neuen Geräten".
Gespeichert in `AppSettings.AutoNmapNewDevice bool`.
`WireEvents(ctx, bus)` in `server.go` abonniert `EventDeviceNew` und ruft `scheduleNmapScan(ctx, mac, ip)`.
`scheduleNmapScan` ist ein extrahierter Helper der den bestehenden `scanMu`-Mutex nutzt
(verhindert gleichzeitige Scans). Muss aus `main.go` nach Bus-Erstellung aufgerufen werden.
Route: `POST /settings/auto-nmap` mit Form-Field `enabled` ("1" / "").

### Topologie-Map Gruppen-Filter: Shift+click Isolierung (seit v1.0.20)
`buildGroupFilter()` in `dashboard.html`:
- Normaler Click: Toggle (wie bisher)
- Shift+Click: Isolierung — zeigt nur diese Gruppe, versteckt alle anderen
- Reset-Button "✕ ALL" erscheint wenn irgendwelche Gruppen versteckt sind
`_allGroupIds` Array wird bei `buildGroupFilter()` befüllt und vom Reset-Button genutzt.

### HTTP-Service-Monitoring (seit v1.0.19)
Opt-in auf zwei Ebenen:
1. **Global**: Settings → HTTP Check aktivieren + Intervall setzen (default: deaktiviert)
2. **Pro Device**: Kategorie auf "http-service" setzen, dann URL im Feld "HTTP-URL" eintragen

Collector: `internal/collectors/httpcheck/httpcheck.go`
- Holt alle Devices mit `http_url != ''` via `reg.HttpServiceDevices()`
- HTTP GET mit 10s Timeout, TLS-Zertifikatsfehler werden ignoriert (heimnetz-üblich)
- Jede Antwort (auch 4xx/5xx) = `EventDeviceSeen` → Device bleibt online
- Kein Response = kein Event → Offline-Checker markiert Device nach Timeout offline
- Startet immer (wie Ping-Collector), Enabled-Status kommt aus `AppSettings.HttpCheck`

### Schema-Migrationen
Neue Spalten werden via `ALTER TABLE ... ADD COLUMN` am Ende von `migrate.go` hinzugefügt.
SQLite ignoriert Fehler bei doppelter Spalte nicht automatisch — der Code nutzt `db.Exec()`
ohne Fehlercheck, was bei "duplicate column name" einfach weiterläuft. Das ist gewollt.

### Event-Bus: SubscribeSync vs. Subscribe
- `SubscribeSync` → sequenziell, blockiert den Publisher. Wird von Registry genutzt um
  race conditions bei schnellen ARP-Bursts zu vermeiden.
- `Subscribe` → async mit Queue, für langsame Consumer (Alerting).

### Offline-Timeout & Debounce
- `offlineTimeout` kommt aus Config (`collectors.arp.offline_timeout`, Default: 15min)
- `debounceWindow = offlineTimeout / 3` — supprimiert redundante DB-Schreibzugriffe für
  stabile Online-Geräte (min. 3 Schreibvorgänge pro Timeout-Periode)

### MAC-Normalisierung
`normalizeMac()` wandelt immer in `AA:BB:CC:DD:EE:FF` (uppercase, Doppelpunkte).
**Immer** bei User-Input oder externen Quellen aufrufen.

### Update-Checker
`Server.StartBackground(ctx)` startet eine Goroutine die alle 6h die GitHub Releases API pollt.
Ergebnis (`latestVersion string`) wird per `sync.RWMutex` gecacht. `updateAvailable()` vergleicht
mit der eingebetteten `version`-Variable (normalisiert mit `v`-Prefix).
`dashboard()` übergibt `UpdateAvailable bool` und `LatestVersion string` ans Template.
Wenn der Request fehlschlägt, bleibt `latestVersion` leer → kein Indikator gezeigt.
`StartBackground` muss aus `main.go` nach dem `ctx`-Setup aufgerufen werden.

### Settings-Persistenz & Verschlüsselung (seit v1.0.22)
UI-Einstellungen werden als JSON in `$DATA_DIR/settings.json` gespeichert (via `AppSettings`-Struct).

**Secrets werden verschlüsselt gespeichert** (`enc:<base64(nonce+ciphertext)>`):
- Discord Webhook-URL → `AppSettings.Discord.WebhookURL`
- ntfy Token → `AppSettings.Ntfy.Token`
- SMTP-Passwort → `AppSettings.Email.SMTPPass`

Schlüssel: `$DATA_DIR/secret.key` (32 Byte, wird beim ersten Start automatisch generiert).
Algo: AES-256-GCM. Package: `internal/crypto/secrets.go`.

**Migration** (bestehende Klartextwerte): `crypto.Decrypt()` gibt Werte ohne `enc:`-Prefix
unverändert zurück → nahtlose Abwärtskompatibilität. Beim nächsten Speichern der Settings-Seite
werden Werte automatisch verschlüsselt.

**Template-Daten für Masking** (Settings-Seite):
- `DiscordConfigured bool` — true wenn Webhook konfiguriert (nie den Webhook-Wert im Template!)
- `NtfyTokenConfigured bool` — true wenn Token gesetzt
- `EmailPassConfigured bool` — true wenn SMTP-Passwort gesetzt
- Sensitive Felder zeigen leere Inputs mit `placeholder="Leave blank to keep..."`
- `setDiscord`/`setNtfy`/`setEmail` behalten bestehenden verschlüsselten Wert wenn Input leer

### E-Mail-Channel (seit v1.0.22)
`internal/alerting/channels/email/email.go`
- Config: `SMTPHost`, `SMTPPort`, `SMTPUser`, `SMTPPass` (Klartext, wird entschlüsselt übergeben),
  `From`, `To`, `TLSMode` ("starttls"|"tls"|"none"), `Lang` ("de"|"en")
- TLS-Modi: Port 465 → direktes TLS (`tls.Dial`), Port 587 → STARTTLS, sonst plain
- HTML-Template inline als const-String, `html/template` (auto-escaping für Device-Metadaten)
- Logo: `web.LogoBytes()` gibt `static/favicon-180.png` aus dem embedded FS zurück,
  wird im Email-Kanal als base64 Data-URI eingebettet (`src="data:image/png;base64,..."`)
- Bilingual via interne `i18n`-Map (kein Bundle, autark im Package)
- Alert-Typen: `new_device`, `priority_offline`, `device_back`, `service_down`, `service_back`
- `TestConnection(cfg Config) error` — sendet eine Test-E-Mail ohne Channel-State zu ändern;
  intern: `sendSMTP(cfg, subject, html)` → gleiche Verbindungslogik wie beim echten Alert
- Route: `POST /settings/email/test` — liest Formwerte, fällt bei leerem Passwort auf
  gespeicherten (entschlüsselten) Wert zurück; gibt JSON `{"ok":true}` / `{"ok":false,"error":"..."}`;
  15s-Timeout via Goroutine + `time.After`. Kein `settingsError`-Redirect, kein Seiten-Reload.

### Deployment (Produktion, seit v1.0.21 Update)
- Bind Mount: `/opt/silentmap:/data` (statt Named Volume — Portainer-Validator akzeptiert
  kein top-level `volumes:` in Stack-Dateien)
- SQLite darf **nicht** auf einem SMB/NFS-Share liegen (Locking-Probleme, 10s-Busy-Timeout)
- Backup-Script: `/home/fischerman/scripts/backup-silentmap.sh` (kopiert DB auf NAS)
- Cron (root): täglich 03:00, Log: `/var/log/silentmap-backup.log`

---

## Release-Prozess

1. Bug fixen / Feature implementieren
2. `VERSION` hochzählen (z.B. `1.0.14` → `1.0.15`)
3. `CHANGELOG.md` — neuen Abschnitt unter `[Unreleased]` einfügen
4. Git commit + Tag:
   ```bash
   git add -A
   git commit -m "fix: <beschreibung> (v1.0.15)"
   git tag v1.0.15
   ```
5. Docker bauen und pushen:
   ```bash
   make docker        # baut fischermanch/silentmap:v1.0.15 + :latest
   make docker-push   # pusht beide Tags auf Docker Hub
   ```
6. GitHub Release erstellen:
   ```bash
   gh release create v1.0.15 --title "v1.0.15" --notes "<changelog-text>"
   ```
7. Auf dem Server neu deployen (Portainer → Stack updaten oder `docker pull` + restart)

**Wichtig:** `make docker` liest VERSION aus `git describe --tags` — der Tag muss
**vor** dem Docker-Build gesetzt sein, sonst landet `-dirty` im Image-Tag.

---

## Deployment (Produktion)

- Läuft auf einem Heimserver unter Portainer
- Stack-Datei: `portainer-stack.yml` im Projektroot
- Daten werden in einem Bind Mount (`/opt/silentmap:/data`) persistiert → `/data` im Container
- Braucht `--net=host` + `--cap-add=NET_RAW` für ARP/mDNS/DHCP
- Web UI: `http://<server-ip>:8080`

---

## Alert-Kanäle (aktuell implementiert)

| Kanal | Status | Config via |
|---|---|---|
| Discord | Implementiert | Settings-UI, Webhook verschlüsselt in settings.json |
| ntfy | Implementiert | Settings-UI, Token verschlüsselt in settings.json |
| Email | Implementiert | Settings-UI, Passwort verschlüsselt in settings.json |
| Webhook | Geplant | — |

---

## Roadmap / Offene TODOs (aus CHANGELOG [Unreleased])

- Webhook Alert-Channel
- Basic Auth für Web UI
- Multi-platform Docker builds (arm64)

---

## Bekannte Architektur-Eigenheiten

- **Keine HTMX-Partials** — alle Seiten sind Full-Page-Renders. HTMX wird nur für
  einzelne Polling-Requests genutzt (z.B. nmap-Status).
- **Keine automatische Gruppen-Zuweisung** — Gruppen werden manuell vom User verwaltet.
- **Topologie-Map** ist D3.js-basiert (`/api/topology` liefert JSON), serverseitig
  gerenderte Nodes/Links mit clientseitigem Force-Layout.
- **i18n** ist DE/EN, Sprache wird per Cookie gesetzt (`lang`), Template-Funktionen
  `{{t "key"}}` und `{{tf "key" arg}}` (mit Format-Argument).
