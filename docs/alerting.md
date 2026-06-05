# Alerting

## Übersicht

Das Alerting-System verarbeitet Events aus dem Event Bus, wendet Regeln an, dedupliziert und leitet Alerts an konfigurierte Kanäle weiter.

```
Event Bus → Rules Engine → Dedup/Cooldown → AI Korrelation → Channel Router → Kanäle
```

## Alert-Schweregrade

| Severity | Bedeutung | Standard-Kanäle |
|---|---|---|
| `critical` | Prio-Gerät offline, Sicherheitsrelevanz | Alle Kanäle |
| `high` | Neues unbekanntes Gerät | Push + Messenger |
| `medium` | Anomalie, Verhaltensänderung | Webhook |
| `info` | Gerät wieder online, Routine | Nur UI/Log |
| `low` | Statistiken, Berichte | Nur UI/Log |

## Eingebaute Alert-Regeln

### `new_device`
- **Trigger:** MAC noch nie zuvor gesehen
- **Default-Severity:** `high`
- **Cooldown:** keiner (jede neue MAC sofort)
- **Alert-Inhalt:** MAC, IP, Vendor, Hostname (falls bekannt), KI-Kategorie

### `priority_offline`
- **Trigger:** Als Priority markiertes Gerät sendet seit X Minuten kein Signal
- **Default-Severity:** `critical`
- **Default-Threshold:** 10 Minuten
- **Cooldown:** 30 Minuten (nicht jede Minute neu alarmieren)
- **Alert-Inhalt:** Gerät, Label, letzte gesehene IP, offline seit

### `device_back`
- **Trigger:** Priority-Gerät, das als offline galt, sendet wieder
- **Default-Severity:** `high`
- **Cooldown:** 5 Minuten
- **Alert-Inhalt:** Gerät, zuletzt offline seit
- **Wichtig:** Nur für als Priority markierte Geräte — gleich wie `priority_offline`

### `anomaly`
- **Trigger:** KI-Anomaliemodul meldet Score > Schwelle
- **Default-Severity:** `medium`
- **Cooldown:** 60 Minuten
- **Alert-Inhalt:** Gerät, Score, Beschreibung der Auffälligkeit

## Dedup & Cooldown

Jeder Alert bekommt einen Fingerprint aus `type + MAC`. Gleicher Fingerprint innerhalb der Cooldown-Zeit wird verworfen.

**Flapping-Erkennung:** Noch nicht implementiert — geplant.

## Alert-Payload

Das `Alert`-Struct das an alle Kanäle übergeben wird:

```go
Alert {
    ID       string
    Type     string         // "new_device" | "priority_offline" | "device_back"
    Severity string         // "critical" | "high" | "medium" | "info" | "low"
    Title    string         // i18n-Key, z.B. "alert.title.new_device"
    Summary  string         // Menschenlesbare Zusammenfassung
    MAC      string
    IP       string
    FiredAt  time.Time
    Meta     map[string]any // Geräte-Kontext: label, hostname, vendor, category, groups, ...
}
```

## Kanäle

### ntfy (implementiert)

Push auf iOS/Android, self-hostbar.

```yaml
channels:
  ntfy:
    enabled: true
    url: "https://ntfy.sh/mein-geheimes-topic"
    token: ""   # optional, für self-hosted mit Auth
```

### Discord (implementiert)

Webhook-Integration.

```yaml
channels:
  discord:
    enabled: true
    webhook_url: "https://discord.com/api/webhooks/..."
```

Nachrichtenformat: Embed mit Farbe nach Severity, Felder für IP, Hostname, Vendor, MAC, Gruppe.

### Webhook (geplant)

Noch nicht implementiert.

### E-Mail (geplant)

Noch nicht implementiert.

## Severity-Routing

```yaml
alerts:
  routing:
    critical: ["ntfy", "discord"]
    high:     ["ntfy", "discord"]
    medium:   []
    info:     []
    low:      []
```
