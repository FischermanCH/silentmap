# Modul: ARP-Collector

**Typ:** Passiv · **Status:** Geplant (Milestone 0.1) · **Pflicht:** Ja

## Zweck

Erkennt Geräte im lokalen Netzwerk durch Mithören von ARP-Traffic (Address Resolution Protocol). ARP wird von jedem Gerät automatisch gesendet — kein aktives Scannen nötig.

## Was erkannt wird

| ARP-Typ | Wann | Was wir lernen |
|---|---|---|
| ARP Request | Gerät sucht MAC zu einer IP | Sender-MAC + Sender-IP |
| ARP Reply | Gerät antwortet auf Request | Sender-MAC + Sender-IP |
| Gratuitous ARP | Gerät kündigt sich an (Boot, IP-Wechsel) | MAC + neue IP |

## Technische Umsetzung

- Bibliothek: `gopacket` mit libpcap-Backend
- Filter: `arp` (BPF-Filter, kein userspace-Overhead)
- Promiscuous Mode: nicht nötig (ARP ist Broadcast)
- Benötigte Rechte: `CAP_NET_RAW`

## Events

Publiziert ausschließlich `device.seen` Events:

```json
{
  "type": "device.seen",
  "mac": "b8:27:eb:12:34:56",
  "ip": "192.168.1.10",
  "source": "arp",
  "meta": {
    "arp_type": "request",
    "target_ip": "192.168.1.1"
  }
}
```

## Konfiguration

```yaml
collectors:
  arp:
    enabled: true
    offline_timeout: 15m
```

`offline_timeout`: Zeit ohne ARP-Signal bevor die Registry das Gerät als offline markiert. Kurze Werte → empfindlicher, mehr Alerts. Längere Werte → robuster gegen kurze Verbindungsabbrüche.

## Limitierungen

- Erkennt nur Geräte im **selben Layer-2-Segment** (kein Routing über Subnetz-Grenzen)
- Geräte die sehr selten ARP senden (z.B. statische IPs, lange ARP-Cache-Zeiten) können spät erkannt werden
- IPv6-Geräte nutzen NDP statt ARP → separates Modul geplant

## Zusammenspiel mit anderen Modulen

- **mDNS-Collector:** ergänzt Hostname zum Gerät
- **DHCP-Collector:** ergänzt vergebene IP und Hostname
- **Ping-Collector:** übernimmt Liveness-Check wenn ARP-Traffic ausbleibt
