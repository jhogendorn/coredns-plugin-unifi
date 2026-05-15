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

The plugin periodically fetches sites, clients, and networks from the UniFi controller. For each DHCP client, it builds a DNS A record by combining the client's name with its network's domain name (e.g. `desktop.home.lan`), and a matching PTR record in the corresponding `in-addr.arpa` zone (IPv4 only).

Client names are sanitized to be DNS-safe: lowercased, spaces and underscores replaced with hyphens, invalid characters stripped. The plugin prefers the UI-assigned **Name** (alias) over the DHCP-reported **Hostname**. Clients with no name or hostname, or on networks with no domain name, are skipped. Hostname collisions (two clients mapping to the same record) are detected and logged. Reverse-mapping collisions (two clients on the same IP) are likewise logged.

The plugin respects CoreDNS zone boundaries — it only answers queries that fall within the zones configured in the server block. To serve PTR records you must include the relevant reverse zone (e.g. `1.168.192.in-addr.arpa.`) in the server block alongside the forward zone.

## Compilation

This plugin is compiled into CoreDNS — it is not a standalone binary.

### Step-by-step build

```sh
# 1. Clone CoreDNS
git clone --depth 1 --branch v1.11.3 https://github.com/coredns/coredns.git
cd coredns

# 2. Add the plugin — insert above the forward: line
sed -i '/^forward:forward/a unifi:github.com/jhogendorn/coredns-plugin-unifi' plugin.cfg

# Or edit plugin.cfg manually; the entry should read:
#   unifi:github.com/jhogendorn/coredns-plugin-unifi
# placed immediately above:
#   forward:forward

# 3. Build
go generate && go build -o coredns .

# 4. Verify the plugin is included
./coredns -plugins | grep unifi
```

Requires CoreDNS >= v1.11 and Go >= 1.24.

### Dockerfile example

The following multi-stage Dockerfile builds a CoreDNS binary with this plugin included. It is optimised for layer caching — the CoreDNS clone and dependency steps are cached across plugin source changes.

```dockerfile
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git

# Clone CoreDNS and inject plugin — these layers are stable
RUN git clone --depth 1 --branch v1.11.3 https://github.com/coredns/coredns.git /coredns
WORKDIR /coredns
RUN sed -i '/^forward:forward/a unifi:github.com/jhogendorn/coredns-plugin-unifi' plugin.cfg

# Copy plugin module files first for dependency caching
COPY go.mod go.sum /plugin/
RUN go generate && \
    go get github.com/jhogendorn/coredns-plugin-unifi && \
    go mod download

# Now copy plugin source and build
COPY *.go /plugin/
RUN go mod tidy && go build -o coredns .

FROM alpine:3.20
COPY --from=builder /coredns/coredns /usr/local/bin/coredns
EXPOSE 53 53/udp
CMD ["coredns", "-conf", "/etc/coredns/Corefile"]
```

> **Note:** This Dockerfile pulls the plugin from the module proxy (`github.com/jhogendorn/coredns-plugin-unifi`). For local development builds, see the integration harness in the `integration/` directory.

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

## UniFi Setup

Create a read-only local admin in UniFi Network (Settings → Admins & Users) and use those credentials for `username` and `password`.

## Examples

### Basic single-zone block

Serve a single internal domain. Unresolved queries fall through to the next plugin.

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

### Multi-site deployment

Use the `sites` directive to limit which UniFi sites are queried. Useful when you have multiple physical sites but only want to expose clients from specific ones.

~~~ corefile
internal.corp {
  unifi {
    controllerurl http://unifi.internal:8443/
    username svc_coredns
    password mysecretpassword
    refreshInterval 60
    ttl 60
    sites default,branch-office
    fallthrough
  }
}
~~~

### Fallthrough chain: unifi → hosts → forward

Unifi handles dynamic DHCP clients. A `hosts` block allows static overrides (e.g. the router itself). Everything else is forwarded to upstream DNS.

~~~ corefile
home.lan {
  unifi {
    controllerurl http://unifi.home:8443/
    username svc_coredns
    password mysecretpassword
    ttl 30
    fallthrough
  }
  hosts {
    192.168.1.1 router.home.lan
    fallthrough
  }
  forward . 1.1.1.1 8.8.8.8
}
~~~

### Forward + reverse (PTR) on a /24

Include the matching `in-addr.arpa.` zone in the server block to enable PTR responses for the same DHCP clients.

~~~ corefile
home.lan 1.168.192.in-addr.arpa. {
  unifi {
    controllerurl http://unifi.home:8443/
    username svc_coredns
    password mysecretpassword
    ttl 30
    fallthrough
  }
}
~~~

## Also See

See the [manual](https://coredns.io/manual).
