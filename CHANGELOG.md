# Changelog

All notable changes are documented here.
Format based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versioning follows [Semantic Versioning](https://semver.org/).

---

## [Unreleased]

---

## [1.0.30] — 2026-06-06

### Changed
- **Multi-platform Docker builds** — `make docker` now uses `docker buildx build --platform linux/amd64,linux/arm64 --push`. The image runs natively on ARM64 (Raspberry Pi, Apple Silicon, ARM-based NAS). No code changes — the binary was already CGO-free (`modernc/sqlite` is pure Go). `make docker-push` is now a no-op since buildx pushes inline.

---

## [1.0.29] — 2026-06-06

### Added
- **Authentication** — single-operator password protection for the entire web UI. First-run setup page auto-redirects when no credentials are set ("OPERATOR CREDENTIAL SETUP"). Login page with terminal/spy aesthetic (green-on-black, scanlines, CRT vignette). Session cookie valid for 7 days. Password hashed with bcrypt and stored in `$DATA_DIR/auth.hash`. Logout in navigation dropdown.
- **Change password** — new section in Settings → General to update the operator password without losing the session.

---

## [1.0.28] — 2026-06-06

### Fixed
- **Settings page width** — settings page was constrained to `max-w-2xl` (~672px) while the rest of the UI uses `max-w-6xl`. Removed the inner constraint so settings fills the same width as all other pages.

---

## [1.0.27] — 2026-06-06

### Added
- **Gmail app-password hint** — when the SMTP test returns a Gmail 5.7.9 error ("Application-specific password required"), the error message is replaced with a readable explanation and a direct link to `myaccount.google.com/apppasswords`.

---

## [1.0.26] — 2026-06-06

### Fixed
- **Email test button — "SMTP host is required" despite host being entered** — the fetch call sent `multipart/form-data` which `r.ParseForm()` does not parse; converted to `URLSearchParams` so all SMTP fields reach the server correctly.

---

## [1.0.25] — 2026-06-06

### Added
- **SMTP connection test** — "Test connection" button in Settings → Alerts → Email. Clicking it sends a real test email using the current form values (or the stored password if the password field is left blank). Shows inline ✓/✗ result without saving. 15-second timeout with error message on failure.

---

## [1.0.24] — 2026-06-06

### Added
- **Email settings validation** — required fields (SMTP host, sender, recipient) and port range (1–65535) are validated server-side when email alerts are enabled. Invalid input shows a red error toast without losing the entered values.

---

## [1.0.23] — 2026-06-06

### Changed
- **Settings page tabs** — General, Network, and Alert Channels are now separate tabs instead of one long scrolling page. Active tab persists in `localStorage` and is restored after save.

---

## [1.0.22] — 2026-06-06

### Added
- **Email alerts** — new alert channel sending bilingual HTML emails via SMTP. Supports STARTTLS (port 587), direct TLS (port 465), and plain SMTP. Logo embedded inline (base64 data URI), alert-type colour-coded header, device info table, footer with GitHub link. Email language (DE/EN) is configurable independently of the UI language.
- **Secrets encrypted at rest** — Discord webhook URL, ntfy token, and SMTP password are now stored AES-256-GCM encrypted in `settings.json`. A random 32-byte key is generated on first start and stored in `secret.key` (inside the data directory, never committed). Legacy cleartext values are transparently decrypted on read and re-encrypted on next save.
- **Masked secrets in Settings UI** — sensitive fields (Discord webhook, ntfy token, SMTP password) show a "configured" badge and an empty password input instead of the stored value. Leaving the field blank on save preserves the existing secret; entering a new value replaces it.

---

## [1.0.21] — 2026-06-05

### Added
- **HTTP Service without IP** — when adding a device with category "HTTP Service", the IP field is optional. If neither IP nor MAC is provided, a UUID-based synthetic MAC is generated (locally administered, no conflict with real hardware). This allows multiple services on the same IP to be tracked as independent devices. The URL field appears directly in the modal when the HTTP Service category is selected.

### Fixed
- **Update indicator** — now checks the tags API endpoint instead of the releases endpoint. The releases endpoint requires an explicitly published GitHub Release; the tags endpoint responds immediately after `git push --tags`.

---

## [1.0.20] — 2026-06-05

### Added
- **Device notes** — free-text notes field on every device detail page. Stored in the database, shown only on the detail page (not in the device list or map tooltips).
- **Approve all new devices** — a button appears in the device list header whenever there are unapproved (NEW) devices. One click approves all of them at once with a confirmation dialog.
- **Alert suppression / maintenance mode** — a timer-based global pause for all alerts. Configurable in Settings → Alert Channels. Duration options: 30 min, 1 h, 2 h, 8 h. The active state survives server restarts (persisted in settings.json). While active, all alerts (priority offline, device back, service down/back, new device) are silently dropped.
- **HTTP service alerts** — new alert types `service_down` and `service_back` for devices in the "HTTP Service" category. Fired by the HTTP checker via the event bus, same cooldown logic as priority alerts. Both enabled by default, configurable in config.yaml.
- **Auto nmap on new device** — opt-in toggle in Settings → Network. When enabled, silentmap automatically schedules an nmap scan whenever a new device is discovered. Respects the existing scan mutex to avoid concurrent scans.
- **Topology map group filter: Shift+click isolate** — shift-clicking a group chip in the map hides all other groups and shows only the selected one. A "✕ ALL" reset button appears whenever any groups are hidden, restoring the full view.

### Changed
- `Device` struct: `Notes` field added, populated only by single-device `get()` (18-column scan). Bulk queries (`List`, `PriorityDevices`, etc.) remain at 17 columns — notes are empty in those contexts.

---

## [1.0.19] — 2026-06-05

### Added
- **HTTP/HTTPS service monitoring** — new category "HTTP Service" for monitoring web-based services (routers, NAS, home-assistant, reverse proxies, etc.). Per device, enter a URL (http:// or https://) in the new "HTTP-URL" field on the device detail page. A global HTTP checker polls all devices with a URL set at a configurable interval and publishes an online/offline event. Any HTTP response (including 4xx/5xx) counts as online — only timeouts and connection failures trigger offline. Self-signed TLS certificates are accepted without error (common on home networks).
- **HTTP Check is opt-in at two levels**: (1) global toggle + interval in Settings → Network, disabled by default; (2) per device, a URL must be explicitly entered — nothing is auto-activated.

### Changed
- Category "HTTP Service" appears in the category dropdown on device detail pages. A hint is shown explaining the two-step opt-in.
- Settings page has a new "HTTP Check" section under Network, mirroring the ARP Poller section.

---

## [1.0.18] — 2026-06-05

### Fixed
- **Add device — naked error page on invalid input** — entering an IP with leading/trailing whitespace (e.g. ` 192.168.1.1`) or any other invalid address no longer shows a raw HTTP error page. Whitespace is now trimmed silently on the server; if the address is genuinely invalid, the user is redirected back to `/devices` and a red toast popup appears at the bottom of the screen with the error message. Auto-dismisses after 6 seconds.

---

## [1.0.17] — 2026-06-05

### Added
- **Update indicator** — a small pulsing amber dot appears in the map toolbar when a newer release is available on GitHub. Clicking it opens the releases page. The server checks the GitHub API once at startup and every 6 hours; no indicator shown if the check fails or the version is current.
- **Map resize handle** — a drag handle at the bottom edge of the topology map lets you resize it vertically. The preferred height is saved in `localStorage` and restored on next visit. The map also responds to window resize via `ResizeObserver` and re-fits the view automatically.

### Changed
- **Map default height** — desktop map height changed from `78vh` to `calc(100vh - 56px - 2rem)`, filling the available viewport consistently across all screen sizes.

---

## [1.0.16] — 2026-06-05

### Fixed
- **Discord — no alert when priority device comes back online** — `EventDeviceBack` was published without the `priority` field in its metadata. The alert engine checks `priority` before firing `device_back` alerts, so they were silently dropped every time. `priority_offline` was unaffected (its event always included the field).

---

## [1.0.15] — 2026-06-05

### Fixed
- **Groups page — all devices shown without group assignment** — `GetGroupDevices` was missing `nmap_ports` from its SELECT, causing every row scan to fail silently. All group device lists appeared empty on `/groups` even though the map showed correct assignments.

---

## [1.0.14] — 2026-06-04

### Added
- **nmap port results in device detail** — open ports from the last nmap scan are now stored in the database (`nmap_ports` column) and displayed as a dedicated "Open Ports" section in the device info card. Ports are also included in the topology map node tooltip.

### Fixed
- **Topology map — group labels flicker on load** — hull labels are now hidden during the entire settling animation and only appear (without transition) once the simulation alpha drops below 0.015. Previously labels were repositioned every tick, causing visible jitter even with the D3 transition added in v1.0.13.

---

## [1.0.13] — 2026-06-04

### Added
- **Scan-in-progress indicator** — clicking the nmap button no longer redirects the page. The button disables, a "running…" badge appears in the accordion header, and the UI polls `/devices/{mac}/nmap/status` every 2 s. When the scan finishes the page reloads automatically to show the new activity-log entry. Opening the device page while a scan is already running detects the state immediately and shows the same indicator. Concurrent scan requests for the same device are rejected with HTTP 409.
- **nmap / mDNS data in map tooltip** — the hover tooltip on the topology map now shows vendor, OS info (from nmap) and mDNS service names when available.

### Fixed
- **Topology map — group label wobbles on load** — hull labels were repositioned every simulation tick, causing visible jitter during the brief settling animation. Label position updates now use a 180 ms eased D3 transition so movement is smooth.

---

## [1.0.12] — 2026-06-04

### Fixed
- **nmap scan fails with "could not locate nse_main.lua"** — in Alpine Linux `nmap` and `nmap-scripts` are separate packages; only `nmap` was installed in the Docker image, so the NSE script engine could not initialise. Added `nmap-scripts` to the `apk add` step in the Dockerfile.

---

## [1.0.11] — 2026-06-04

### Fixed
- **Docker — data directory permission error on fresh deploy** — the `silentmap` user (non-root) could not write to `/data` on first start because the directory was created as root during the image build. Added `mkdir -p /data && chown silentmap:silentmap /data` to the Dockerfile so named volumes are initialised with correct ownership automatically.

### Changed
- **`portainer-stack.yml`** — removed deprecated `version: '3.8'` field; switched from bind mount to a named volume (`silentmap-data`) so Docker handles data-directory permissions automatically on any host.

---

## [1.0.10] — 2026-06-04

### Fixed
- **Light theme — nav dropdown unreadable** — dropdown links inherited `nav-text` (near-white) from the nav bar override, making them invisible against the white `card-bg` dropdown background. Added an explicit `a.dd-link` rule that resets link colour to `text-primary` and hover to `accent`.
- **All themes — semantic badge colours not theme-aware** — status badges (`bg-green-100`, `bg-red-100`, `bg-orange-100`, `bg-yellow-100`, `bg-amber-50`) and their text classes used hardcoded Tailwind colours. On dark themes these light-coloured backgrounds were jarring against dark cards. All themes now define `badge-success/danger/orange/warning/amber` colour pairs; the CSS override layer maps the Tailwind classes to these variables.
- **Light theme — topology map auto-links invisible** — auto-detected link lines used `COLORS.border` (`--sm-card-border` = `#e5e7eb` in light theme) at reduced opacity, making them practically invisible on the white map background. Switched to `COLORS.text` so auto-links render as a subtle semi-transparent dark stroke in all themes.
- **All themes — `font-mono` colour override too broad** — the global `.font-mono { color: var(--sm-text-primary) !important }` rule overwrote explicit inline colours on nav stat numbers and table IPs. Removed; each element already carries its own explicit colour.
- **All themes — primary button hover shows wrong colour** — `hover:bg-blue-700` was mapped to `text-secondary` (grey). Changed to use the accent colour with `brightness(0.85)` so the hover darkens the button naturally in all themes.

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
