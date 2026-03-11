# Feature: Zone Filtering in ServeDNS

## Status

Proposed — not yet implemented.

## Problem

The plugin currently answers queries for any domain that matches an entry in its
mappings, regardless of what DNS zone the Corefile block is configured to serve.
For example, if a client has the record `desktop.home.lan` and the plugin is
configured inside a `corp.local` zone block, it will still attempt to match
queries — the zone context is ignored.

This means:

- The plugin doesn't respect CoreDNS zone boundaries
- It could return answers for domains outside its configured authority
- In multi-zone setups, the same plugin instance might answer queries it
  shouldn't

## Proposed Solution

Filter queries in `ServeDNS()` to only answer for hostnames that fall within the
configured zone(s).

### How CoreDNS Zones Work

In a Corefile, zones are defined by the server block:

```corefile
home.lan {
  unifi { ... }
}
```

The plugin receives the zone via `plugin.Zones()` or can extract it from the
server block configuration during setup. The `request.Request` object provides
`Zone` to identify which zone matched the query.

### Implementation Notes

- During `ServeDNS()`, check that the queried name's suffix matches the
  configured zone before looking up the mapping.
- This is a relatively small change — add a zone check before the mapping
  lookup in the question loop.
- Could also filter during `refresh()` to avoid building mappings for domains
  outside the zone, but this would prevent the plugin from working across
  multiple zones in a single block.

### Example Behaviour

```corefile
home.lan {
  unifi { ... }
}
```

- Query for `desktop.home.lan` → check mappings, respond if found
- Query for `desktop.corp.local` → skip, fall through to next plugin

### Open Questions

- Should filtering happen at query time (ServeDNS) or at refresh time, or both?
  Query-time is simpler and allows one plugin instance to serve multiple zones
  if needed.
- How does this interact with `fallthrough`? If the query is outside the zone,
  should it fall through or return NXDOMAIN?
- Should the plugin support being configured in a wildcard zone block (`.`)?
  If so, zone filtering would need to be optional.
