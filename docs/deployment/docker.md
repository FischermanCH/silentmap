# Deployment: Docker

## Schnellstart (minimales Setup)

```bash
docker run -d \
  --name silentmap \
  --net=host \
  --cap-add=NET_RAW \
  --restart=unless-stopped \
  -v silentmap-data:/data \
  fischermanch/silentmap:latest
```

Web UI: `http://localhost:8080`

`--net=host` ist erforderlich — ohne Host-Netzwerk sind ARP/mDNS/DHCP nicht sichtbar.  
`--cap-add=NET_RAW` erlaubt Packet-Capturing ohne vollständige Root-Rechte.

## docker-compose / Portainer (empfohlen)

Siehe `portainer-stack.yml` im Projektroot.

```bash
# Starten
docker compose up -d

# Logs
docker compose logs -f silentmap

# Stoppen
docker compose down
```

## Daten & Backup

Alle Daten in einem Volume oder Bind Mount:
```
/data/
├── silentmap.db      # Gerätedaten, Events, Alerts
├── silentmap.yaml    # Konfiguration
├── settings.json     # UI-Einstellungen (Kanäle etc.)
├── secret.key        # Verschlüsselungsschlüssel (auto-generiert)
└── auth.hash         # bcrypt Passwort-Hash (nach /setup)
```

**Hinweis:** SQLite nicht auf SMB/NFS-Share betreiben (Locking-Probleme).

Backup:
```bash
docker cp silentmap:/data/silentmap.db ./backup-$(date +%Y%m%d).db
```

## Updates

```bash
docker pull fischermanch/silentmap:latest
# dann Container neu starten (Portainer: Stack Update / docker compose up -d)
```

Daten bleiben im Volume erhalten.

## Ressourcen

| Ressource | Minimum | Empfohlen |
|---|---|---|
| CPU | 1 Core | 2 Cores |
| RAM | 128MB | 256MB (512MB mit Ollama) |
| Disk | 500MB | 2GB (für 1+ Jahr History) |
| Netzwerk | Host-Mode | Host-Mode (Pflicht) |
