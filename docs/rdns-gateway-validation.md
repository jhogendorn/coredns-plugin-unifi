# Feature: rDNS Gateway Validation

## Status

Proposed — not yet implemented.

## Problem

The UniFi controller API provides raw client names (UI aliases) and DHCP-reported
hostnames, but does not expose the sanitized hostname that UniFi's gateway actually
registers in its `/etc/hosts` / dnsmasq. Our plugin applies its own sanitization
(lowercase, replace spaces/underscores with hyphens, strip invalid characters), but
this may not match UniFi's internal logic exactly for all edge cases.

## Proposed Solution

Add an optional `gatewayip` directive that enables reverse DNS validation against
the UniFi gateway's DNS server.

### How it would work

1. User configures `gatewayip 192.168.1.1` in their Corefile block.
2. During each refresh cycle, after building mappings from the API, the plugin
   performs PTR lookups for each client IP against the gateway's DNS (port 53).
3. The PTR result (the FQDN the gateway has registered) is compared against our
   computed record name.
4. Mismatches are logged as warnings so the user can see where our sanitization
   diverges from UniFi's.

### Caching

PTR results would be cached by IP with a configurable TTL to avoid flooding the
gateway with DNS queries on every refresh cycle. Only expired cache entries would
be re-queried.

### Configuration

```corefile
unifi {
  ...
  gatewayip 192.168.1.1
}
```

The feature is opt-in. Without `gatewayip`, no rDNS queries are made.

### Safety

- On startup, compare the configured gateway IP against the host's own IPs.
  If they match, refuse to enable rDNS checking (would create a self-referential
  loop if this CoreDNS instance is the resolver for the gateway's network).
- This plugin should not run on the gateway itself when rDNS checking is enabled.

### Open Questions

- Should mismatches only be logged, or should the gateway's answer override ours?
  Using it as a source of truth undermines the plugin's purpose (improving on
  UniFi's DNS behavior), but ignoring it means we can't self-correct.
- Should we support bulk PTR queries or is per-IP sufficient?
- What's a reasonable default cache TTL? Probably match the refresh interval.
- Should we query forward (A record) as well as reverse (PTR)? The gateway might
  have A records but not PTR records, or vice versa.
