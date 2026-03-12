# CoreDNS Unifi Plugin

## Description

This is a plugin that queries a Unifi Controller in order to determine dhcp clients and network domains.

The Unifi system will do odd things with regards to the dnsmasq implementation, such as:

  - Aliases assigned in the interface will not be utilised as the domains assigned in `/etc/hosts` on gateway.
    - Hence misbehaving clients with bad/unconfigurable hostnames will not get assigned sane ones.
  - Setting a network's search domain to a public domain will cause odd behaviour because the dnsmasq instance
    will leak queries for internal clients to the upstream public DNS and subsequently fail.
    - You can 'fix' this by using a non public subdomain, but still.

It is similar to [USG Easy DNS](https://github.com/confirm/USG-Easy-DNS/) and was adapted originally from
the [CoreDNS Traefik](https://github.com/scottt732/coredns-traefik/) Plugin.

## How It Works

The plugin periodically fetches sites, clients, and networks from the UniFi controller. For each DHCP client, it builds a DNS A record by combining the client's name with its network's domain name (e.g. `desktop.home.lan`).

Client names are sanitized to be DNS-safe: lowercased, spaces and underscores replaced with hyphens, invalid characters stripped. The plugin prefers the UI-assigned **Name** (alias) over the DHCP-reported **Hostname**. Clients with no name or hostname, or on networks with no domain name, are skipped. Hostname collisions (two clients mapping to the same record) are detected and logged.

The plugin respects CoreDNS zone boundaries — it only answers queries that fall within the zones configured in the server block.

## Compilation

This package is compiled as part of CoreDNS, not standalone. Add the following to CoreDNS's [plugin.cfg](https://github.com/coredns/coredns/blob/master/plugin.cfg):

~~~
unifi:github.com/jhogendorn/coredns-unifi
~~~

Put this early in the plugin list, so that *unifi* is executed before any of the other plugins.

Then compile CoreDNS as [detailed on coredns.io](https://coredns.io/2017/07/25/compile-time-enabling-or-disabling-plugins/#build-with-compile-time-configuration-file):

``` sh
go generate
go build
```

Or you can instead use make:

``` sh
make
```

## Syntax

~~~ txt
unifi {
  controllerurl http://controller:port/
  username svc_coredns
  password mysecretpassword
  refreshInterval 30
  ttl 30
  sites default,branch-office
  fallthrough
}
~~~

- **controllerurl** — URL of the UniFi controller API.
- **username** — Username for controller authentication.
- **password** — Password for controller authentication.
- **refreshInterval** — How often (in seconds) to re-fetch clients from the controller. Default: `30`.
- **ttl** — TTL (in seconds) for DNS responses. Default: `30`.
- **sites** — Comma-separated list of UniFi site names to query. If omitted, all sites are queried.
- **fallthrough** — If present, pass unresolved queries to the next plugin.

## Metrics

If monitoring is enabled (via the *prometheus* directive) the following metric is exported:

* `coredns_unifi_request_count_total{server}` - query count to the *unifi* plugin.

The `server` label indicates which server handled the request, see the *metrics* plugin for details.

## Ready

This plugin reports readiness to the ready plugin. It will be immediately ready.

## Examples

Put it in a block that serves the internal search domain space. If you have other resolving plugins
for this domain it should fallthrough correctly.

~~~ corefile
mylocal.tld {
  unifi {
    controllerurl http://controller:port/
    username svc_coredns
    password mysecretpassword
    refreshInterval 30
    ttl 30
    fallthrough
  }
}
~~~

## Also See

See the [manual](https://coredns.io/manual).
