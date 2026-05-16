# Modul: DHCP-Collector

**Typ:** Passiv · **Status:** Geplant (Milestone 0.2) · **Pflicht:** Nein

## Zweck

Lauscht auf DHCP-Requests im Netzwerk. Liefert die direkteste Verknüpfung von MAC-Adresse, IP und Hostname — ohne selbst DHCP-Server zu sein.

## Was erkannt wird

| DHCP-Typ | Wann | Was wir lernen |
|---|---|---|
| DHCP Discover | Gerät sucht IP | MAC + Hostname (Option 12) |
| DHCP Request | Gerät bestätigt IP | MAC + gewünschte IP + Hostname |
| DHCP Inform | Gerät hat statische IP | MAC + IP + Hostname |

## Konfiguration

```yaml
collectors:
  dhcp:
    enabled: true
```

## Events

```json
{
  "type": "device.seen",
  "mac": "aa:bb:cc:dd:ee:ff",
  "ip": "192.168.1.44",
  "source": "dhcp",
  "meta": {
    "dhcp_type": "request",
    "hostname": "Johns-iPhone",
    "requested_ip": "192.168.1.44",
    "vendor_class": "dhcpcd-9.4.1"
  }
}
```

## Limitierungen

- Geräte mit statischer IP senden kein DHCP → nur DHCP Inform (wenn überhaupt)
- Hostname-Feld (Option 12) ist optional — nicht alle Geräte senden es
- Erkennt nur neue DHCP-Vorgänge — Geräte mit langer Lease-Zeit tauchen selten auf
