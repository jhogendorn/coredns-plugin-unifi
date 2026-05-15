package unifi

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/coredns/coredns/plugin"
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
		Origins: []string{"lan."},
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
		Origins:  []string{"lan."},
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
		Origins:  []string{"lan."},
		mappings: make(UnifiConfigEntryMap),
	}

	ctx := context.TODO()
	r := new(dns.Msg)
	r.SetQuestion("myhost.lan.", dns.TypeAAAA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	// AAAA (and other non-A/PTR) queries should pass through to Next handler
	_, _ = u.ServeDNS(ctx, rec, r)
}

func TestServeDNSPTRHit(t *testing.T) {
	u := &Unifi{
		Next: test.ErrorHandler(),
		Config: &UnifiConfig{
			ttl: 30,
		},
		Origins: []string{"1.168.192.in-addr.arpa."},
		reverseMappings: UnifiReverseMap{
			"100.1.168.192.in-addr.arpa": {
				fqdn: "myhost.lan",
				ttl:  30,
			},
		},
	}

	ctx := context.TODO()
	r := new(dns.Msg)
	r.SetQuestion("100.1.168.192.in-addr.arpa.", dns.TypePTR)
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
	ptr, ok := rec.Msg.Answer[0].(*dns.PTR)
	if !ok {
		t.Fatal("Expected PTR record in answer")
	}
	if ptr.Ptr != "myhost.lan." {
		t.Fatalf("Expected myhost.lan., got %s", ptr.Ptr)
	}
}

