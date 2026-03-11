# Feature: Device Interrogation

## Status

Proposed — not yet implemented.

## Problem

The plugin currently only resolves DHCP clients fetched via `GetClients()`. UniFi
networks also contain infrastructure devices (access points, switches, gateways,
cameras) that have hostnames and IP addresses but are not DHCP clients. These
devices are invisible to the plugin.

## Proposed Solution

Extend the refresh cycle to also query UniFi devices and include them in the DNS
mappings.

### Potential API Sources

The unpoller library provides several device-fetching methods:

- `GetUSW()` — UniFi switches
- `GetUAP()` — UniFi access points
- `GetUDM()` — UniFi Dream Machines / gateways
- `GetUSG()` — UniFi Security Gateways

Each of these returns device structs that include `Name`, `IP`, and `SiteID`
fields.

### Configuration

```corefile
unifi {
  ...
  includedevices true
}
```

Opt-in to avoid changing existing behaviour. Could also support filtering by
device type:

```corefile
unifi {
  ...
  includedevices uap,usw
}
```

### Implementation Notes

- Device names from UniFi are typically human-assigned in the UI (e.g.
  "Office AP - 2nd Floor"), so they would need the same `sanitizeHostname()`
  treatment as client names.
- Devices would need to be associated with a network domain. Unlike clients,
  devices may not have a `NetworkID` — they may need to be mapped via their
  management IP subnet or a default domain.
- Collision detection between devices and clients sharing the same sanitized
  hostname would need to be handled (device names like "switch" are common).

### Open Questions

- How to determine the domain for a device? Devices don't have a `NetworkID`
  like clients do. Options: use a configurable default domain, match by IP
  subnet against known networks, or use the site's default network domain.
- Should device DNS entries have a different TTL than client entries? Devices
  are more stable than DHCP clients.
- The `UnifiAPI` interface would need to be extended with device-fetching
  methods, which affects the mock in tests.
