package unifi

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
	unpoller_unifi "github.com/unpoller/unifi"
)

func TestServeDNSHit(t *testing.T) {
	u := &Unifi{
		Next: test.ErrorHandler(),
		Config: &UnifiConfig{
			ttl: 30,
		},
		mappings: UnifiConfigEntryMap{
			"myhost.lan": {
				a:   net.ParseIP("192.168.1.100"),
				ttl: 30,
			},
		},
	}

	ctx := context.TODO()
	r := new(dns.Msg)
	r.SetQuestion("myhost.lan.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	code, err := u.ServeDNS(ctx, rec, r)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if code != dns.RcodeSuccess {
		t.Fatalf("Expected rcode %d, got %d", dns.RcodeSuccess, code)
	}
	if len(rec.Msg.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(rec.Msg.Answer))
	}

	a, ok := rec.Msg.Answer[0].(*dns.A)
	if !ok {
		t.Fatal("Expected A record in answer")
	}
	if !a.A.Equal(net.ParseIP("192.168.1.100")) {
		t.Fatalf("Expected 192.168.1.100, got %s", a.A)
	}
}

func TestServeDNSMiss(t *testing.T) {
	u := &Unifi{
		Next: test.ErrorHandler(),
		Config: &UnifiConfig{
			ttl: 30,
		},
		mappings: make(UnifiConfigEntryMap),
	}

	ctx := context.TODO()
	r := new(dns.Msg)
	r.SetQuestion("unknown.lan.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	code, err := u.ServeDNS(ctx, rec, r)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if code != dns.RcodeSuccess {
		t.Fatalf("Expected rcode %d, got %d", dns.RcodeSuccess, code)
	}
	if rec.Msg.Rcode != dns.RcodeNameError {
		t.Fatalf("Expected NXDOMAIN, got rcode %d", rec.Msg.Rcode)
	}
}

