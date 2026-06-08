<p align="center">
  <img src="https://raw.githubusercontent.com/FischermanCH/silentmap/master/internal/web/static/silentmap-logo-01.png" alt="SilentMap" width="200">
</p>

# SilentMap — Home Network Monitor

**Passive LAN device discovery and monitoring for self-hosters and homelabs.**

SilentMap is a lightweight, self-hosted network monitor that automatically discovers every device on your home or office network — without active scanning. It listens to traffic your network already generates (ARP, mDNS, DHCP) and alerts you when something unexpected appears or a priority device goes offline.

> **No agent to install. No cloud. No noise. Runs on a Raspberry Pi.**

Single binary · Embedded SQLite · Zero external dependencies · Docker-native · amd64 + arm64

---

## Why SilentMap?

Most network monitors are either too complex (Zabbix, Nagios) or too simple (basic ping sweeps). SilentMap hits the sweet spot for home labs and small networks:

- **Sees everything passively** — phones, IoT devices, smart TVs, guests — without sending a single probe
- **Alerts that matter** — new device on your network? Priority device offline? You get notified via ntfy, Discord or Email
- **Runs anywhere** — Raspberry Pi, Proxmox, NAS, any Docker host; uses ~80MB RAM

---

## Screenshots

![Dashboard](https://raw.githubusercontent.com/FischermanCH/silentmap/master/internal/web/static/PS-SilentMap-Main.png)

| Devices | Alerts |
|---|---|
| ![Devices](https://raw.githubusercontent.com/FischermanCH/silentmap/master/internal/web/static/PS-SilentMap-Devices.png) | ![Alerts](https://raw.githubusercontent.com/FischermanCH/silentmap/master/internal/web/static/PS-SilentMap-Alerts.png) |

| Event Log | Settings |
|---|---|
| ![Log](https://raw.githubusercontent.com/FischermanCH/silentmap/master/internal/web/static/PS-SilentMap-Log.png) | ![Settings](https://raw.githubusercontent.com/FischermanCH/silentmap/master/internal/web/static/PS-SilentMap-Config.png) |

---

## Quickstart

```bash
docker run -d \
  --name silentmap \
  --network host \
  --cap-add NET_RAW \
  -v silentmap-data:/data \
  -e TZ=Europe/Zurich \
  fischermanch/silentmap:latest
```

Open **http://localhost:8080** — done. First run opens a setup page to set your password.

> `--network host` is required — SilentMap needs to see raw ARP, mDNS and DHCP traffic on your LAN.  
> `--cap-add NET_RAW` grants packet-capture without running as root.

---

## Docker Compose

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
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/health"]
      interval: 30s
      timeout: 5s
      retries: 3

volumes:
  silentmap-data:
```

---

## Features

| Feature | Details |
|---|---|
| **Passive discovery** | ARP · mDNS · DHCP — no active scanning by default |
| **Active monitoring** | Optional ICMP ping for priority devices · on-demand nmap port scan |
| **Device inventory** | Hostname · label · category · vendor (OUI) · groups · notes |
| **Topology map** | Interactive D3.js network graph with connection relationships |
| **Alert engine** | New device · priority device offline · device back online · HTTP service down |
| **Notification channels** | ntfy (push) · Discord (webhook) · Email (SMTP) |
| **HTTP monitoring** | Opt-in HTTP/HTTPS availability check for services on your network |
| **Authentication** | Single-operator password protection for the web UI |
| **Maintenance mode** | Pause alerts during planned downtime |
| **Export / Import** | Full JSON backup of device inventory, labels and groups |
| **Themes** | Dark · light · custom, switchable at runtime |
| **Bilingual** | German and English UI, switchable at runtime |
| **REST API** | `/health` · `/api/stats` · `/api/topology` · `/api/export` |

---

## Update

```bash
docker compose pull && docker compose up -d
```

Device inventory, labels, groups and settings survive every update — stored in the volume.

---

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `TZ` | UTC | Timezone for UI timestamps |
| `SILENTMAP_INTERFACE` | auto | Network interface to listen on |
| `SILENTMAP_LISTEN` | 0.0.0.0:8080 | Web UI bind address |

---

## Data

All persistent data is stored in `/data` inside the container:

| File | Purpose |
|---|---|
| `silentmap.db` | SQLite — all device data, events, alerts |
| `settings.json` | UI settings (channels, intervals, etc.) |
| `auth.hash` | bcrypt password hash |
| `secret.key` | AES-256 encryption key for stored secrets |

Mount a named volume or a host path (`/opt/silentmap:/data`).

> Do not store the volume on SMB/NFS shares — SQLite requires reliable file locking.

---

## Links

- **GitHub:** https://github.com/FischermanCH/silentmap — source code, documentation, changelog
- **Issues / Feature requests:** https://github.com/FischermanCH/silentmap/issues

---

## License

MIT — free to use, modify and distribute.