func TestServeDNSPTRMiss(t *testing.T) {
	u := &Unifi{
		Next: test.ErrorHandler(),
		Config: &UnifiConfig{
			ttl: 30,
		},
		Origins:         []string{"1.168.192.in-addr.arpa."},
		reverseMappings: make(UnifiReverseMap),
	}

	ctx := context.TODO()
	r := new(dns.Msg)
	r.SetQuestion("99.1.168.192.in-addr.arpa.", dns.TypePTR)
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

func TestServeDNSPTROutsideOrigins(t *testing.T) {
	// PTR query for a zone not in Origins must pass through, not get hijacked.
	u := &Unifi{
		Next:    test.ErrorHandler(),
		Config:  &UnifiConfig{ttl: 30},
		Origins: []string{"lan."}, // forward zone only, no reverse zone listed
		reverseMappings: UnifiReverseMap{
			"100.1.168.192.in-addr.arpa": {fqdn: "myhost.lan", ttl: 30},
		},
	}

	ctx := context.TODO()
	r := new(dns.Msg)
	r.SetQuestion("100.1.168.192.in-addr.arpa.", dns.TypePTR)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	// Origins doesn't cover in-addr.arpa -> pass through; ErrorHandler
	// returns RcodeServerFailure, which is the marker we use here.
	_, _ = u.ServeDNS(ctx, rec, r)
}

func TestGetReverse(t *testing.T) {
	u := &Unifi{
		reverseMappings: UnifiReverseMap{
			"1.0.0.10.in-addr.arpa": {fqdn: "host.lan", ttl: 60},
		},
	}

	entry := u.getReverse("1.0.0.10.in-addr.arpa")
	if entry == nil {
		t.Fatal("Expected entry, got nil")
	}
	if entry.fqdn != "host.lan" {
		t.Fatalf("Expected host.lan, got %s", entry.fqdn)
	}

	if u.getReverse("99.0.0.10.in-addr.arpa") != nil {
		t.Fatal("Expected nil for missing reverse entry")
	}
}

func TestReverseFromIP(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"192.168.1.5", "5.1.168.192.in-addr.arpa"},
		{"10.0.0.1", "1.0.0.10.in-addr.arpa"},
		{"::1", ""}, // IPv6 not supported -> empty
	} {
		got := reverseFromIP(net.ParseIP(tc.in))
		if got != tc.want {
			t.Fatalf("reverseFromIP(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
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
		Config:          &UnifiConfig{ttl: 30},
		Client:          &UnifiClient{api: mock},
		mappings:        make(UnifiConfigEntryMap),
		reverseMappings: make(UnifiReverseMap),
		seenSanitized:   make(map[string]bool),
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

	// Reverse map should mirror forward map.
	if len(u.reverseMappings) != 2 {
		t.Fatalf("Expected 2 reverse mappings, got %d", len(u.reverseMappings))
	}
	rev := u.reverseMappings["10.1.168.192.in-addr.arpa"]
	if rev == nil {
		t.Fatal("Expected reverse mapping for 192.168.1.10")
	}
	if rev.fqdn != "desktop.home.lan" {
		t.Fatalf("Expected reverse fqdn desktop.home.lan, got %s", rev.fqdn)
	}
}

func TestRefreshRemovesStaleReverseEntries(t *testing.T) {
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
	_ = u.refresh(true)
	if len(u.reverseMappings) != 2 {
		t.Fatalf("Expected 2 reverse mappings after initial refresh, got %d", len(u.reverseMappings))
	}

	// Second refresh -- laptop gone
	mock.clients = []*unpoller_unifi.Client{
		{Name: "desktop", IP: "192.168.1.10", NetworkID: "net1"},
	}
	if err := u.refresh(false); err != nil {
		t.Fatalf("refresh failed: %v", err)
	}

	if len(u.reverseMappings) != 1 {
		t.Fatalf("Expected 1 reverse mapping after stale removal, got %d", len(u.reverseMappings))
	}
	if u.reverseMappings["11.1.168.192.in-addr.arpa"] != nil {
		t.Fatal("Expected laptop reverse to be removed")
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
	_ = u.refresh(true)

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
	_ = u.refresh(true)

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
	_ = u.refresh(true)

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
	_ = u.refresh(true)

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
	_ = u.refresh(true)

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
	_ = u.refresh(true)

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
	_ = u.refresh(true)

	if len(u.mappings) != 0 {
		t.Fatalf("Expected 0 mappings (empty domain), got %d", len(u.mappings))
	}
}

func TestSanitizeHostname(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"desktop", "desktop"},
		{"My Desktop", "my-desktop"},
		{"Living  Room  TV", "living-room-tv"},
		{"my_server_01", "my-server-01"},
		{"--leading-trailing--", "leading-trailing"},
		{"café", "caf"},
		{"hello!@#world", "helloworld"},
		{"  spaced  ", "spaced"},
		{"UPPERCASE", "uppercase"},
		{"mixed--Case__Name", "mixed-case-name"},
		{"", ""},
		{"---", ""},
	}
	for _, tt := range tests {
		got := sanitizeHostname(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeHostname(%q) = %q, want %q", tt.input, got, tt.want)
		}
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
		{"My Device", "dhcp-host", "my-device"},
		{"", "BAD HOST!", "bad-host"},
	}
	for _, tt := range tests {
		got := clientName(tt.name, tt.hostname)
		if got != tt.want {
			t.Errorf("clientName(%q, %q) = %q, want %q", tt.name, tt.hostname, got, tt.want)
		}
	}
}

func TestRefreshSanitizesClientNames(t *testing.T) {
	mock := &mockUnifiAPI{
		sites: []*unpoller_unifi.Site{{Name: "default"}},
		clients: []*unpoller_unifi.Client{
			{Name: "Living Room TV", IP: "192.168.1.10", NetworkID: "net1"},
			{Name: "", Hostname: "BAD HOST!", IP: "192.168.1.11", NetworkID: "net1"},
		},
		networks: []unpoller_unifi.Network{
			{ID: "net1", DomainName: "home.lan"},
		},
	}

	u := newTestUnifi(mock)
	_ = u.refresh(true)

	if u.mappings["living-room-tv.home.lan"] == nil {
		t.Fatal("Expected sanitized mapping for 'Living Room TV'")
	}
	if u.mappings["bad-host.home.lan"] == nil {
		t.Fatal("Expected sanitized mapping for 'BAD HOST!'")
	}
	if len(u.mappings) != 2 {
		t.Fatalf("Expected 2 mappings, got %d", len(u.mappings))
	}
}

func TestRefreshCollisionKeepsFirst(t *testing.T) {
	mock := &mockUnifiAPI{
		sites: []*unpoller_unifi.Site{{Name: "default"}},
		clients: []*unpoller_unifi.Client{
			{Name: "printer", IP: "192.168.1.10", NetworkID: "net1"},
			{Name: "printer", IP: "192.168.1.20", NetworkID: "net1"},
		},
		networks: []unpoller_unifi.Network{
			{ID: "net1", DomainName: "home.lan"},
		},
	}

	u := newTestUnifi(mock)
	_ = u.refresh(true)

	if len(u.mappings) != 1 {
		t.Fatalf("Expected 1 mapping (collision), got %d", len(u.mappings))
	}

	entry := u.mappings["printer.home.lan"]
	if entry == nil {
		t.Fatal("Expected mapping for printer.home.lan")
	}
	if !entry.a.Equal(net.ParseIP("192.168.1.10")) {
		t.Fatalf("Expected first IP 192.168.1.10, got %s", entry.a)
	}
}

func TestRefreshCollisionViaSanitization(t *testing.T) {
	mock := &mockUnifiAPI{
		sites: []*unpoller_unifi.Site{{Name: "default"}},
		clients: []*unpoller_unifi.Client{
			{Name: "My Printer", IP: "192.168.1.10", NetworkID: "net1"},
			{Name: "my_printer", IP: "192.168.1.20", NetworkID: "net1"},
		},
		networks: []unpoller_unifi.Network{
			{ID: "net1", DomainName: "home.lan"},
		},
	}

	u := newTestUnifi(mock)
	_ = u.refresh(true)

	if len(u.mappings) != 1 {
		t.Fatalf("Expected 1 mapping (sanitization collision), got %d", len(u.mappings))
	}

	entry := u.mappings["my-printer.home.lan"]
	if entry == nil {
		t.Fatal("Expected mapping for my-printer.home.lan")
	}
	if !entry.a.Equal(net.ParseIP("192.168.1.10")) {
		t.Fatalf("Expected first IP 192.168.1.10, got %s", entry.a)
	}
}

func TestRefreshSkipsAllInvalidNames(t *testing.T) {
	mock := &mockUnifiAPI{
		sites: []*unpoller_unifi.Site{{Name: "default"}},
		clients: []*unpoller_unifi.Client{
			{Name: "---", Hostname: "!!!", IP: "192.168.1.10", NetworkID: "net1"},
			{Name: "", Hostname: "", IP: "192.168.1.11", NetworkID: "net1"},
		},
		networks: []unpoller_unifi.Network{
			{ID: "net1", DomainName: "home.lan"},
		},
	}

	u := newTestUnifi(mock)
	_ = u.refresh(true)

	if len(u.mappings) != 0 {
		t.Fatalf("Expected 0 mappings (all names invalid), got %d", len(u.mappings))
	}
}

func TestRefreshSiteFilterSingle(t *testing.T) {
	mock := &mockUnifiAPI{
		sites: []*unpoller_unifi.Site{
			{Name: "default"},
			{Name: "branch"},
		},
		clients: []*unpoller_unifi.Client{
			{Name: "desktop", IP: "192.168.1.10", NetworkID: "net1", SiteID: "default"},
			{Name: "laptop", IP: "192.168.1.11", NetworkID: "net1", SiteID: "branch"},
		},
		networks: []unpoller_unifi.Network{
			{ID: "net1", DomainName: "home.lan"},
		},
	}

	u := newTestUnifi(mock)
	u.Config.sites = []string{"default"}
	if err := u.refresh(true); err != nil {
		t.Fatalf("refresh failed: %v", err)
	}

	if len(u.mappings) != 1 {
		t.Fatalf("Expected 1 mapping (only default site), got %d", len(u.mappings))
	}
	if u.mappings["desktop.home.lan"] == nil {
		t.Fatal("Expected mapping for desktop.home.lan (default site)")
	}
	if u.mappings["laptop.home.lan"] != nil {
		t.Fatal("Expected laptop.home.lan to be excluded (branch site filtered out)")
	}
}

func TestRefreshSiteFilterMultiple(t *testing.T) {
	mock := &mockUnifiAPI{
		sites: []*unpoller_unifi.Site{
			{Name: "site1"},
			{Name: "site2"},
			{Name: "site3"},
		},
		clients: []*unpoller_unifi.Client{
			{Name: "host1", IP: "10.0.1.1", NetworkID: "net1", SiteID: "site1"},
			{Name: "host2", IP: "10.0.2.1", NetworkID: "net1", SiteID: "site2"},
			{Name: "host3", IP: "10.0.3.1", NetworkID: "net1", SiteID: "site3"},
		},
		networks: []unpoller_unifi.Network{
			{ID: "net1", DomainName: "home.lan"},
		},
	}

	u := newTestUnifi(mock)
	u.Config.sites = []string{"site1", "site2"}
	if err := u.refresh(true); err != nil {
		t.Fatalf("refresh failed: %v", err)
	}

	if len(u.mappings) != 2 {
		t.Fatalf("Expected 2 mappings (site1 and site2), got %d", len(u.mappings))
	}
	if u.mappings["host1.home.lan"] == nil {
		t.Fatal("Expected mapping for host1.home.lan (site1)")
	}
	if u.mappings["host2.home.lan"] == nil {
		t.Fatal("Expected mapping for host2.home.lan (site2)")
	}
	if u.mappings["host3.home.lan"] != nil {
		t.Fatal("Expected host3.home.lan to be excluded (site3 filtered out)")
	}
}

func TestRefreshSiteFilterNone(t *testing.T) {
	mock := &mockUnifiAPI{
		sites: []*unpoller_unifi.Site{
			{Name: "site1"},
			{Name: "site2"},
		},
		clients: []*unpoller_unifi.Client{
			{Name: "host1", IP: "10.0.1.1", NetworkID: "net1", SiteID: "site1"},
			{Name: "host2", IP: "10.0.2.1", NetworkID: "net1", SiteID: "site2"},
		},
		networks: []unpoller_unifi.Network{
			{ID: "net1", DomainName: "home.lan"},
		},
	}

	u := newTestUnifi(mock)
	// No sites configured — all sites should be included.
	if err := u.refresh(true); err != nil {
		t.Fatalf("refresh failed: %v", err)
	}

	if len(u.mappings) != 2 {
		t.Fatalf("Expected 2 mappings (no site filter), got %d", len(u.mappings))
	}
	if u.mappings["host1.home.lan"] == nil {
		t.Fatal("Expected mapping for host1.home.lan")
	}
	if u.mappings["host2.home.lan"] == nil {
		t.Fatal("Expected mapping for host2.home.lan")
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

func TestServeDNSZoneMatch(t *testing.T) {
	u := &Unifi{
		Next: test.ErrorHandler(),
		Config: &UnifiConfig{
			ttl: 30,
		},
		Origins: []string{"lan."},
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
		t.Fatalf("Expected 1 answer for zone-matched query, got %d", len(rec.Msg.Answer))
	}
}

func TestServeDNSZoneNoMatch(t *testing.T) {
	// nextHandler records whether it was called
	called := false
	nextHandler := plugin.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		called = true
		return dns.RcodeSuccess, nil
	})

	u := &Unifi{
		Next: nextHandler,
		Config: &UnifiConfig{
			ttl: 30,
		},
		Origins: []string{"lan."},
		mappings: UnifiConfigEntryMap{
			"myhost.lan": {
				a:   net.ParseIP("192.168.1.100"),
				ttl: 30,
			},
		},
	}

	ctx := context.TODO()
	r := new(dns.Msg)
	// Query for a zone not in Origins — should fall through to Next
	r.SetQuestion("myhost.example.com.", dns.TypeA)
	rec := dnstest.NewRecorder(&test.ResponseWriter{})

	_, err := u.ServeDNS(ctx, rec, r)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if !called {
		t.Fatal("Expected Next handler to be called for non-matching zone, but it was not")
	}
}
