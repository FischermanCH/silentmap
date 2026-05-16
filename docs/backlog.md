# Backlog

Priorisierter Feature-Backlog. Status: `[ ]` offen · `[x]` erledigt · `[~]` in Arbeit · `[-]` zurückgestellt.

---

## Milestone 0.1 — MVP (Core + ARP + ntfy)

> Ziel: System läuft, erkennt Geräte passiv via ARP, sendet Alerts via ntfy.

| Prio | Status | Aufgabe | Modul |
|---|---|---|---|
| P0 | [ ] | Event Bus implementieren | `internal/bus` |
| P0 | [ ] | SQLite Schema & Migrations | `internal/registry` |
| P0 | [ ] | Device Registry (CRUD, Timeouts) | `internal/registry` |
| P0 | [ ] | ARP-Collector (gopacket) | `internal/collectors/arp` |
| P0 | [ ] | OUI-Datenbank (Download + Query) | `internal/registry` |
| P0 | [ ] | Alert Engine (Rules + Dedup) | `internal/alerting/engine` |
| P0 | [ ] | ntfy-Channel | `internal/alerting/channels` |
| P0 | [ ] | Web UI: Dashboard (read-only) | `internal/web` |
| P0 | [ ] | Web UI: Device List | `internal/web` |
| P0 | [ ] | Dockerfile | `/` |
| P0 | [ ] | docker-compose.yml | `/` |
| P0 | [ ] | Config-Loading mit Defaults | `internal/config` |
| P1 | [ ] | Auto-Interface-Detection | `cmd/silentmap` |
| P1 | [ ] | Graceful Shutdown | `cmd/silentmap` |

---

## Milestone 0.2 — Passive Collectors

> Ziel: mDNS und DHCP ergänzen das Bild — Hostnames und Gerätekategorien ohne Scan.

| Prio | Status | Aufgabe | Modul |
|---|---|---|---|
| P0 | [ ] | mDNS-Collector | `internal/collectors/mdns` |
| P0 | [ ] | DHCP-Collector | `internal/collectors/dhcp` |
| P1 | [ ] | Hostname-Merge (mDNS + DHCP → Registry) | `internal/registry` |
| P1 | [ ] | Web UI: Device Detail-Page | `internal/web` |
| P1 | [ ] | Web UI: Label & Priority setzen | `internal/web` |
| P2 | [ ] | Telegram-Channel | `internal/alerting/channels` |
| P2 | [ ] | Webhook-Channel (generic) | `internal/alerting/channels` |
| P2 | [ ] | REST API v1 (read-only) | `internal/web` |

---

## Milestone 0.3 — KI-Engine

> Ziel: Automatische Geräteklassifikation und intelligente Alert-Korrelation.

| Prio | Status | Aufgabe | Modul |
|---|---|---|---|
| P0 | [ ] | ONNX Fingerprint-Modell integrieren | `internal/ai/fingerprint` |
| P0 | [ ] | Trainings-Dataset aufbauen (MAC OUI + Patterns) | `docs/ai-dataset` |
| P1 | [ ] | Ollama-Client implementieren | `internal/ai/correlation` |
| P1 | [ ] | Alert-Korrelation (90s Fenster) | `internal/ai/correlation` |
| P1 | [ ] | Statistisches Anomalie-Baseline-Modell | `internal/ai/anomaly` |
| P2 | [ ] | KI-Insights in Web UI anzeigen | `internal/web` |
| P2 | [ ] | Konfidenz-Anzeige bei Gerätekategorie | `internal/web` |

---

## Milestone 0.4 — Aktive Module

> Ziel: Optionale aktive Checks für präzisere Liveness-Erkennung.

| Prio | Status | Aufgabe | Modul |
|---|---|---|---|
| P0 | [ ] | Ping-Collector (ICMP, für Priority-Devices) | `internal/collectors/ping` |
| P1 | [ ] | Nmap-Light (on-demand bei neuen Geräten) | `internal/collectors/nmap` |
| P1 | [ ] | Nmap-Output in Registry mergen | `internal/registry` |
| P2 | [ ] | Port-Liste in Device-Detail anzeigen | `internal/web` |

---

## Milestone 0.5 — Polishing & Deployment

> Ziel: Produktionsreif, gut dokumentiert, einfach zu deployen.

| Prio | Status | Aufgabe | Modul |
|---|---|---|---|
| P0 | [ ] | E-Mail-Channel (SMTP) | `internal/alerting/channels` |
| P0 | [ ] | Raspberry Pi ARM-Build | `scripts/` |
| P1 | [ ] | Alert-Regeln per Web UI konfigurierbar | `internal/web` |
| P1 | [ ] | Backup/Restore Script | `scripts/` |
| P1 | [ ] | Health-Check Endpoint (`/health`) | `internal/web` |
| P2 | [ ] | Optional: Basic Auth für Web UI | `internal/web` |
| P2 | [ ] | Metriken-Endpoint für Prometheus (optional) | `internal/web` |
| P2 | [ ] | Dark Mode Web UI | `internal/web` |

---

## Ideen-Pool (noch nicht bewertet)

- SNMP-Collector für Switches/Router
- NetFlow/sFlow-Collector
- Mobile App (ntfy reicht eigentlich)
- Gerätebild aus Fingerprint-Kategorie
- Export: CSV / JSON
- Verlaufs-Graph: Geräte online über Zeit
- Automatische OUI-Updates (wöchentlich)
- Community Fingerprint-Dataset

---

## Entschieden: NICHT umsetzen

| Feature | Begründung |
|---|---|
| Vulnerability-Scanning | Scope, andere Tools besser geeignet |
| Firewall-Integration | Widerspricht passivem Ansatz |
| Multi-User / RBAC | Zielgruppe braucht das nicht |
| Netzwerk-Topologie-Graph | Aufwand > Nutzen für Zielgruppe |
| Traffic-Analyse / DPI | Datenschutz, Komplexität |
