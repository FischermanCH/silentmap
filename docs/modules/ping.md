# Modul: Ping-Collector

**Typ:** Aktiv · **Status:** Geplant (Milestone 0.4) · **Pflicht:** Nein

## Zweck

Aktiver Liveness-Check für Geräte — primär für Priority-Devices die wenig eigenen Traffic erzeugen (z.B. Smart-Home-Geräte, Server im Standby).

**Achtung:** Dieses Modul ist aktiv — es sendet ICMP-Pakete. Muss explizit aktiviert werden.

## Wann sinnvoll

- Priority-Devices die selten ARP senden
- Geräte mit statischer IP (kein DHCP, seltenes ARP)
- Präzisere Offline-Erkennung als ARP-Timeout

## Konfiguration

```yaml
collectors:
  ping:
    enabled: false        # Explizit opt-in
    targets: "priority"   # "priority" | "all" | ["mac1", "mac2"]
    interval: 60s
    timeout: 3s
    retries: 2
```

## Events

```json
{
  "type": "device.seen",
  "mac": "aa:bb:cc:dd:ee:ff",
  "ip": "192.168.1.10",
  "source": "ping",
  "meta": {
    "rtt_ms": 1.2,
    "ttl": 64
  }
}
```

Bei ausbleibendem Ping:
```json
{
  "type": "device.lost",
  "mac": "aa:bb:cc:dd:ee:ff",
  "source": "ping",
  "meta": { "reason": "no_reply", "retries": 2 }
}
```

## Limitierungen

- Manche Geräte/Firewalls blockieren ICMP
- Erfordert `CAP_NET_RAW` (bereits für ARP vorhanden)
- Erzeugt messbaren (wenn auch minimalen) Netzwerk-Traffic
