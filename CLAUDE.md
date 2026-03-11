# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

CoreDNS plugin (`unifi`) that queries a UniFi Controller for DHCP clients and resolves their hostnames using network domain names. Built on the [unpoller/unifi](https://github.com/unpoller/unifi) library. This is **not a standalone binary** — it compiles as part of CoreDNS via `plugin.cfg`.

## Build & Test

This plugin is compiled into CoreDNS, not independently. To build:

1. Add `unifi:github.com/jhogendorn/coredns-unifi` to CoreDNS's `plugin.cfg`
2. Run `go generate && go build` in the CoreDNS source directory

For local development (syntax checking, tests):
```sh
go vet ./...
go test ./...
go test -run TestSetup   # single test
```

Integration tests require Docker:
```sh
go test -tags integration -v -timeout 180s .
```

Linting:
```sh
golangci-lint run ./...
```

Go version: 1.24.9 (see `mise.toml`)

## Architecture

The plugin follows the standard CoreDNS plugin pattern:

- **setup.go** — Plugin registration (`init()`) and Corefile config parsing. Parses block directives (`controllerurl`, `username`, `password`, `ttl`, `refreshinterval`) and starts the background refresh goroutine.
- **unifi.go** — Core plugin logic. `ServeDNS()` handles A record lookups against an in-memory map (`UnifiConfigEntryMap`). `refresh()` periodically fetches sites/clients/networks from the UniFi controller and rebuilds the hostname→IP mapping. Client names are sanitized via `sanitizeHostname()` (lowercase, replace spaces/underscores with hyphens, strip invalid chars). Prefers UI alias (`Name`) over DHCP-reported `Hostname`. Protected by `sync.RWMutex`. Detects hostname collisions and logs skipped clients.
- **client.go** — Wraps `unpoller/unifi` to create the controller API client via `UnifiAPI` interface. `NewUnifiClient()` initializes the connection.
- **ready.go** — Implements CoreDNS readiness interface (currently always returns true).
- **metrics.go** — Prometheus counter `coredns_unifi_request_count_total` for query tracking.
- **mock_test.go** — `mockUnifiAPI` implementing `UnifiAPI` interface for unit tests.
- **integration_test.go** — Testcontainers-go integration tests (build tag: `integration`).
- **integration/** — Docker compose, Dockerfiles, mock controller, Corefile for integration tests.

## Corefile Configuration

```
unifi {
  controllerurl http://controller:port/
  username svc_coredns
  password mysecretpassword
  refreshInterval 30
  ttl 30
  fallthrough
}
```
