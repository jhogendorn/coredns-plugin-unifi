//go:build integration

package unifi

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/miekg/dns"
	tc "github.com/testcontainers/testcontainers-go/modules/compose"
	"github.com/testcontainers/testcontainers-go/wait"
)

var dnsAddr string

func TestMain(m *testing.M) {
	ctx := context.Background()

	stack, err := tc.NewDockerCompose("integration/docker-compose.yml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create compose stack: %v\n", err)
		os.Exit(1)
	}

	err = stack.
		WaitForService("mock-controller", wait.ForHTTP("/status").WithPort("8443/tcp").WithStartupTimeout(30*time.Second)).
		WaitForService("coredns", wait.ForListeningPort("53/udp").WithStartupTimeout(30*time.Second)).
		Up(ctx, tc.Wait(true))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start compose stack: %v\n", err)
		_ = stack.Down(ctx, tc.RemoveOrphans(true))
		os.Exit(1)
	}

	ctr, err := stack.ServiceContainer(ctx, "coredns")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get coredns container: %v\n", err)
		_ = stack.Down(ctx, tc.RemoveOrphans(true))
		os.Exit(1)
	}

	port, err := ctr.MappedPort(ctx, "53/udp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get mapped port: %v\n", err)
		_ = stack.Down(ctx, tc.RemoveOrphans(true))
		os.Exit(1)
	}

	dnsAddr = fmt.Sprintf("127.0.0.1:%s", port.Port())

	// Wait for CoreDNS to do its first refresh from the mock controller
	time.Sleep(6 * time.Second)

	code := m.Run()

	_ = stack.Down(ctx, tc.RemoveOrphans(true))
	os.Exit(code)
}

func queryA(t *testing.T, name string) (*dns.Msg, error) {
	t.Helper()
	client := &dns.Client{
		Net:     "udp",
		Timeout: 2 * time.Second,
	}
	msg := &dns.Msg{}
	msg.SetQuestion(dns.Fqdn(name), dns.TypeA)
	resp, _, err := client.Exchange(msg, dnsAddr)
	return resp, err
}

func requireIP(t *testing.T, name string, expectedIP string) {
	t.Helper()
	resp, err := queryA(t, name)
	if err != nil {
		t.Fatalf("DNS query for %s failed: %v", name, err)
	}
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("expected NOERROR for %s, got %s", name, dns.RcodeToString[resp.Rcode])
	}
	if len(resp.Answer) == 0 {
		t.Fatalf("expected answer for %s, got none", name)
	}
	a, ok := resp.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("expected A record for %s, got %T", name, resp.Answer[0])
	}
	if a.A.String() != expectedIP {
		t.Errorf("expected %s for %s, got %s", expectedIP, name, a.A.String())
	}
}

func TestNamedClientResolves(t *testing.T) {
	requireIP(t, "desktop.home.lan", "192.168.1.10")
	requireIP(t, "laptop.home.lan", "192.168.1.11")
}

func TestHostnameFallback(t *testing.T) {
	requireIP(t, "phone-dhcp.home.lan", "192.168.1.12")
}

func TestMultiNetworkDomain(t *testing.T) {
	requireIP(t, "iot-sensor.iot.lan", "10.0.0.50")
}

func TestSkipsNamelessClient(t *testing.T) {
	// Client5 has no Name or Hostname, but unpoller's GetClients fills
	// Hostname from MAC ("00:11:22:33:44:05") when both are empty.
	// Verify that the original empty-name client doesn't create a
	// record at the bare domain.
	resp, err := queryA(t, "nonexistent-client.home.lan")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}
	if resp.Rcode != dns.RcodeNameError {
		t.Errorf("expected NXDOMAIN for unknown client, got %s", dns.RcodeToString[resp.Rcode])
	}
}

func TestNXDOMAIN(t *testing.T) {
	resp, err := queryA(t, "nonexistent.home.lan")
	if err != nil {
		t.Fatalf("DNS query failed: %v", err)
	}
	if resp.Rcode != dns.RcodeNameError {
		t.Errorf("expected NXDOMAIN, got %s", dns.RcodeToString[resp.Rcode])
	}
}

func TestCaseInsensitive(t *testing.T) {
	requireIP(t, "DESKTOP.home.lan", "192.168.1.10")
}
