# Architektur

## Übersicht

silentmap ist um einen zentralen **Event Bus** aufgebaut. Alle Komponenten kommunizieren ausschließlich über Events — kein direkter Aufruf zwischen Modulen.

```
┌─────────────────────────────────────────────────────────────────┐
│                         silentmap                               │
│                                                                 │
│  ┌──────────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │    Collectors    │    │  Event Bus   │    │  Consumers   │  │
│  │                  │───▶│              │───▶│              │  │
│  │  arp (passiv)    │    │ device.seen  │    │  Registry    │  │
│  │  mdns (passiv)   │    │ device.new   │    │  AI Engine   │  │
│  │  dhcp (passiv)   │    │ device.lost  │    │  Alerter     │  │
│  │  ping (aktiv)    │    │ device.back  │    │  Web UI      │  │
│  │  nmap (aktiv)    │    │ alert.fire   │    │              │  │
│  │  [custom]        │    │ ai.insight   │    │              │  │
│  └──────────────────┘    └──────────────┘    └──────────────┘  │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                   SQLite (shared state)                   │  │
│  │   devices | events | alerts | ai_labels | config         │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

## Event Bus

Der Event Bus ist der Kern des Systems. Er ist synchron mit optionaler Async-Queue für langsame Consumer (KI, Alerting).

### Event-Typen

| Event | Sender | Empfänger | Bedeutung |
|---|---|---|---|
| `device.seen` | Collector | Registry, AI | Bekanntes Gerät sichtbar |
| `device.new` | Registry | Alerter, AI | Gerät erstmals gesehen |
| `device.lost` | Registry | Alerter, AI | Gerät nicht mehr sichtbar |
| `device.back` | Registry | Alerter | Gerät wieder online |
| `device.updated` | Registry | Web UI | Gerätedaten geändert |
| `alert.fire` | Alert Engine | Channels | Alert soll versendet werden |
| `ai.insight` | AI Engine | Registry, Web UI | KI-Erkenntnis verfügbar |

### Event-Struktur

```go
type Event struct {
    ID      string
    Type    string
    Time    time.Time
    MAC     string
    IP      string
    Meta    map[string]any  // collector-spezifische Felder
    Source  string          // "arp", "mdns", "dhcp", ...
}
```

## Collector-Interface

Jedes Modul implementiert dieses Interface:

```go
type Collector interface {
    Name()    string
    Start(ctx context.Context, bus EventBus) error
    Stop()    error
    Config()  CollectorConfig
}
```

Collector werden beim Start registriert und können zur Laufzeit (de)aktiviert werden.

## Device Registry

Zentrale Datenhaltung in SQLite. Verantwortlich für:
- Deduplizierung von `device.seen` Events
- Erkennung von neuen vs. bekannten Geräten
- Timeout-Erkennung (Gerät gilt als offline nach X Minuten ohne Event)
- Persistierung aller Gerätedaten

### Device-Modell

```
Device {
    MAC          string    (Primary Key, normalisiert AA:BB:CC:DD:EE:FF)
    IP           string    (letzte bekannte)
    Hostname     string    (manuell gesetzt, UI)
    HostnameAuto string    (automatisch via mDNS/DHCP/PTR)
    Vendor       string    (aus OUI-Datenbank)
    Label        string    (Freitext-Label, optional)
    Category     string    (z.B. "smartphone", "nas", "router")
    Services     []string  (mDNS-Dienste, z.B. ["_airplay._tcp"])
    NmapPorts    []string  (offene Ports, z.B. ["22/tcp open ssh"])
    OsInfo       string    (nmap OS-Erkennung)
    ForcePing    bool      (ICMP statt ARP für Geräte ausserhalb Subnet)
    Priority     bool      (manuell, löst kritische Alerts aus)
    Approved     bool      (neue Geräte starten mit false)
    Online       bool
    FirstSeen    time.Time
    LastSeen     time.Time
}
```

## KI-Engine

Noch nicht implementiert — in Planung. Die Config-Struktur ist vorhanden,
die Logik noch nicht. Konfigurierbar unter `ai.*` in `silentmap.yaml`.

## Alerting-Pipeline

```
Event Bus
    │ device.new / device.lost / device.back
    ▼
Alert Rules Engine          ← eingebaute Regeln + Cooldown
    │
    ▼
Dedup & Cooldown Layer      ← verhindert Alert-Flut
    │
    ▼
Channel Router              ← Severity → Kanal-Mapping
    │
    ├── ntfy        (implementiert)
    ├── Discord     (implementiert)
    ├── Webhook     (geplant)
    └── E-Mail      (geplant)
```

## Web UI

Server-Side Rendering mit HTMX — kein JavaScript-Framework.

| Route | Inhalt |
|---|---|
| `GET /` | Dashboard — Online/Offline-Übersicht, letzte Events |
| `GET /devices` | Inventory — alle Geräte |
| `GET /devices/:mac` | Geräte-Detail — History, Labels, nmap, Priority |
| `GET /groups` | Gruppen verwalten, Geräte zuweisen |
| `GET /alerts` | Alert-Log |
| `GET /log` | Aktivitäts-Log aller Geräte |
| `GET /settings` | Discord, ntfy, Ping, Theme, Sprache |
| `GET /api/topology` | Topologie-Daten für D3-Map (JSON) |
| `GET /api/export` | Export aller Geräte + Gruppen als JSON |
| `POST /api/import` | Import eines Exports |
| `GET /api/stats` | Online-Zähler und Uptime |
| `GET /health` | Health-Check Endpoint |

## Dateistruktur im Betrieb

```
/data/
├── silentmap.db        # SQLite — alle Gerätedaten, Events, Alerts
└── silentmap.yaml      # Konfiguration (auto-erstellt mit Defaults)
```

OUI-Daten werden in die SQLite-DB integriert, kein separates File.

## Technologie-Entscheidungen

| Komponente | Wahl | Begründung |
|---|---|---|
| Sprache | Go | Single binary, exzellente libpcap-Bindings, geringer RAM-Verbrauch |
| Paket-Sniffer | gopacket + libpcap | Stabil, gut dokumentiert, AF_PACKET-Support |
| Datenbank | SQLite (modernc) | Kein Daemon, Backup = cp, CGO-frei möglich |
| Web-Framework | chi Router + html/template | Leichtgewichtig, stdlib-nah |
| Frontend | HTMX + Tailwind CSS (CDN) | Kein Build-Step, kein JS-Framework |
| Container | Alpine + nmap + nmap-scripts | ~25MB Image |
