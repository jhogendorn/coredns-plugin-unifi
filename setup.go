package unifi

import (
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

// init registers this plugin.
func init() { plugin.Register("unifi", setup) }

// setup is the function that gets called when the config parser see the token "example". Setup is responsible
// for parsing any extra options the example plugin may have. The first token this function sees is "example".
func setup(c *caddy.Controller) error {
	c.Next() // Ignore "example" and give us the next token.

	cfg := &UnifiConfig{
		controllerUrl:			nil,
		username:						nil,
		password:						nil,
	}

	unifi := &Unifi{
		Config: cfg
	}

	for c.Next() {
		for c.NextBlock() {
			var value = c.Val()
			if !c.NextArg() {
				return unifi, c.ArgErr()
			}
			switch value {
				case "controllerurl":
					cfg.controllerUrl = c.Val()
				case "username":
					cfg.username = c.Val()
				case "password":
					cfg.password = c.Val()
				case "ttl":
					cfg.ttl = c.Val()
				case "refreshinterval":
					cfg.refreshInterval = c.Val()
				default:
					return unifi, c.Err(":unknown property: '%s'", c.Val())
			}
		}
	}

	unifiClient, err := NewUnifiClient(cfg)
	if err != nil {
		return nil, err
	}

	unifi.client = unifiClient
	unifi.mappings = &make(UnifiConfigEntryMap)

	log.Infof("Unifi Controller: %s", cfg.controllerUrl)

	go unifi.start()

	// Add the Plugin to CoreDNS, so Servers can use it in their plugin chain.
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return Unifi{Next: next}
	})

	// All OK, return a nil error.
	return nil
}
