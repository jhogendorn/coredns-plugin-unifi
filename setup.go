package unifi

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() { plugin.Register("unifi", setup) }

func setup(c *caddy.Controller) error {
	c.Next() // Consume "unifi" token.
	origins := plugin.OriginsFromArgsOrServerBlock(c.RemainingArgs(), c.ServerBlockKeys)

	cfg := &UnifiConfig{
		ttl:             defaultTTL,
		refreshInterval: defaultRefreshInterval,
	}

	u := &Unifi{
		Config:   cfg,
		Origins:  origins,
		mappings: make(UnifiConfigEntryMap),
		done:     make(chan struct{}),
	}

	for c.NextBlock() {
		switch c.Val() {
		case "controllerurl":
			if !c.NextArg() {
				return c.ArgErr()
			}
			cfg.controllerUrl = c.Val()
		case "username":
			if !c.NextArg() {
				return c.ArgErr()
			}
			cfg.username = c.Val()
		case "password":
			if !c.NextArg() {
				return c.ArgErr()
			}
			cfg.password = c.Val()
		case "ttl":
			if !c.NextArg() {
				return c.ArgErr()
			}
			val, err := strconv.ParseUint(c.Val(), 10, 32)
			if err != nil {
				return fmt.Errorf("invalid ttl value: %s", c.Val())
			}
			cfg.ttl = uint32(val)
		case "refreshinterval":
			if !c.NextArg() {
				return c.ArgErr()
			}
			val, err := strconv.ParseUint(c.Val(), 10, 32)
			if err != nil {
				return fmt.Errorf("invalid refreshinterval value: %s", c.Val())
			}
			cfg.refreshInterval = uint32(val)
		case "sites":
			if !c.NextArg() {
				return c.ArgErr()
			}
			parts := strings.Split(c.Val(), ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			cfg.sites = parts
		case "fallthrough":
			u.fall.SetZonesFromArgs(c.RemainingArgs())
		default:
			return c.Errf("unknown property: '%s'", c.Val())
		}
	}

	unifiClient, err := NewUnifiClient(cfg)
	if err != nil {
		return err
	}

	u.Client = unifiClient

	log.Infof("Unifi Controller: %s", cfg.controllerUrl)

	go u.start()

	c.OnShutdown(func() error {
		close(u.done)
		return nil
	})

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		u.Next = next
		return u
	})

	return nil
}
