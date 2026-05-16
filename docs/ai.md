# KI-Engine

silentmap setzt KI gezielt ein — nicht als Buzzword, sondern dort wo echte Mehrwerte entstehen. Alle KI-Komponenten sind optional und haben sinnvolle Fallbacks.

## Übersicht

| Modul | Technologie | Benötigt Ollama? | Mehrwert |
|---|---|---|---|
| Fingerprinting | ONNX Classifier | Nein | Automatische Gerätekategorie |
| Alert-Korrelation | LLM (Phi-3 mini) | Ja (optional) | Bündelt Alert-Flut zu einem Satz |
| Anomalieerkennung | Statistisches Modell | Nein | Ungewöhnliche Aktivität erkennen |

---

## 1. Gerät-Fingerprinting (`internal/ai/fingerprint/`)

### Was es macht
Ordnet jedem Gerät automatisch eine Kategorie zu:

```
smartphone | tablet | laptop | desktop | smart-tv | nas | router |
printer | iot-device | gaming-console | media-player | smart-speaker | unknown
```

### Input-Features

| Feature | Quelle | Beispiel |
|---|---|---|
| MAC-OUI (Hersteller-Prefix) | ARP | `b8:27:eb` → Raspberry Pi Foundation |
| Hostname-Pattern | mDNS/DHCP | `ipad-pro`, `DESKTOP-ABC123` |
| mDNS-Services | mDNS | `_airplay._tcp`, `_smb._tcp` |
| Offene Ports (Top-20) | Nmap (optional) | 22, 80, 443, 548 |

### Modell
- Format: ONNX (Open Neural Network Exchange)
- Größe: ~5MB
- Inference: CPU, kein GPU nötig, <10ms pro Gerät
- Eingebettet in die Binary, kein externer Download nötig

### Output
```json
{
  "category": "nas",
  "confidence": 0.91,
  "reasoning": ["mac_oui:Synology", "mdns:_smb._tcp", "mdns:_afpovertcp._tcp"]
}
```

### Konfidenz-Schwellen
- `>= 0.85` → Kategorie wird gesetzt, in UI angezeigt
- `0.60–0.84` → Kategorie als "unsicher" markiert
- `< 0.60` → `unknown`, keine Anzeige

---

## 2. Alert-Korrelation (`internal/ai/correlation/`)

### Was es macht
Statt 10 separate Alerts bei einem Router-Neustart kommt eine einzige verständliche Meldung.

### Ablauf
1. Events werden in einem 90-Sekunden-Fenster gesammelt
2. Am Fenster-Ende: enthält das Fenster >2 ähnliche Events?
3. Ja: LLM-Anfrage an Ollama mit Event-Liste + Gerätekontext
4. LLM entscheidet: korreliert oder nicht, formuliert Zusammenfassung
5. Korrelierte Events → ein Alert; Einzelevents → einzelne Alerts

### LLM-Anforderungen
- Modell: `phi3:mini` (Standard), alternativ `llama3.2:3b`, `gemma2:2b`
- Ollama lokal oder per URL erreichbar
- Antwortzeit: typisch 1–3 Sekunden auf moderner Hardware

### Fallback ohne Ollama
Wenn Ollama nicht konfiguriert oder nicht erreichbar:
- Alerts werden direkt weitergeleitet ohne Korrelation
- Einfache Heuristik: >5 `device.lost` Events in 90s → ein gebündelter Alert mit Zähler

### Beispiel-Korrelationen

**Router-Neustart:**
```
Input:  11x device.lost + 11x device.back innerhalb 3 Minuten
Output: "Router-Neustart um 02:34 — 11 Geräte kurz offline, alle wieder erreichbar."
Severity override: info (statt critical)
```

**Gäste kommen:**
```
Input:  3x device.new (Apple-Geräte) innerhalb 60 Sekunden
Output: "3 neue Apple-Geräte gleichzeitig — wahrscheinlich Gäste."
Severity: bleibt high
```

---

## 3. Anomalieerkennung (`internal/ai/anomaly/`)

### Was es macht
Lernt das normale Verhalten jedes Geräts und schlägt Alarm bei Abweichungen.

### Baseline-Modell
Für jedes Gerät wird eine Aktivitäts-Baseline aufgebaut:
- Stündliche Aktivitäts-Buckets über 24h
- Minimale Beobachtungen: 50 Events (konfigurierbar)
- Baseline-Fenster: 14 Tage (gleitend)

```
NAS:        ████░░░░░░░░████████████░░    aktiv 06:00–00:00
iPhone-Jan: ░░░░░░░░████████████████░░    aktiv 07:00–23:00
IoT-Sensor: ████████████████████████████  immer aktiv
```

### Erkannte Muster

| Muster | Beispiel | Score |
|---|---|---|
| Aktivität außerhalb Normalzeit | NAS aktiv um 03:15 | 0.85 |
| Plötzliche Inaktivität | Always-on-Gerät 4h still | 0.75 |
| Neues Kommunikationsmuster | Gerät plötzlich sehr aktiv | 0.70 |

### Anomalie-Alert
```json
{
  "type": "anomaly",
  "device": "NAS (192.168.1.99)",
  "score": 0.85,
  "description": "NAS aktiv um 03:15 — normalerweise inaktiv zwischen 00:30 und 05:45.",
  "baseline_window": "14 Tage",
  "observations": 2847
}
```

---

## Ollama einrichten (optional)

```bash
# Ollama installieren
curl -fsSL https://ollama.com/install.sh | sh

# Modell herunterladen (~2GB)
ollama pull phi3:mini

# In silentmap.yaml konfigurieren
ai:
  correlation:
    enabled: true
    ollama_url: "http://localhost:11434"
    model: "phi3:mini"
```

Oder als Docker-Service in `docker-compose.yml` (bereits vorkonfiguriert, optional aktivierbar).

---

## Datenschutz

- Alle KI-Verarbeitung läuft **lokal** — keine Daten verlassen das System
- Ollama läuft lokal, keine Cloud-API
- ONNX-Modell ist eingebettet, kein externer Download zur Laufzeit
- Anomalie-Baseline bleibt in SQLite, kein externes Speichern
