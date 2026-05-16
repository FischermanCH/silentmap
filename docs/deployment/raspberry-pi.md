# Deployment: Raspberry Pi

Empfohlene Hardware für den Dauerbetrieb als dedizierten Netzwerk-Monitor.

## Empfohlene Hardware

| Komponente | Empfehlung | Minimum |
|---|---|---|
| Modell | Raspberry Pi 4 (2GB+) | Raspberry Pi 3B+ |
| SD-Karte | 32GB Class A1/A2 | 16GB |
| Kühlung | Passiv-Kühler | Nichts (aber warm) |
| Stromversorgung | Offizielles Pi-Netzteil | 3A USB-C |

## Installation (Docker, empfohlen)

```bash
# Docker installieren
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER

# Projekt holen
git clone https://github.com/[user]/silentmap
cd silentmap

# Starten
docker compose up -d
```

## Installation (Binary, ohne Docker)

```bash
# ARM64-Binary herunterladen
curl -L https://github.com/[user]/silentmap/releases/latest/download/silentmap-linux-arm64 \
  -o /usr/local/bin/silentmap
chmod +x /usr/local/bin/silentmap

# libpcap installieren
sudo apt install -y libpcap0.8

# Als systemd-Service einrichten
sudo cp scripts/silentmap.service /etc/systemd/system/
sudo systemctl enable --now silentmap
```

## SD-Karten-Schonung

SQLite schreibt häufig. Für längere SD-Karten-Lebensdauer:

```yaml
# silentmap.yaml
storage:
  wal_mode: true           # WAL statt Journal (weniger Writes)
  checkpoint_interval: 1h  # Seltener checkpointing
  vacuum_interval: 7d      # Wöchentliches VACUUM
```

Oder: Externes USB-SSD für `/data` mounten.

## Performance

- **CPU:** silentmap nutzt <5% CPU auf RPi 4 bei normalem Heimnetz-Traffic
- **RAM:** ~80MB Basis, ~200MB mit KI-Fingerprinting
- **Ollama auf RPi:** Möglich ab RPi 4 (4GB) — langsam (~10s Inference), aber funktional
