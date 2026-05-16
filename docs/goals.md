# Ziele & Zweck

## Vision

silentmap ist ein passiver Netzwerk-Monitor für Heimnetzwerke und kleine Büros. Er soll ohne Konfiguration laufen, nichts im Netzwerk stören und dennoch ein vollständiges Bild aller Geräte liefern — ergänzt durch intelligente Alarme und KI-gestützte Einblicke.

## Kernziele

### 1. Passivität
Das System sendet **keinen eigenen Netzwerk-Traffic** im Grundbetrieb. Es lauscht ausschließlich auf den Traffic, den Geräte von selbst erzeugen (ARP, mDNS, DHCP). Aktive Module (Ping, Nmap) sind optional und explizit opt-in.

### 2. Zero-Config-Setup
```bash
docker run --net=host --cap-add=NET_RAW silentmap/silentmap
```
Das muss reichen. Keine Konfigurationsdatei, kein Datenbank-Setup, keine Netzwerk-Topologie manuell eingeben.

### 3. Modulare Erweiterbarkeit
Jedes Feature ist ein eigenständiges Modul. Neue Collector, Alert-Kanäle oder KI-Funktionen können hinzugefügt werden ohne den Core anzufassen. Das System wächst mit den Anforderungen.

### 4. Sinnvoller KI-Einsatz
KI wird nicht als Buzzword eingesetzt, sondern dort wo sie echten Mehrwert bringt:
- Geräte automatisch klassifizieren (kein manuelles Nachschlagen)
- Zusammengehörige Alerts bündeln statt Alarm-Flut
- Ungewöhnliches Verhalten erkennen ohne manuelle Regeln

### 5. Einfaches Inventory
Das Inventory zeigt was relevant ist: welches Gerät, welche IP, wann zuletzt gesehen, was ist es. Keine Port-Listen, keine CVE-Scans, keine Netzwerk-Graphen.

## Abgrenzung — Was silentmap bewusst NICHT ist

| Feature | Begründung |
|---|---|
| Vulnerability Scanner | Scope-Creep, andere Tools machen das besser (Nessus, OpenVAS) |
| Firewall / IDS | Aktiver Eingriff widerspricht dem passiven Ansatz |
| Traffic-Analyse / DPI | Datenschutz, Komplexität, andere Tools (ntopng, Zeek) |
| Netzwerk-Topology-Graph | Hoher Aufwand, wenig Mehrwert für Zielgruppe |
| Multi-User / Rollen | Heimnetz-Kontext, ein Admin reicht |
| Netzwerk-Konfiguration | Kein DHCP-Server, kein DNS-Resolver |

## Zielgruppe

**Primär:** Heimnetz-Nutzer mit technischem Hintergrund — wissen was ein Raspberry Pi ist, wollen aber kein NetAlertX konfigurieren.

**Sekundär:** Kleine Büros / Labs — bis ~200 Geräte, ein Verantwortlicher, kein dediziertes IT-Team.

## Erfolgskriterien

- Setup dauert unter 5 Minuten (Docker)
- Neue Geräte werden innerhalb von 60 Sekunden erkannt
- False-Positive-Rate bei Alerts unter 5%
- KI-Fingerprinting erkennt >80% der Geräte korrekt
- Speicherbedarf unter 512MB RAM, 1GB Disk für 1 Jahr History
