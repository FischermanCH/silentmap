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
  # Authentifizierung wird nicht via YAML konfiguriert.
  # Beim ersten Start ohne $DATA_DIR/auth.hash leitet jeder Request auf /setup.
  # Passwort wird als bcrypt-Hash in auth.hash gespeichert.
  # Ändern via Settings → General oder /setup.

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
    enabled: true         # Aktiv-Ping für Priority-Devices
    targets: "priority"   # "priority" (nur Prio-Geräte) | "all"
    interval: 5m
    timeout: 3s

  nmap:
    enabled: false        # Aktiv — explizit opt-in
    trigger: "new_device" # "new_device" | "manual"
    args: "-sV --top-ports 20 -T3"

# KI-Engine (Konfigurationsstruktur vorhanden, Logik noch nicht implementiert)
ai:
  fingerprint:
    enabled: true

  correlation:
    enabled: false
    ollama_url: "http://localhost:11434"
    model: "phi3:mini"
    window: 90s

  anomaly:
    enabled: true
    baseline_days: 14
    min_observations: 50

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
      severity: "high"   # nur für Priority-Geräte
      cooldown: 5m

    service_down:
      enabled: true
      severity: "high"   # für Devices mit category == "http-service"
      cooldown: 15m

    service_back:
      enabled: true
      severity: "high"
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
      token: ""           # optional bei self-hosted mit Auth

    discord:
      enabled: false
      webhook_url: ""     # Discord Webhook URL (verschlüsselt in settings.json)

    email:
      enabled: false
      smtp_host: ""
      smtp_port: 587      # 465 = TLS, 587 = STARTTLS, sonstige = plain
      smtp_user: ""
      smtp_pass: ""       # Wird verschlüsselt gespeichert (AES-256-GCM)
      from: ""
      to: ""
      tls_mode: "starttls" # "starttls" | "tls" | "none"
      lang: "de"           # "de" | "en"

    # webhook: noch nicht implementiert

# Severity → Kanal Mapping
  routing:
    critical: ["ntfy", "discord", "email"]
    high:     ["ntfy", "discord", "email"]
    medium:   []
    info:     []
    low:      []
```

## Prioritäts-Reihenfolge

1. `silentmap.yaml` (aus `/data/` oder `--config` Pfad)
2. Eingebaute Defaults

Env-Var-Override ist nicht implementiert.

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
