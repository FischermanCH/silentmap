# Modul: Nmap-Collector

**Typ:** Aktiv · **Status:** Implementiert · **Pflicht:** Nein

## Zweck

Ergänzt bekannte Geräte mit Port-Informationen und OS-Fingerprint für genauere KI-Klassifikation. Wird nur on-demand oder geplant ausgeführt — kein kontinuierliches Scannen.

**Achtung:** Aktiv scannendes Modul. Kann in sensiblen Umgebungen Alarm auslösen oder Geräte belasten.

## Trigger-Modi

| Modus | Wann |
|---|---|
| `new_device` | Automatisch bei jedem neuen Gerät (empfohlen) |
| `manual` | Nur über Web UI (Button auf Geräte-Detailseite) |

## Konfiguration

```yaml
collectors:
  nmap:
    enabled: false
    trigger: "new_device"
    args: "-sV --top-ports 20 -T3"
```

## Was erkannt wird

- Offene Ports (Top 20 by default)
- Service-Versionen (wenn `-sV` aktiv)
- OS-Fingerprint (wenn `-O` aktiv, braucht root)

## Ergebnis-Speicherung

Scan-Ergebnisse werden direkt in der Datenbank gespeichert:
- `devices.nmap_ports` — JSON-Array, z.B. `["22/tcp open ssh OpenSSH 8.9", "80/tcp open http nginx 1.24"]`
- `devices.os_info` — Freitext OS-Erkennung

Beide Felder sind in der Geräte-Detailseite und im Topologie-Map-Tooltip sichtbar.

## Voraussetzungen

- `nmap` muss auf dem System installiert sein
- Für OS-Detection: Root-Rechte (in Docker vorhanden)

## Limitierungen

- Erzeugt sichtbaren Netzwerk-Traffic
- Scan kann 5–30 Sekunden dauern
- Manche Geräte reagieren empfindlich auf Port-Scans (Reboots, Logs)
- Nicht geeignet für Produktionsnetzwerke ohne Genehmigung
