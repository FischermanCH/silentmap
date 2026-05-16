# Deployment: Docker

## Schnellstart (minimales Setup)

```bash
docker run -d \
  --name silentmap \
  --net=host \
  --cap-add=NET_RAW \
  --restart=unless-stopped \
  -v silentmap-data:/data \
  silentmap/silentmap:latest
```

Web UI: `http://localhost:8080`

`--net=host` ist erforderlich — ohne Host-Netzwerk sind ARP/mDNS/DHCP nicht sichtbar.  
`--cap-add=NET_RAW` erlaubt Packet-Capturing ohne vollständige Root-Rechte.

## docker-compose (empfohlen)

Siehe `docker-compose.yml` im Projektroot. Enthält optionalen Ollama-Service für KI-Korrelation.

```bash
# Starten
docker compose up -d

# Logs
docker compose logs -f silentmap

# Stoppen
docker compose down
```

## Mit Ollama (KI-Korrelation)

```bash
docker compose --profile ai up -d
```

Lädt Phi-3 mini automatisch beim ersten Start (~2GB).

## Daten & Backup

Alle Daten in einem Volume:
```
/data/
├── silentmap.db      # Gerätedaten, Events, Alerts
├── silentmap.yaml    # Konfiguration
└── models/           # KI-Modelle
```

Backup:
```bash
docker cp silentmap:/data/silentmap.db ./backup-$(date +%Y%m%d).db
```

## Updates

```bash
docker compose pull
docker compose up -d
```

Daten bleiben im Volume erhalten.

## Ressourcen

| Ressource | Minimum | Empfohlen |
|---|---|---|
| CPU | 1 Core | 2 Cores |
| RAM | 128MB | 256MB (512MB mit Ollama) |
| Disk | 500MB | 2GB (für 1+ Jahr History) |
| Netzwerk | Host-Mode | Host-Mode (Pflicht) |
