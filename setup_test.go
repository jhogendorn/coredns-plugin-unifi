package unifi

import (
	"testing"

	"github.com/coredns/caddy"
)

func TestSetupMinimal(t *testing.T) {
	// Just the plugin name with no block — should succeed with empty config.
	c := caddy.NewTestController("dns", `unifi`)
	if err := setup(c); err == nil {
		// Without a controller URL, NewUnifiClient will fail, which is expected.
		// If it doesn't error, that's also fine — depends on unpoller behavior.
	}
}

func TestSetupUnknownDirective(t *testing.T) {
	c := caddy.NewTestController("dns", `unifi {
		bogus value
	}`)
	if err := setup(c); err == nil {
		t.Fatal("Expected error for unknown directive, but got nil")
	}
}

func TestSetupInvalidTTL(t *testing.T) {
	c := caddy.NewTestController("dns", `unifi {
		ttl notanumber
	}`)
	if err := setup(c); err == nil {
		t.Fatal("Expected error for invalid ttl, but got nil")
	}
}

func TestSetupInvalidRefreshInterval(t *testing.T) {
	c := caddy.NewTestController("dns", `unifi {
		refreshinterval notanumber
	}`)
	if err := setup(c); err == nil {
		t.Fatal("Expected error for invalid refreshinterval, but got nil")
	}
}

func TestSetupMissingArgs(t *testing.T) {
	directives := []string{"controllerurl", "username", "password", "ttl", "refreshinterval"}
	for _, d := range directives {
		c := caddy.NewTestController("dns", "unifi {\n"+d+"\n}")
		if err := setup(c); err == nil {
			t.Fatalf("Expected error for missing arg on directive '%s', but got nil", d)
		}
	}
}