func TestServeDNSNonARecord(t *testing.T) {
	u := &Unifi{
		Next: test.ErrorHandler(),
		Config: &UnifiConfig{
			ttl: 30,
		},
		mappings: make(UnifiConfigEntryMap),
	}

	ctx := context.TODO()
	r := new(dns.Msg)
	r.SetQuestion("myhost.lan.", dns.TypeAAAA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	// Non-A queries should pass through to Next handler
	u.ServeDNS(ctx, rec, r)
}

func TestGetEntry(t *testing.T) {
	u := &Unifi{
		mappings: UnifiConfigEntryMap{
			"test.lan": {
				a:   net.ParseIP("10.0.0.1"),
				ttl: 60,
			},
		},
	}

	entry := u.getEntry("test.lan")
	if entry == nil {
		t.Fatal("Expected entry, got nil")
	}
	if !entry.a.Equal(net.ParseIP("10.0.0.1")) {
		t.Fatalf("Expected 10.0.0.1, got %s", entry.a)
	}

	entry = u.getEntry("missing.lan")
	if entry != nil {
		t.Fatal("Expected nil for missing entry")
	}
}

func TestName(t *testing.T) {
	u := &Unifi{}
	if u.Name() != "unifi" {
		t.Fatalf("Expected 'unifi', got '%s'", u.Name())
	}
}

func TestReady(t *testing.T) {
	u := &Unifi{}
	if !u.Ready() {
		t.Fatal("Expected Ready() to return true")
	}
}

func newTestUnifi(mock *mockUnifiAPI) *Unifi {
	return &Unifi{
		Config: &UnifiConfig{ttl: 30},
		Client: &UnifiClient{api: mock},
		mappings: make(UnifiConfigEntryMap),
	}
}

func TestRefreshBuildsMapping(t *testing.T) {
	mock := &mockUnifiAPI{
		sites: []*unpoller_unifi.Site{{Name: "default"}},
		clients: []*unpoller_unifi.Client{
			{Name: "desktop", IP: "192.168.1.10", NetworkID: "net1"},
			{Name: "laptop", IP: "192.168.1.11", NetworkID: "net1"},
		},
		networks: []unpoller_unifi.Network{
			{ID: "net1", DomainName: "home.lan"},
		},
	}

	u := newTestUnifi(mock)
	if err := u.refresh(true); err != nil {
		t.Fatalf("refresh failed: %v", err)
	}

	if len(u.mappings) != 2 {
		t.Fatalf("Expected 2 mappings, got %d", len(u.mappings))
	}

	entry := u.mappings["desktop.home.lan"]
	if entry == nil {
		t.Fatal("Expected mapping for desktop.home.lan")
	}
	if !entry.a.Equal(net.ParseIP("192.168.1.10")) {
		t.Fatalf("Expected 192.168.1.10, got %s", entry.a)
	}
	if entry.ttl != 30 {
		t.Fatalf("Expected ttl 30, got %d", entry.ttl)
	}

	entry = u.mappings["laptop.home.lan"]
	if entry == nil {
		t.Fatal("Expected mapping for laptop.home.lan")
	}
}

func TestRefreshRemovesStaleEntries(t *testing.T) {
	mock := &mockUnifiAPI{
		sites: []*unpoller_unifi.Site{{Name: "default"}},
		clients: []*unpoller_unifi.Client{
			{Name: "desktop", IP: "192.168.1.10", NetworkID: "net1"},
			{Name: "laptop", IP: "192.168.1.11", NetworkID: "net1"},
		},
		networks: []unpoller_unifi.Network{
			{ID: "net1", DomainName: "home.lan"},
		},
	}

	u := newTestUnifi(mock)
	u.refresh(true)

	// Second refresh — laptop is gone
	mock.clients = []*unpoller_unifi.Client{
		{Name: "desktop", IP: "192.168.1.10", NetworkID: "net1"},
	}

	if err := u.refresh(false); err != nil {
		t.Fatalf("refresh failed: %v", err)
	}

	if len(u.mappings) != 1 {
		t.Fatalf("Expected 1 mapping after stale removal, got %d", len(u.mappings))
	}
	if u.mappings["laptop.home.lan"] != nil {
		t.Fatal("Expected laptop.home.lan to be removed")
	}
}

func TestRefreshMultipleNetworks(t *testing.T) {
	mock := &mockUnifiAPI{
		sites: []*unpoller_unifi.Site{{Name: "default"}},
		clients: []*unpoller_unifi.Client{
			{Name: "server", IP: "10.0.0.5", NetworkID: "net1"},
			{Name: "phone", IP: "192.168.2.20", NetworkID: "net2"},
		},
		networks: []unpoller_unifi.Network{
			{ID: "net1", DomainName: "corp.local"},
			{ID: "net2", DomainName: "iot.local"},
		},
	}

	u := newTestUnifi(mock)
	u.refresh(true)

	if u.mappings["server.corp.local"] == nil {
		t.Fatal("Expected mapping for server.corp.local")
	}
	if u.mappings["phone.iot.local"] == nil {
		t.Fatal("Expected mapping for phone.iot.local")
	}
}

func TestRefreshSitesError(t *testing.T) {
	mock := &mockUnifiAPI{
		sitesErr: fmt.Errorf("connection refused"),
	}

	u := newTestUnifi(mock)
	err := u.refresh(true)
	if err == nil {
		t.Fatal("Expected error from refresh when GetSites fails")
	}
}

func TestRefreshClientsError(t *testing.T) {
	mock := &mockUnifiAPI{
		sites:      []*unpoller_unifi.Site{{Name: "default"}},
		clientsErr: fmt.Errorf("timeout"),
	}

	u := newTestUnifi(mock)
	err := u.refresh(true)
	if err == nil {
		t.Fatal("Expected error from refresh when GetClients fails")
	}
}

func TestRefreshNetworksError(t *testing.T) {
	mock := &mockUnifiAPI{
		sites:       []*unpoller_unifi.Site{{Name: "default"}},
		clients:     []*unpoller_unifi.Client{},
		networksErr: fmt.Errorf("timeout"),
	}

	u := newTestUnifi(mock)
	err := u.refresh(true)
	if err == nil {
		t.Fatal("Expected error from refresh when GetNetworks fails")
	}
}

func TestRefreshFallsBackToHostname(t *testing.T) {
	mock := &mockUnifiAPI{
		sites: []*unpoller_unifi.Site{{Name: "default"}},
		clients: []*unpoller_unifi.Client{
			{Name: "", Hostname: "dhcp-reported", IP: "192.168.1.50", NetworkID: "net1"},
		},
		networks: []unpoller_unifi.Network{
			{ID: "net1", DomainName: "home.lan"},
		},
	}

	u := newTestUnifi(mock)
	u.refresh(true)

	if u.mappings["dhcp-reported.home.lan"] == nil {
		t.Fatal("Expected mapping using Hostname fallback")
	}
}

func TestRefreshNameTakesPrecedenceOverHostname(t *testing.T) {
	mock := &mockUnifiAPI{
		sites: []*unpoller_unifi.Site{{Name: "default"}},
		clients: []*unpoller_unifi.Client{
			{Name: "my-alias", Hostname: "dhcp-name", IP: "192.168.1.50", NetworkID: "net1"},
		},
		networks: []unpoller_unifi.Network{
			{ID: "net1", DomainName: "home.lan"},
		},
	}

	u := newTestUnifi(mock)
	u.refresh(true)

	if u.mappings["my-alias.home.lan"] == nil {
		t.Fatal("Expected mapping using Name over Hostname")
	}
	if u.mappings["dhcp-name.home.lan"] != nil {
		t.Fatal("Should not have mapping for Hostname when Name is set")
	}
}

func TestRefreshSkipsClientWithNoName(t *testing.T) {
	mock := &mockUnifiAPI{
		sites: []*unpoller_unifi.Site{{Name: "default"}},
		clients: []*unpoller_unifi.Client{
			{Name: "", Hostname: "", IP: "192.168.1.99", NetworkID: "net1"},
			{Name: "known", IP: "192.168.1.10", NetworkID: "net1"},
		},
		networks: []unpoller_unifi.Network{
			{ID: "net1", DomainName: "home.lan"},
		},
	}

	u := newTestUnifi(mock)
	u.refresh(true)

	if len(u.mappings) != 1 {
		t.Fatalf("Expected 1 mapping (nameless client skipped), got %d", len(u.mappings))
	}
}

func TestRefreshSkipsClientWithUnknownNetwork(t *testing.T) {
	mock := &mockUnifiAPI{
		sites: []*unpoller_unifi.Site{{Name: "default"}},
		clients: []*unpoller_unifi.Client{
			{Name: "orphan", IP: "192.168.1.99", NetworkID: "unknown-net-id"},
			{Name: "known", IP: "192.168.1.10", NetworkID: "net1"},
		},
		networks: []unpoller_unifi.Network{
			{ID: "net1", DomainName: "home.lan"},
		},
	}

	u := newTestUnifi(mock)
	u.refresh(true)

	if len(u.mappings) != 1 {
		t.Fatalf("Expected 1 mapping (unknown network skipped), got %d", len(u.mappings))
	}
	if u.mappings["known.home.lan"] == nil {
		t.Fatal("Expected mapping for known.home.lan")
	}
}

func TestRefreshSkipsNetworkWithEmptyDomain(t *testing.T) {
	mock := &mockUnifiAPI{
		sites: []*unpoller_unifi.Site{{Name: "default"}},
		clients: []*unpoller_unifi.Client{
			{Name: "device", IP: "192.168.1.5", NetworkID: "net-no-domain"},
		},
		networks: []unpoller_unifi.Network{
			{ID: "net-no-domain", DomainName: ""},
		},
	}

	u := newTestUnifi(mock)
	u.refresh(true)

	if len(u.mappings) != 0 {
		t.Fatalf("Expected 0 mappings (empty domain), got %d", len(u.mappings))
	}
}

func TestClientName(t *testing.T) {
	tests := []struct {
		name, hostname, want string
	}{
		{"alias", "dhcp-host", "alias"},
		{"", "dhcp-host", "dhcp-host"},
		{"alias", "", "alias"},
		{"", "", ""},
	}
	for _, tt := range tests {
		got := clientName(tt.name, tt.hostname)
		if got != tt.want {
			t.Errorf("clientName(%q, %q) = %q, want %q", tt.name, tt.hostname, got, tt.want)
		}
	}
}

func TestStartShutdown(t *testing.T) {
	mock := &mockUnifiAPI{
		sites:    []*unpoller_unifi.Site{{Name: "default"}},
		clients:  []*unpoller_unifi.Client{},
		networks: []unpoller_unifi.Network{},
	}

	u := &Unifi{
		Config:   &UnifiConfig{ttl: 30, refreshInterval: 1},
		Client:   &UnifiClient{api: mock},
		mappings: make(UnifiConfigEntryMap),
		done:     make(chan struct{}),
	}

	started := make(chan struct{})
	go func() {
		close(started)
		u.start()
	}()

	<-started
	close(u.done)
	// If start() doesn't return, the test will timeout and fail.
}
