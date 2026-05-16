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
    MAC          string    (Primary Key)
    IP           string    (letzte bekannte)
    Hostname     string    (aus mDNS/DHCP)
    Vendor       string    (aus OUI-Datenbank)
    Label        string    (manuell, optional)
    Category     string    (KI-Fingerprint: "smartphone", "nas", ...)
    Priority     bool      (manuell, löst kritische Alerts aus)
    FirstSeen    time.Time
    LastSeen     time.Time
    Online       bool
    Meta         JSON      (collector-spezifische Felder)
}
```

## KI-Engine

Drei unabhängige Sub-Module:

### 1. Fingerprinting (`internal/ai/fingerprint/`)
- **Input:** MAC-OUI + Hostname + mDNS-Services + offene Ports
- **Modell:** ONNX Classifier (~5MB, lokal, keine GPU)
- **Output:** Kategorie + Konfidenz (z.B. `"smartphone" 0.94`)
- **Trigger:** Bei jedem `device.new` und wenn neue Meta-Daten ankommen

### 2. Alert-Korrelation (`internal/ai/correlation/`)
- **Input:** Alert-Events im 90-Sekunden-Fenster + Gerätekontext
- **Modell:** Phi-3 mini via Ollama (optional, konfigurierbar)
- **Output:** Korreliertes Alert-Event mit menschenlesbarer Zusammenfassung
- **Fallback:** Ohne Ollama werden Alerts einzeln weitergeleitet

### 3. Anomalieerkennung (`internal/ai/anomaly/`)
- **Input:** Aktivitäts-Zeitstempel je Gerät (letzte 30 Tage)
- **Modell:** Statistisches Baseline-Modell (kein LLM nötig)
- **Output:** `anomaly.detected` Event mit Score und Beschreibung
- **Trigger:** Bei `device.seen` Events außerhalb der gelernten Aktivitätsfenster

## Alerting-Pipeline

```
Event Bus
    │ device.new / device.lost / ai.insight
    ▼
Alert Rules Engine          ← YAML-Regeln + Defaults
    │
    ▼
Dedup & Cooldown Layer      ← verhindert Alert-Flut
    │
    ▼
AI Korrelation              ← bündelt verwandte Alerts (optional)
    │
    ▼
Channel Router              ← Severity → Kanal-Mapping
    │
    ├── ntfy
    ├── Telegram
    ├── Webhook (generic)
    └── E-Mail (SMTP)
```

## Web UI

Server-Side Rendering mit HTMX — kein JavaScript-Framework.

| Route | Inhalt |
|---|---|
| `GET /` | Dashboard — Online/Offline-Übersicht, letzte Events |
| `GET /devices` | Inventory — alle Geräte, filterbar/sortierbar |
| `GET /devices/:mac` | Geräte-Detail — History, Labels, Priority setzen |
| `GET /alerts` | Alert-Log + Channel-Konfiguration |
| `GET /settings` | Module aktivieren/deaktivieren, Config |
| `GET /api/v1/*` | REST API für externe Integration |

## Dateistruktur im Betrieb

```
/data/
├── silentmap.db        # SQLite — alle Gerätedaten, Events, Alerts
├── silentmap.yaml      # Konfiguration (auto-erstellt mit Defaults)
├── oui.db              # MAC OUI Datenbank (auto-download)
└── models/
    └── fingerprint.onnx  # KI-Modell für Geräteklassifikation
```

## Technologie-Entscheidungen

| Komponente | Wahl | Begründung |
|---|---|---|
| Sprache | Go | Single binary, exzellente libpcap-Bindings, geringer RAM-Verbrauch |
| Paket-Sniffer | gopacket + libpcap | Stabil, gut dokumentiert, AF_PACKET-Support |
| Datenbank | SQLite (modernc) | Kein Daemon, Backup = cp, CGO-frei möglich |
| Web-Framework | chi Router + html/template | Leichtgewichtig, stdlib-nah |
| Frontend | HTMX + Tailwind CSS (CDN) | Kein Build-Step, kein JS-Framework |
| KI Inference | ONNX Runtime (Go binding) | Plattformunabhängig, keine Python-Dependency |
| LLM | Ollama HTTP API | Lokales Modell, optional, einfache Integration |
| Container | Alpine + libpcap | ~25MB Image |
