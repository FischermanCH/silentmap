# SilentMap — Help

**Languages:** [English](#english) · [Deutsch](#deutsch)

---

<a name="english"></a>
# English

## What SilentMap does

SilentMap discovers devices passively from existing network traffic (ARP, mDNS, DHCP) and never probes unknown hosts. Once a device is known, it optionally uses lightweight ARP or ICMP to track its online/offline state.

---

## Installation

### Docker (recommended)

```bash
docker run -d \
  --name silentmap \
  --network host \
  --cap-add NET_RAW \
  -v silentmap-data:/data \
  -e TZ=Europe/Zurich \
  fischermanch/silentmap:latest
```

Open **http://localhost:8080**

`--network host` — required to see ARP, mDNS and DHCP traffic on your LAN.  
`--cap-add NET_RAW` — grants packet-capture without running as root.

### Docker Compose

```yaml
services:
  silentmap:
    image: fischermanch/silentmap:latest
    container_name: silentmap
    restart: unless-stopped
    network_mode: host
    cap_add:
      - NET_RAW
    volumes:
      - silentmap-data:/data
    environment:
      - TZ=Europe/Zurich

volumes:
  silentmap-data:
```

**Update:** `docker compose pull && docker compose up -d`

### Native Linux

```bash
git clone https://github.com/FischermanCH/silentmap
cd silentmap
go build -o silentmap ./cmd/silentmap
sudo setcap cap_net_raw+eip ./silentmap
./silentmap --data ./data
```

Requires Go 1.25+. Single static binary, no external dependencies.

---

## Configuration

All settings are optional — SilentMap works out of the box without a config file.

Place `silentmap.yaml` in your data directory:

```yaml
interface: ""           # empty = auto-detect

web:
  listen: "0.0.0.0:8080"

collectors:
  arp:
    offline_timeout: 15m   # time until a silent device is marked offline
  ping:
    enabled: true
    targets: "priority"    # ARP-poll only Priority devices ("all" = all known)
    interval: 5m

alerts:
  rules:
    new_device:
      enabled: true
      severity: "high"
    priority_offline:
      enabled: true
      severity: "critical"
      cooldown: 30m
    device_back:
      enabled: true
      severity: "info"
  channels:
    ntfy:
      enabled: true
      url: "https://ntfy.sh/your-topic"
      token: ""
    discord:
      enabled: false
      webhook_url: ""
  routing:
    critical: ["ntfy", "discord"]
    high:     ["ntfy"]
    info:     []
```

Full reference: [configs/silentmap.example.yaml](configs/silentmap.example.yaml)

---

## Features

### Device inventory

Every discovered device shows: IP, MAC, hostname, vendor (OUI lookup), category, label, and online/offline status. Click any device to open its detail page where you can set a label, category, priority flag, and run an on-demand nmap scan.

### Priority devices

Mark a device as **Priority** (★) on its detail page. SilentMap actively polls priority devices with ARP requests every 5 minutes (configurable via `collectors.ping.interval`) and triggers a `priority_offline` alert when they go offline. Use `collectors.ping.targets: "all"` to poll all known devices instead of just priority ones.

### Force Ping (ICMP)

For devices outside your local subnet (e.g. a router at a different IP range), enable **Force Ping** on the device detail page. SilentMap will use ICMP instead of ARP to monitor the device — ARP cannot cross subnet boundaries.

### Topology map

The **Dashboard** shows an interactive D3.js network graph. Devices are grouped by their assigned group (color-coded hulls). Use the toolbar to:
- Toggle link types: Auto · Physical · Logical
- Filter by status: Online · Offline · New
- Toggle groups on/off
- Click the crosshair to re-center the view

### Groups

Create groups under **Groups** and assign devices to them. Groups appear as colored areas on the map and can be toggled on/off in the map toolbar.

### Alerts

Alerts are triggered by rules and sent to configured channels (ntfy, Discord). The alert history is available under **Alerts**. Alerts are routed by severity — assign channels to each severity level in your config.

**ntfy setup:**
```yaml
alerts:
  channels:
    ntfy:
      enabled: true
      url: "https://ntfy.sh/your-secret-topic"
```

**Discord setup:**  
Create a webhook: Discord server → Settings → Integrations → Webhooks → New Webhook.
```yaml
alerts:
  channels:
    discord:
      enabled: true
      webhook_url: "https://discord.com/api/webhooks/..."
```

### Export / Import

On the **Devices** page, use **↓ Export** to download a full JSON backup of your device inventory (labels, hostnames, categories, priorities, groups, connections). Use **↑ Import** to restore or migrate to another instance.

### Listening toggle

The nav bar shows a **REC** indicator. Click it (via Settings) to pause passive discovery — useful when demoing the app or during maintenance.

---

## FAQ

**Devices are not being discovered.**  
Make sure you're using `--network host` (Docker) and `--cap-add NET_RAW`. SilentMap only sees devices that generate traffic — a completely quiet device may take a few minutes to appear after it sends its first ARP packet.

**A device shows as offline but it's actually online.**  
If the device is on a different subnet (e.g. your router), enable **Force Ping** on its detail page. ARP cannot cross subnet boundaries.

**The nmap scan fails.**  
nmap is pre-installed in the Docker image. For native installs: `apt install nmap` or `apk add nmap`.

**Does SilentMap send data to external servers?**  
No. All data stays local. The only external connections are the alert channels you configure.

**How do I update?**  
Docker: `docker compose pull && docker compose up -d`  
Native: pull the latest code, rebuild, restart.

---

**Issues / Feature requests:** [github.com/FischermanCH/silentmap/issues](https://github.com/FischermanCH/silentmap/issues)

---

<a name="deutsch"></a>
# Deutsch

## Was SilentMap macht

SilentMap erkennt Geräte passiv aus bestehendem Netzwerk-Traffic (ARP, mDNS, DHCP) und sendet nie Probes an unbekannte Hosts. Für bekannte Geräte werden optional leichtgewichtige ARP- oder ICMP-Anfragen verwendet um den Online/Offline-Status zu verfolgen.

---

## Installation

### Docker (empfohlen)

```bash
docker run -d \
  --name silentmap \
  --network host \
  --cap-add NET_RAW \
  -v silentmap-data:/data \
  -e TZ=Europe/Zurich \
  fischermanch/silentmap:latest
```

Web-UI: **http://localhost:8080**

### Docker Compose

```yaml
services:
  silentmap:
    image: fischermanch/silentmap:latest
    container_name: silentmap
    restart: unless-stopped
    network_mode: host
    cap_add:
      - NET_RAW
    volumes:
      - silentmap-data:/data
    environment:
      - TZ=Europe/Zurich

volumes:
  silentmap-data:
```

**Update:** `docker compose pull && docker compose up -d`

### Native Linux

```bash
git clone https://github.com/FischermanCH/silentmap
cd silentmap
go build -o silentmap ./cmd/silentmap
sudo setcap cap_net_raw+eip ./silentmap
./silentmap --data ./data
```

Benötigt Go 1.25+. Einzelne statische Binary, keine externen Abhängigkeiten.

---

## Konfiguration

Alle Einstellungen sind optional — SilentMap läuft ohne Konfigurationsdatei.

`silentmap.yaml` im Data-Verzeichnis ablegen:

```yaml
interface: ""           # leer = automatische Erkennung

collectors:
  ping:
    enabled: true
    targets: "priority"    # nur Prioritäts-Geräte pingen
    interval: 5m

alerts:
  channels:
    ntfy:
      enabled: true
      url: "https://ntfy.sh/dein-topic"
    discord:
      enabled: false
      webhook_url: ""
```

Vollständige Referenz: [configs/silentmap.example.yaml](configs/silentmap.example.yaml)

---

## Features

### Geräte-Inventar

Jedes erkannte Gerät zeigt: IP, MAC, Hostname, Hersteller (OUI), Kategorie, Label und Online/Offline-Status. Klick auf ein Gerät öffnet die Detailseite mit Label, Kategorie, Priorität, nmap-Scan.

### Prioritäts-Geräte

Gerät als **Priorität** (★) markieren. SilentMap sendet aktiv ARP-Requests an Prioritäts-Geräte (Standard: alle 5 Min., konfigurierbar via `collectors.ping.interval`) und löst bei Ausfall einen `priority_offline`-Alarm aus. Mit `collectors.ping.targets: "all"` werden alle bekannten Geräte statt nur Prioritäts-Geräte abgefragt.

### Force Ping (ICMP)

Für Geräte ausserhalb des lokalen Subnetzes (z.B. Router in einem anderen IP-Bereich) **Force Ping** auf der Detailseite aktivieren. SilentMap verwendet dann ICMP statt ARP — ARP funktioniert nicht über Subnetz-Grenzen.

### Topologie-Map

Das **Dashboard** zeigt einen interaktiven D3.js-Netzwerkgraph. Geräte werden nach Gruppe organisiert (farbige Bereiche). Toolbar-Optionen:
- Link-Typen ein/aus: Auto · Physisch · Logisch
- Status-Filter: Online · Offline · Neu
- Gruppen ein/ausblenden
- Fadenkreuz-Button zum Zentrieren

### Gruppen

Unter **Gruppen** Gruppen erstellen und Geräte zuweisen. Gruppen erscheinen als farbige Bereiche auf der Map.

### Alarme

Alarme werden per Regel ausgelöst und an ntfy oder Discord gesendet. Die Alarm-Historie ist unter **Alarme** einsehbar.

### Export / Import

Auf der **Geräte**-Seite: **↓ Exportieren** für ein vollständiges JSON-Backup, **↑ Importieren** zum Wiederherstellen oder Migrieren.

---

## FAQ

**Geräte werden nicht erkannt.**  
Docker: `--network host` und `--cap-add NET_RAW` prüfen. SilentMap sieht nur Geräte die selbst Traffic senden — ruhige Geräte erscheinen erst nach ihrem ersten ARP-Paket.

**Ein Gerät wird als offline angezeigt, ist aber online.**  
Liegt das Gerät in einem anderen Subnetz (z.B. Router)? Dann **Force Ping** auf der Detailseite aktivieren — ARP funktioniert nicht über Subnetz-Grenzen.

**Gibt SilentMap Daten weiter?**  
Nein. Alle Daten bleiben lokal. Externe Verbindungen entstehen nur durch die konfigurierten Alert-Kanäle.

---

**Fragen / Feature-Wünsche:** [github.com/FischermanCH/silentmap/issues](https://github.com/FischermanCH/silentmap/issues)
