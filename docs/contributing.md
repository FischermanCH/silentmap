# Contributing

## Entwicklungsumgebung

### Voraussetzungen

```bash
# Go >= 1.22
go version

# libpcap (für Packet-Sniffer)
# Debian/Ubuntu:
sudo apt install libpcap-dev

# macOS:
brew install libpcap

# Docker (optional, für Container-Build)
docker --version
```

### Repository klonen und starten

```bash
git clone https://github.com/[user]/silentmap
cd silentmap

go mod download
go run ./cmd/silentmap --debug --interface lo
```

## Projektstruktur

Kurze Orientierung was wo liegt:

```
cmd/silentmap/      → main.go, CLI-Flags, Bootstrap
internal/bus/       → Event Bus — hier nie Geschäftslogik
internal/registry/  → Device Registry, SQLite, OUI-DB, Groups
internal/collectors/→ Ein Ordner pro Collector-Modul
internal/alerting/  → Rules, Dedup, Channels (discord, ntfy)
internal/scanner/   → nmap-Integration
internal/web/       → HTTP-Handler, Templates, API
internal/i18n/      → DE/EN Übersetzungen
configs/            → Beispiel-Konfigurationen
docs/               → Diese Dokumentation
scripts/            → Hilfsskripte (OUI-Update, ...)
```

## Neues Collector-Modul hinzufügen

1. Ordner anlegen: `internal/collectors/meinmodul/`
2. `Collector`-Interface implementieren:

```go
type MyCollector struct { ... }

func (c *MyCollector) Name() string { return "meinmodul" }

func (c *MyCollector) Start(ctx context.Context, bus bus.EventBus) error {
    // Hier lauschen und Events publishen
    bus.Publish(bus.Event{
        Type:   "device.seen",
        MAC:    "aa:bb:cc:dd:ee:ff",
        IP:     "192.168.1.5",
        Source: "meinmodul",
        Meta:   map[string]any{"custom_field": "wert"},
    })
    return nil
}

func (c *MyCollector) Stop() error { return nil }
```

3. In `cmd/silentmap/main.go` registrieren
4. In `configs/silentmap.example.yaml` dokumentieren
5. Modul-Doku in `docs/modules/meinmodul.md` anlegen

## Neuen Alert-Kanal hinzufügen

1. `internal/alerting/channels/meinkanal.go` anlegen
2. `Channel`-Interface implementieren:

```go
type Channel interface {
    Name()   string
    Send(ctx context.Context, alert Alert) error
    Enabled() bool
}
```

3. In `internal/alerting/engine` registrieren
4. In `docs/alerting.md` dokumentieren

## Dokumentations-Pflicht

**Jede Code-Änderung braucht:**
- Aktualisierung der relevanten `docs/*.md`-Datei
- Eintrag in `CHANGELOG.md` unter `[Unreleased]`
- Config-Änderungen → `docs/configuration.md` aktualisieren

**Neue Features zusätzlich:**
- Eintrag unter `[Unreleased]` in `CHANGELOG.md`

## Commit-Konventionen

```
feat: neuer ARP-Collector
fix: DHCP-Parser crasht bei leerem Hostname
docs: Alerting-Dokumentation vervollständigt
chore: OUI-Datenbank aktualisiert
refactor: Event Bus auf Channels umgestellt
test: Unit-Tests für Registry-Timeout-Logik
```

## Pull Request Checkliste

- [ ] Code kompiliert (`go build ./...`)
- [ ] Tests laufen durch (`go test ./...`)
- [ ] Relevante Doku aktualisiert
- [ ] CHANGELOG.md ergänzt
- [ ] Keine neuen externen Abhängigkeiten ohne Begründung
