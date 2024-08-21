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

## Compilation

This package will always be compiled as part of CoreDNS and not in a standalone way. It will require you to use `go get` or as a dependency on [plugin.cfg](https://github.com/coredns/coredns/blob/master/plugin.cfg).

The [manual](https://coredns.io/manual/toc/#what-is-coredns) will have more information about how to configure and extend the server with external plugins.

A simple way to consume this plugin, is by adding the following on [plugin.cfg](https://github.com/coredns/coredns/blob/master/plugin.cfg), and recompile it as [detailed on coredns.io](https://coredns.io/2017/07/25/compile-time-enabling-or-disabling-plugins/#build-with-compile-time-configuration-file).

~~~
example:github.com/coredns/example
unifi:github.com/jhogendorn/coredns-unifi
~~~

Put this early in the plugin list, so that *unifi* is executed before any of the other plugins.

After this you can compile coredns by:

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
}
~~~

## Metrics

If monitoring is enabled (via the *prometheus* directive) the following metric is exported:

* `coredns_example_request_count_total{server}` - query count to the *example* plugin.

The `server` label indicated which server handled the request, see the *metrics* plugin for details.

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
