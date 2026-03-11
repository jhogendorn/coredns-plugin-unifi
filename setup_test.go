package unifi

import (
	"strings"
	"testing"

	"github.com/coredns/caddy"
)

func TestSetupMinimal(t *testing.T) {
	// Just the plugin name with no block — setup may fail because
	// NewUnifiClient needs a controller URL. Either outcome is valid.
	c := caddy.NewTestController("dns", `unifi`)
	_ = setup(c)
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
	directives := []string{"controllerurl", "username", "password", "ttl", "refreshinterval", "sites"}
	for _, d := range directives {
		c := caddy.NewTestController("dns", "unifi {\n"+d+"\n}")
		if err := setup(c); err == nil {
			t.Fatalf("Expected error for missing arg on directive '%s', but got nil", d)
		}
	}
}

func TestSetupSites(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantSites []string
	}{
		{
			name:      "single site",
			input:     "unifi {\n\tsites default\n}",
			wantSites: []string{"default"},
		},
		{
			name:      "multiple sites comma-separated",
			input:     "unifi {\n\tsites site1,site2,site3\n}",
			wantSites: []string{"site1", "site2", "site3"},
		},
		{
			name:      "two sites",
			input:     "unifi {\n\tsites default,branch\n}",
			wantSites: []string{"default", "branch"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := parseSitesFromConfig(t, tt.input)
			if len(cfg) != len(tt.wantSites) {
				t.Fatalf("Expected %d sites, got %d: %v", len(tt.wantSites), len(cfg), cfg)
			}
			for i, s := range tt.wantSites {
				if cfg[i] != s {
					t.Errorf("sites[%d]: want %q, got %q", i, s, cfg[i])
				}
			}
		})
	}
}

// parseSitesFromConfig uses the caddy controller to parse the sites directive
// out of a minimal Corefile snippet, returning the parsed sites slice.
func parseSitesFromConfig(t *testing.T, input string) []string {
	t.Helper()
	c := caddy.NewTestController("dns", input)
	c.Next() // consume "unifi"
	_ = c.RemainingArgs()

	var sites []string
	for c.NextBlock() {
		if c.Val() == "sites" {
			if !c.NextArg() {
				t.Fatal("Expected argument for sites directive")
			}
			sites = strings.Split(c.Val(), ",")
		}
	}
	return sites
}
