# Feature: Site Filtering

## Status

Proposed — not yet implemented.

## Problem

The plugin currently fetches clients from all sites on the UniFi controller. In
multi-site deployments (e.g. an MSP managing multiple customer networks, or a
large organisation with separate sites), this means:

- All clients from all sites end up in the same DNS namespace
- Hostname collisions are more likely across unrelated sites
- The plugin may be resolving names for networks it shouldn't be serving

## Proposed Solution

Add an optional `sites` directive to the Corefile configuration that limits which
UniFi sites the plugin queries.

### Configuration

```corefile
unifi {
  ...
  sites default,remote-office
}
```

Without the directive, the current behaviour is preserved (all sites).

### Implementation Notes

- The `sites` value would be matched against `Site.Name` or `Site.SiteName` from
  the unpoller API response.
- Filter should be applied immediately after `GetSites()`, before fetching
  clients and networks — this avoids unnecessary API calls for excluded sites.
- Consider supporting glob patterns (e.g. `customer-*`) for flexibility.
- The existing `@TODO` comment in `unifi.go:refresh()` marks where this filter
  would be inserted.

### Open Questions

- Match on `Site.Name` (internal identifier like `default`) or `Site.SiteName`
  (human-readable like "Default Site"), or both?
- Should it be an allowlist (only these sites) or also support a denylist
  (all sites except these)?
- Should different sites be able to have different TTLs or domain overrides?
