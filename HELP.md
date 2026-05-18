# silentmap — Help / Hilfe

> EN and DE sections are interleaved. Jump to a section:
> [Installation](#installation) · [Configuration](#configuration) · [Alerts](#alerts) · [Export/Import](#exportimport) · [Docker](#docker) · [FAQ](#faq)

---

## Installation

### Docker (recommended)

```bash
docker run -d \
  --name silentmap \
  --net=host \
  --cap-add=NET_RAW \
  -v silentmap-data:/data \
  fischerman/silentmap:latest
```

`--net=host` is required so silentmap can see raw ARP, mDNS, and DHCP traffic.  
`--cap-add=NET_RAW` grants the packet-capture capability.

### Docker Compose

```yaml
services:
  silentmap:
    image: fischerman/silentmap:latest
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

### Native (Linux)

```bash
git clone https://github.com/fischerman/silentmap
cd silentmap
go build -o silentmap ./cmd/silentmap

# Grant packet-capture capability (no root required after this)
sudo setcap cap_net_raw+eip ./silentmap

./silentmap --data ./data
```

---

## Configuration

All settings are optional. silentmap works out of the box without any configuration file.

Place a `silentmap.yaml` in your data directory (e.g. `/data/silentmap.yaml`):

```yaml
# Network interface (empty = auto-detect)
interface: ""

web:
  listen: "0.0.0.0:8080"
  auth:
    enabled: false
    username: "admin"
    password: "changeme"

collectors:
  arp:
    enabled: true
    offline_timeout: 15m   # how long until a device is marked offline

  mdns:
    enabled: true          # hostname and service discovery

  dhcp:
    enabled: true          # DHCP hostname discovery

  ping:
    enabled: true
    targets: "priority"    # "priority" = only priority devices
    interval: 5m           # ping interval (default: 5 min)
    timeout: 3s

  nmap:
    enabled: false
    args: "-sV --top-ports 20 -T3"

storage:
  log_retention_days: 30   # delete events older than N days

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
      enabled: false
      url: "https://ntfy.sh/your-topic"
      token: ""            # optional Bearer token

    discord:
      enabled: false
      webhook_url: "https://discord.com/api/webhooks/..."

    webhook:
      enabled: false
      url: "https://your-webhook-endpoint"
      method: "POST"

    email:
      enabled: false
      smtp_host: "smtp.example.com"
      smtp_port: 587
      smtp_user: "alerts@example.com"
      smtp_pass: ""
      from: "silentmap@example.com"
      to: ["you@example.com"]

  routing:
    critical: ["ntfy", "discord", "email"]
    high:     ["ntfy", "discord"]
    medium:   ["webhook"]
    info:     []
    low:      []
```

---

*DE: Alle Einstellungen sind optional. silentmap läuft ohne Konfigurationsdatei.*

*Datei ablegen unter: `<data-verzeichnis>/silentmap.yaml`*

---

## Alerts

### ntfy

[ntfy](https://ntfy.sh) is a simple push notification service. You can use the public server or self-host.

```yaml
alerts:
  channels:
    ntfy:
      enabled: true
      url: "https://ntfy.sh/your-secret-topic"
      token: ""   # only needed for protected topics
```

### Discord

```yaml
alerts:
  channels:
    discord:
      enabled: true
      webhook_url: "https://discord.com/api/webhooks/YOUR_ID/YOUR_TOKEN"
```

Create a webhook: Discord server → Settings → Integrations → Webhooks → New Webhook.

### Routing

Alerts are routed by severity level. Assign channel names to severity buckets:

```yaml
alerts:
  routing:
    critical: ["ntfy", "discord"]
    high:     ["ntfy"]
    medium:   []
    info:     []
```

---

## Export/Import

silentmap can export all device data to a JSON file and import it into another instance. This is useful for migrating from a development setup to production.

**Export:** In the Devices view, click the **↓ Export** button. This downloads `silentmap-export.json`.

**Import:** Click the **↑ Import** button and select your JSON file.

The import updates labels, hostnames, categories, priorities, and approved status. Auto-discovered data (IP, vendor, hostname_auto) is only imported for devices that don't exist yet.

---

*DE: Export/Import ermöglicht die Migration zwischen Instanzen (z.B. Dev → Produktion).*

*Export: Taste **↓ Exportieren** auf der Geräte-Seite. Import: Taste **↑ Importieren**.*

---

## Docker

### Portainer Stack (local build)

Build the image on your server first:

```bash
cd silentmap
docker build -t silentmap:latest .
```

Then deploy via Portainer using the stack file at `portainer-stack.yml`.

### Capabilities

| Capability   | Required for              |
|--------------|---------------------------|
| `NET_RAW`    | ARP sniffing, mDNS, DHCP, ping, nmap |

### Data volume

All data is stored in `/data` inside the container:
- `silentmap.db` — SQLite database (devices, events, alerts)
- `silentmap.yaml` — optional configuration file

---

## FAQ

**Q: Does silentmap send any data to external servers?**  
A: No. All data stays local. The only external connections are alert notifications you explicitly configure (ntfy, Discord, webhook, email).

**Q: Why does silentmap need NET_RAW?**  
A: ARP packet capture, mDNS multicast, and DHCP sniffing all require raw socket access at the network layer. Without it, only the ping and nmap features work (and even those need a capable host).

**Q: The nmap scan says "nmap not found".**  
A: Install nmap in your environment. In the Docker image it is pre-installed. For native installs: `apt install nmap` or `apk add nmap`.

**Q: Devices are not being discovered.**  
A: Make sure you're running with `--net=host` (Docker) or with `CAP_NET_RAW` (native). silentmap only sees devices that generate traffic — quiet devices may take a while to appear.

**Q: How do I disable the active ping for priority devices?**  
A: Set `collectors.ping.enabled: false` in your configuration file.

---

*Weitere Fragen? → https://github.com/fischerman/silentmap/issues*
