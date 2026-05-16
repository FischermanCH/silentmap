# Modul: Nmap-Collector

**Typ:** Aktiv · **Status:** Geplant (Milestone 0.4) · **Pflicht:** Nein

## Zweck

Ergänzt bekannte Geräte mit Port-Informationen und OS-Fingerprint für genauere KI-Klassifikation. Wird nur on-demand oder geplant ausgeführt — kein kontinuierliches Scannen.

**Achtung:** Aktiv scannendes Modul. Kann in sensiblen Umgebungen Alarm auslösen oder Geräte belasten.

## Trigger-Modi

| Modus | Wann |
|---|---|
| `new_device` | Automatisch bei jedem neuen Gerät (empfohlen) |
| `manual` | Nur über Web UI oder API auslösbar |
| `scheduled` | Cron-Ausdruck, z.B. wöchentlicher Scan |

## Konfiguration

```yaml
collectors:
  nmap:
    enabled: false
    trigger: "new_device"
    schedule: ""
    args: "-sV --top-ports 20 -T3"
    timeout: 30s
```

## Was erkannt wird

- Offene Ports (Top 20 by default)
- Service-Versionen (wenn `-sV` aktiv)
- OS-Fingerprint (wenn `-O` aktiv, braucht root)

## Events

```json
{
  "type": "device.seen",
  "mac": "aa:bb:cc:dd:ee:ff",
  "ip": "192.168.1.10",
  "source": "nmap",
  "meta": {
    "ports": [
      {"port": 22, "service": "ssh", "version": "OpenSSH 8.9"},
      {"port": 80, "service": "http", "version": "nginx 1.24"}
    ],
    "os_guess": "Linux 5.x",
    "os_confidence": 85
  }
}
```

## Voraussetzungen

- `nmap` muss auf dem System installiert sein
- Für OS-Detection: Root-Rechte (in Docker vorhanden)

## Limitierungen

- Erzeugt sichtbaren Netzwerk-Traffic
- Scan kann 5–30 Sekunden dauern
- Manche Geräte reagieren empfindlich auf Port-Scans (Reboots, Logs)
- Nicht geeignet für Produktionsnetzwerke ohne Genehmigung
