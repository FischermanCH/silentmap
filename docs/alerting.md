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
- **Trigger:** Gerät, das als offline galt, sendet wieder
- **Default-Severity:** `info`
- **Cooldown:** 5 Minuten
- **Alert-Inhalt:** Gerät, Offline-Dauer

### `anomaly`
- **Trigger:** KI-Anomaliemodul meldet Score > Schwelle
- **Default-Severity:** `medium`
- **Cooldown:** 60 Minuten
- **Alert-Inhalt:** Gerät, Score, Beschreibung der Auffälligkeit

## Dedup & Cooldown

Jeder Alert bekommt einen Fingerprint aus `type + MAC`. Gleicher Fingerprint innerhalb der Cooldown-Zeit wird verworfen.

**Flapping-Erkennung:** Gerät geht 3x innerhalb 5 Minuten on/off → einen kombinierten Alert statt drei separate.

## KI-Korrelation

Wenn Ollama konfiguriert ist, werden alle Alerts in einem 90-Sekunden-Fenster gesammelt und dem Modell übergeben:

**Prompt-Kontext (vereinfacht):**
```
Folgende Netzwerk-Events sind innerhalb von 90 Sekunden aufgetreten:
- [02:34:12] 8 Geräte offline
- [02:34:15] 3 weitere Geräte offline
- [02:35:40] Alle 11 Geräte wieder online

Bekannte Geräte: NAS, Router, 2x iPhone, iPad, Samsung TV, ...

Bitte: Sind das zusammengehörige Events? Was ist wahrscheinlich passiert?
Antworte mit: korreliert (ja/nein), Zusammenfassung (1 Satz), Handlungsbedarf (ja/nein)
```

**Output:**
```json
{
  "correlated": true,
  "summary": "Router-Neustart um 02:34 — alle Geräte kurz offline, selbst erholt.",
  "action_required": false,
  "severity_override": "info"
}
```

Ohne Ollama: alle Alerts werden einzeln weitergeleitet (Fallback: `passthrough`).

## Alert-Payload

Jeder Alert wird als strukturiertes JSON an alle Kanäle übergeben:

```json
{
  "id": "01J...",
  "type": "new_device",
  "severity": "high",
  "title": "Neues Gerät erkannt",
  "summary": "Apple iPhone — 192.168.1.44 — zuerst gesehen 14:23",
  "ai_context": "Wahrscheinlich Gast-Smartphone. Erstmals heute Nachmittag.",
  "correlated": false,
  "device": {
    "mac": "aa:bb:cc:dd:ee:ff",
    "ip": "192.168.1.44",
    "hostname": "Johns-iPhone",
    "vendor": "Apple",
    "category": "smartphone",
    "first_seen": "2026-05-16T14:23:05Z"
  },
  "timestamp": "2026-05-16T14:23:06Z"
}
```

## Kanäle

### ntfy (empfohlen)

Einfachste Einrichtung, Push auf iOS/Android, self-hostbar.

```yaml
channels:
  ntfy:
    enabled: true
    url: "https://ntfy.sh/mein-geheimes-topic"
```

Nachrichtenformat:
- **Title:** Alert-Titel
- **Body:** Summary + AI-Kontext
- **Priority:** critical=5, high=4, medium=3, info=2
- **Tags:** Geräte-Kategorie als Emoji-Tag

### Telegram

```yaml
channels:
  telegram:
    enabled: true
    token: "123456:ABC..."
    chat_id: "-100..."
```

Nachrichtenformat: Markdown mit Inline-Keyboard für Quick-Actions (Label setzen, als trusted markieren).

### Webhook (generic)

```yaml
channels:
  webhook:
    enabled: true
    url: "https://mein-server.de/silentmap-hook"
    method: "POST"
    headers:
      Authorization: "Bearer mein-token"
```

Body: vollständiger Alert-Payload als JSON.

### E-Mail (SMTP)

```yaml
channels:
  email:
    enabled: true
    smtp_host: "smtp.example.com"
    smtp_port: 587
    smtp_user: "alert@example.com"
    smtp_pass: "..."
    from: "silentmap <alert@example.com>"
    to: ["admin@example.com"]
```

Format: HTML-E-Mail mit Gerätedaten-Tabelle.

## Severity-Routing

```yaml
alerts:
  routing:
    critical: ["ntfy", "telegram", "email"]
    high:     ["ntfy", "telegram"]
    medium:   ["webhook"]
    info:     []
    low:      []
```
