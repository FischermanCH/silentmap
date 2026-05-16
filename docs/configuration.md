# Konfiguration

silentmap läuft ohne Konfigurationsdatei. Alle Einstellungen haben sinnvolle Defaults.  
Die Konfiguration wird beim ersten Start als `/data/silentmap.yaml` angelegt.

## Vollständige Konfigurationsreferenz

```yaml
# silentmap.yaml

# Netzwerk-Interface (leer = automatische Erkennung)
interface: ""

# Web UI
web:
  listen: "0.0.0.0:8080"
  auth:
    enabled: false
    username: "admin"
    password: ""          # Pflicht wenn auth.enabled = true

# Collector-Module
collectors:
  arp:
    enabled: true
    offline_timeout: 15m  # Gerät gilt nach X Minuten ohne ARP als offline

  mdns:
    enabled: true

  dhcp:
    enabled: true

  ping:
    enabled: false        # Aktiv — explizit opt-in
    targets: "priority"  # "priority" | "all" | spezifische MACs
    interval: 60s
    timeout: 3s

  nmap:
    enabled: false        # Aktiv — explizit opt-in
    trigger: "new_device" # "new_device" | "manual" | "scheduled"
    schedule: ""          # cron-Ausdruck wenn trigger = scheduled
    args: "-sV --top-ports 20"

# KI-Engine
ai:
  fingerprint:
    enabled: true         # ONNX Classifier, lokal, kein Ollama nötig
    model: ""             # Leer = eingebettetes Default-Modell

  correlation:
    enabled: true         # Alert-Korrelation via Ollama
    ollama_url: "http://localhost:11434"
    model: "phi3:mini"
    window: 90s           # Zeitfenster für Korrelation
    fallback: "passthrough" # Ohne Ollama: Alerts einzeln weiterleiten

  anomaly:
    enabled: true         # Statistisches Modell, kein LLM
    baseline_days: 14     # Wie viele Tage für Baseline
    min_observations: 50  # Mindest-Events bevor Erkennung aktiv

# Alerting
alerts:
  rules:
    new_device:
      enabled: true
      severity: "high"
      cooldown: 0s        # Immer sofort alarmieren

    priority_offline:
      enabled: true
      severity: "critical"
      threshold: 10m      # Offline seit X → Alert
      cooldown: 30m       # Nicht öfter als alle 30min

    device_back:
      enabled: true
      severity: "info"
      cooldown: 5m

    anomaly:
      enabled: true
      severity: "medium"
      min_score: 0.7      # Score-Schwelle (0.0–1.0)
      cooldown: 60m

  channels:
    ntfy:
      enabled: false
      url: "https://ntfy.sh/mein-topic"  # oder self-hosted
      token: ""           # Optional bei self-hosted mit Auth

    telegram:
      enabled: false
      token: ""
      chat_id: ""

    webhook:
      enabled: false
      url: ""
      method: "POST"
      headers: {}

    email:
      enabled: false
      smtp_host: ""
      smtp_port: 587
      smtp_user: ""
      smtp_pass: ""
      from: ""
      to: []

# Severity → Kanal Mapping
# Welche Severity geht an welche Kanäle
  routing:
    critical: ["ntfy", "telegram", "email"]
    high:     ["ntfy", "telegram"]
    medium:   ["webhook"]
    info:     []           # Nur im UI/Log
    low:      []
```

## Umgebungsvariablen

Alle Config-Werte können als Env-Var überschrieben werden.  
Schema: `SILENTMAP_` + Pfad in Großbuchstaben, Punkte als `_`.

```bash
SILENTMAP_INTERFACE=eth0
SILENTMAP_WEB_LISTEN=0.0.0.0:9090
SILENTMAP_ALERTS_CHANNELS_NTFY_URL=https://ntfy.sh/mein-topic
SILENTMAP_AI_CORRELATION_OLLAMA_URL=http://ollama:11434
```

## Prioritäts-Reihenfolge

1. Umgebungsvariablen
2. `silentmap.yaml` (aus `/data/` oder `--config` Pfad)
3. Eingebaute Defaults

## Kommandozeilen-Flags

```
silentmap [flags]

Flags:
  --config string     Pfad zur Konfigurationsdatei (default: /data/silentmap.yaml)
  --interface string  Netzwerk-Interface (überschreibt Config)
  --data string       Datenpfad für SQLite und Modelle (default: /data)
  --debug             Verbose Logging
  --version           Version ausgeben
```
