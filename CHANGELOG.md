# Changelog

All notable changes are documented here.
Format based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versioning follows [Semantic Versioning](https://semver.org/).

---

## [Unreleased]

- Webhook alert channel
- Email alert channel
- Basic auth for web UI
- Multi-platform Docker builds (arm64)

---

## [1.0.9] — 2026-06-03

### Fixed
- **Topology map — delayed centering on load** — the map now centres correctly within the first render instead of drifting for several seconds. Previously the D3 simulation started at full alpha and `fitView` only fired once `alpha` decayed below 0.05 (~75 ticks / 1–2 s), during which nodes flew around off-screen. The simulation is now pre-ticked synchronously before the first paint so nodes are already in their near-final positions when the canvas appears. The same fix applies when filters are toggled (`rebuildSim`), replacing the previous 150 ms `setTimeout` hack with the same pre-tick approach.

---

## [1.0.8] — 2026-06-02

### Fixed
- **Discord alert — device back online** — priority devices now send a Discord notification when they come back online. The `device_back` rule was previously severity `info`, which the routing never forwarded to Discord; raised to `high` (matching `new_device`). The handler now also checks the priority flag — only priority devices trigger this alert, mirroring the existing `priority_offline` behaviour.
- **Topology map — connection lines lost when online filter is active** — virtual nodes (e.g. a wireless link node) are no longer hidden by the online/offline status filter, so their connections stay visible regardless of the filter state.
- **Topology map — ghost areas when hiding nodes** — toggling the online, offline, or new-device filter now triggers a full D3 simulation rebuild using only the visible nodes. The layout re-calculates clusters and forces from scratch, eliminating the empty space previously left behind by hidden nodes.

### Changed
- **Internal cleanup** — six near-identical device-update HTTP handlers unified into a single `deviceUpdate` helper; JS node-scatter logic extracted into a `scatterNew` helper; `COLORS` object extended with `danger`/`warning` entries used by status dots and filter buttons instead of hard-coded hex values; ignored `json.Unmarshal`/`MarshalIndent` errors in the settings handler now logged/returned; Makefile Docker image name corrected to `fischermanch/silentmap`.

---

## [1.0.7] — 2026-05-22

### Added
- **Device search** — live search box above the device table filters by name, IP or MAC. Works alongside the existing status filter (`?status=online` etc.). Press `/` to focus the search box from anywhere on the page.

---

## [1.0.6] — 2026-05-22

### Changed
- **Device activity log** — completely redesigned for readability: event type is now shown as a clear label ("Online", "Offline", "Services", etc.) with colour coding (green/red for online/offline). Source labels are human-friendly ("Poller" instead of "registry", "mDNS" instead of "mdns", "Manual" instead of "web"). Import events no longer log noisy "updated" entries.

---

## [1.0.5] — 2026-05-20

### Changed
- **Groups page — devices sorted by IP** — devices within each group are now listed in ascending IP order
- **Groups page — reorder groups** — ▲/▼ buttons in each group header let you change the display order; order is persisted in the database

---

## [1.0.4] — 2026-05-20

### Added
- **Map settings panel** — gear icon (⚙) in the map toolbar opens a live settings panel. Adjust repulsion, link distance, collision radius and cluster pull with sliders; changes apply instantly to the running simulation. Settings are stored server-side (shared across all devices/browsers) with localStorage as fast cache. On desktop: side panel slides in from the right. On mobile: bottom sheet slides up.

---

## [1.0.3] — 2026-05-19

### Added
- **Category "virtual"** — devices assigned to this category are not monitored (no ARP/ICMP polling, no online/offline tracking, no offline alerts). They appear with a ◆ badge and are excluded from all counters. Intended for logical network nodes like VLANs, network segments, or powerline groups — not for actual VMs (use "server" for those).
- **Topology map improvements** — 360° cluster distribution, hub-radial force (highly connected nodes pulled toward center), cluster anchor force (prevents small clusters from drifting off-screen)

### Changed
- **Device detail sidebar** — Priority, ICMP Ping, Scan, and Delete device are now collapsible accordion sections; Delete is always last
- **Removed parent devices feature** — the "parent device" relationship has been removed entirely; existing data is cleaned up automatically on first start

### Fixed
- **Virtual nodes on topology map** — no longer show a red offline dot; rendered at full opacity with category color
- **Virtual devices excluded from counters** — online, offline and total counts no longer include virtual devices
- **Fischerman theme** — button text was green on green background; fixed via `btn-text` theme variable
- **Mobile layout** — NEW banner on device detail page now stacks vertically on small screens

---

## [1.0.2] — 2026-05-18

### Added
- **Status filter on topology map** — filter nodes by Online / Offline / New
- **Favicon** — browser tab icon
- **Screenshots** in README and Docker Hub description

### Fixed
- **Translation** — "Neu" shown as "New" in EN locale on the topology map
- **Help link** — now points to HELP.md instead of external README anchor

### Docs
- HELP.md completely rewritten with installation, configuration and FAQ sections
- Docker Hub description and README cross-referenced

---

## [1.0.1] — 2026-05-18

> Internal version — no functional changes. CI pipeline and Docker Hub automation set up.

---

## [1.0.0] — 2026-05-18 — Initial public release

### Added
- **Passive discovery** via ARP sniffer, mDNS, DHCP snooping
- **Device inventory** — MAC, IP, hostname, OUI vendor lookup, labels, categories
- **Device groups** with color coding
- **Topology map** — interactive D3.js network graph with connections and group hulls
- **Priority devices** — configurable ARP polling (default: every 5 min)
- **On-demand nmap scan** per device
- **Alert engine** — rules for new devices, priority offline, device back online
- **ntfy integration** — push notifications via ntfy.sh or self-hosted
- **Discord integration** — webhook-based alerts
- **Export / Import** — full JSON export including groups and connections
- **Listening toggle** — pause passive discovery with one click (useful for demo instances)
- **Version display** — version and build time in UI footer and `/api/version`
- **Dark / light themes** — switchable at runtime, persisted per browser
- **Bilingual UI** — German and English, switchable at runtime
- **Zero-config** — works without any configuration file
- **Single binary** — no CGO, no external dependencies (uses modernc/sqlite)
- **Docker image** — multi-stage build, `fischermanch/silentmap:latest`
- **API** — `/health`, `/api/stats`, `/api/version`, `/api/topology`, `/api/export`, `/api/import`
- **Log rotation** — automatic cleanup after configurable retention period
