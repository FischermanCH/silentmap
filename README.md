# silentmap

**Passiver Netzwerk-Monitor** — sieht alles, stört nichts.

silentmap lauscht passiv im Netzwerk, erkennt Geräte durch deren eigenen Traffic und sendet Alarme bei neuen Geräten oder wenn Prioritäts-Geräte ausfallen. Kein aktives Scannen, kein komplexes Setup.

## Schnellstart

```bash
docker run -d \
  --net=host \
  --cap-add=NET_RAW \
  -v ./data:/data \
  silentmap/silentmap
```

Web UI erreichbar unter `http://localhost:8080`

## Features

- **100% passiv** — kein Ping, kein Scan, kein Netzwerklärm
- **Zero-Config** — funktioniert ohne Konfigurationsdatei
- **Modulares Design** — optionale aktive Module (Ping, Nmap) zuschaltbar
- **KI-gestützt** — Geräteerkennung, Alert-Korrelation, Anomalieerkennung
- **Einfaches Inventory** — MAC, IP, Hostname, Hersteller, Labels
- **Flexibles Alerting** — ntfy, Telegram, Webhook, E-Mail

## Dokumentation

| Dokument | Inhalt |
|---|---|
| [Ziele & Zweck](docs/goals.md) | Vision, Abgrenzung, Zielgruppe |
| [Architektur](docs/architecture.md) | Technischer Aufbau, Module, Event Bus |
| [Konfiguration](docs/configuration.md) | Alle Config-Optionen mit Defaults |
| [Alerting](docs/alerting.md) | Alert-Kanäle, Regeln, Dedup-Logik |
| [KI-Engine](docs/ai.md) | Fingerprinting, Korrelation, Anomalie |
| [Module](docs/modules/) | Dokumentation je Collector-Modul |
| [Deployment](docs/deployment/) | Docker, Bare-Metal, Raspberry Pi |
| [Backlog](docs/backlog.md) | Geplante Features und Prioritäten |
| [Changelog](CHANGELOG.md) | Versionshistorie |
| [Contributing](docs/contributing.md) | Wie man beiträgt |

## Projektstruktur

```
silentmap/
├── cmd/silentmap/          # Einstiegspunkt
├── internal/
│   ├── bus/                # Event Bus
│   ├── registry/           # Device Registry (SQLite)
│   ├── collectors/         # Collector-Module
│   │   ├── arp/            # ARP-Sniffer (passiv)
│   │   ├── mdns/           # mDNS/Bonjour (passiv)
│   │   ├── dhcp/           # DHCP-Sniffer (passiv)
│   │   ├── ping/           # Ping-Watchdog (aktiv, optional)
│   │   └── nmap/           # Nmap-Scanner (aktiv, optional)
│   ├── alerting/           # Alert-System
│   │   ├── engine/         # Regelauswertung & Dedup
│   │   ├── rules/          # Alert-Regeln
│   │   └── channels/       # ntfy, Telegram, Webhook, Mail
│   ├── ai/                 # KI-Engine
│   │   ├── fingerprint/    # Geräteklassifikation (ONNX)
│   │   ├── correlation/    # Alert-Korrelation (Ollama)
│   │   └── anomaly/        # Anomalieerkennung (statistisch)
│   └── web/                # Web UI
├── configs/                # Beispiel-Konfigurationen
├── docs/                   # Vollständige Dokumentation
└── scripts/                # Hilfsskripte
```

## Lizenz

MIT
