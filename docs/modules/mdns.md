# Modul: mDNS-Collector

**Typ:** Passiv · **Status:** Implementiert · **Pflicht:** Nein

## Zweck

Lauscht auf mDNS (Multicast DNS) und DNS-SD (Service Discovery) Traffic. Liefert Hostnamen und angekündigte Services ohne jede aktive Anfrage.

## Was erkannt wird

- Hostname (z.B. `Johns-MacBook.local`)
- Angekündigte Services (z.B. `_airplay._tcp`, `_smb._tcp`, `_http._tcp`)
- TXT-Records (Service-Details, z.B. Modell, Versionsnummern)

## Erkennbare Gerätekategorien (via Service-Patterns)

| Services | Wahrscheinliches Gerät |
|---|---|
| `_airplay._tcp` + `_raop._tcp` | Apple TV / AirPlay-Gerät |
| `_smb._tcp` + `_afpovertcp._tcp` | NAS / Mac |
| `_ipp._tcp` + `_printer._tcp` | Drucker |
| `_googlecast._tcp` | Chromecast / Google-Gerät |
| `_homekit._tcp` | HomeKit-Gerät |
| `_ssh._tcp` | Linux/Server |

## Konfiguration

```yaml
collectors:
  mdns:
    enabled: true
```

## Events

```json
{
  "type": "device.seen",
  "mac": "",
  "ip": "192.168.1.15",
  "source": "mdns",
  "meta": {
    "hostname": "Johns-MacBook.local",
    "services": ["_smb._tcp", "_ssh._tcp", "_afpovertcp._tcp"],
    "txt_records": {"model": "MacBookPro18,1"}
  }
}
```

**Hinweis:** mDNS enthält keine MAC-Adresse. Die Registry matcht via IP auf bekannte Geräte.

## Limitierungen

- Nur Geräte die aktiv mDNS/Bonjour nutzen (Apple-Geräte, moderne Linux-Systeme, viele IoT-Geräte)
- Windows-Geräte oft nur mit aktiviertem Bonjour (iTunes) oder WSD
- Kein MAC — Matching via IP kann bei DHCP-Wechseln fehlschlagen
