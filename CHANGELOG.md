# Changelog

All notable changes are documented here.
Format based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versioning follows [Semantic Versioning](https://semver.org/).

---

## [1.0.0] — 2026-05-18 — Initial public release

### Added
- **Passive discovery** via ARP sniffer, mDNS, DHCP snooping
- **Device inventory** — MAC, IP, hostname, OUI vendor lookup, labels, categories
- **Device groups** with color coding
- **Topology map** — visual network graph with parent/child relationships and connections
- **Priority devices** — configurable ping monitoring (default: every 5 min)
- **On-demand nmap scan** per device
- **Alert engine** — rules for new devices, priority offline, device back online
- **ntfy integration** — push notifications via ntfy.sh or self-hosted
- **Discord integration** — webhook-based alerts
- **Export / Import** — full JSON export including groups, parents, connections
- **Listening toggle** — pause passive discovery with one click (useful for demo instances)
- **Version display** — version and build time in UI footer and `/api/version`
- **Dark / light themes** — switchable at runtime, persisted per browser
- **Bilingual UI** — German and English, switchable at runtime
- **Zero-config** — works without any configuration file
- **Single binary** — no CGO, no external dependencies (uses modernc/sqlite)
- **Docker image** — multi-stage build, `fischerman/silentmap:latest`
- **API** — `/health`, `/api/stats`, `/api/version`, `/api/topology`, `/api/export`, `/api/import`
- **Log rotation** — automatic cleanup after configurable retention period

---

## [Unreleased]

- Webhook alert channel
- Email alert channel
- Basic auth for web UI
- Multi-platform Docker builds (arm64)
