package unifi

import (
	"context"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/plugin/pkg/fall"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

const (
	defaultTTL             = 30
	defaultRefreshInterval = 30
)

var log = clog.NewWithPlugin("unifi")

type UnifiConfigEntry struct {
	a   net.IP
	ttl uint32
}

type UnifiConfigEntryMap map[string]*UnifiConfigEntry

type UnifiConfig struct {
	controllerUrl   string
	username        string
	password        string
	ttl             uint32
	refreshInterval uint32
}

type Unifi struct {
	Next    plugin.Handler
	Config  *UnifiConfig
	Client  *UnifiClient
	Origins []string

	mappings      UnifiConfigEntryMap
	seenSanitized map[string]bool // tracks raw->sanitized mappings we've already logged
	mutex         sync.RWMutex
	fall          fall.F
	done          chan struct{}
}

func (u *Unifi) Name() string { return "unifi" }

func (u *Unifi) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {

	state := request.Request{W: w, Req: r}

	if state.QClass() != dns.ClassINET || state.QType() != dns.TypeA {
		return plugin.NextOrFailure(u.Name(), u.Next, ctx, w, r)
	}

	qname := state.QName()
	zone := plugin.Zones(u.Origins).Matches(qname)
	if zone == "" {
		return plugin.NextOrFailure(u.Name(), u.Next, ctx, w, r)
	}

	requestCount.WithLabelValues(metrics.WithServer(ctx)).Inc()

	answers := []dns.RR{}
	for _, q := range state.Req.Question {
		find := strings.ToLower(q.Name[:len(q.Name)-1])

		result := u.getEntry(find)
		if result != nil {
			rr := new(dns.A)
			rr.Hdr = dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: result.ttl}
			rr.A = result.a
			answers = append(answers, rr)
		}
	}

	if len(answers) == 0 {
		if u.fall.Through(qname) && u.Next != nil {
			log.Debug("Falling through. 0 answers")
			return plugin.NextOrFailure(u.Name(), u.Next, ctx, w, r)
		}
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	m.Answer = answers

	if len(answers) == 0 {
		log.Debug("Returning NXDOMAIN")
		m.Rcode = dns.RcodeNameError
	}

	if err := w.WriteMsg(m); err != nil {
		return dns.RcodeServerFailure, err
	}
	return dns.RcodeSuccess, nil
}

func (u *Unifi) start() {
	log.Info("Starting Unifi Query")
	err := u.refresh(true)

	if err != nil {
		log.Warningf("Failed to load clients from Unifi Controller, will retry: %s", err)
	}

	uptimeTicker := time.NewTicker(time.Duration(u.Config.refreshInterval) * time.Second)
	defer uptimeTicker.Stop()

	for {
		select {
		case <-uptimeTicker.C:
			log.Debug("Refreshing from Unifi Controller")
			err := u.refresh(false)
			if err != nil {
				log.Warningf("Error loading Unifi Clients: %s", err)
			}
		case <-u.done:
			log.Info("Shutting down Unifi refresh loop")
			return
		}
	}
}

func (u *Unifi) getEntry(host string) *UnifiConfigEntry {
	u.mutex.RLock()
	defer u.mutex.RUnlock()

	value, found := u.mappings[host]
	if !found {
		return nil
	}

	return value
}

var separatorPattern = regexp.MustCompile(`[\s_-]+`)
var invalidCharsPattern = regexp.MustCompile(`[^a-z0-9-]`)

// sanitizeHostname converts a raw name into a DNS-safe hostname label,
// matching UniFi's internal sanitization behavior.
func sanitizeHostname(name string) string {
	name = strings.ToLower(name)
	name = separatorPattern.ReplaceAllString(name, "-")
	name = invalidCharsPattern.ReplaceAllString(name, "")
	name = strings.Trim(name, "-")
	return name
}

// clientName returns the best available name for a client,
// preferring Name (UI alias) over Hostname (DHCP-reported),
// and sanitizes the result for use as a DNS hostname label.
func clientName(name, hostname string) string {
	raw := name
	if raw == "" {
		raw = hostname
	}
	return sanitizeHostname(raw)
}

func (u *Unifi) refresh(first bool) error {
	if first {
		log.Infof("Querying the Unifi Controller")
	}

	u.mutex.Lock()
	defer u.mutex.Unlock()

	sites, err := u.Client.api.GetSites()
	if err != nil {
		return err
	}

	// @TODO some way to filter/limit the list of sites.

	clients, err := u.Client.api.GetClients(sites)
	if err != nil {
		return err
	}

	networks, err := u.Client.api.GetNetworks(sites)
	if err != nil {
		return err
	}

	domains := map[string]string{}
	for _, network := range networks {
		domains[network.ID] = network.DomainName
	}

	if u.seenSanitized == nil {
		u.seenSanitized = make(map[string]bool)
	}

	keepClients := map[string]bool{}
	for _, client := range clients {
		name := clientName(client.Name, client.Hostname)
		if name == "" {
			log.Warningf("Skipping client %s (MAC %s): no usable name or hostname", client.IP, client.Mac)
			continue
		}

		domain := domains[client.NetworkID]
		if domain == "" {
			log.Warningf("Skipping client %q (%s): network %s has no domain name", name, client.IP, client.NetworkID)
			continue
		}

		record := name + "." + domain

		if existing, ok := u.mappings[record]; ok && keepClients[record] {
			log.Errorf("Hostname collision: %s already mapped to %s, ignoring duplicate %s", record, existing.a, client.IP)
			continue
		}

		ip := net.ParseIP(client.IP)

		// Log sanitization on first occurrence only
		raw := client.Name
		if raw == "" {
			raw = client.Hostname
		}
		if raw != name && !u.seenSanitized[raw] {
			log.Infof("Sanitized %q -> %q", raw, name)
			u.seenSanitized[raw] = true
		}

		// Log new or changed mappings
		existing := u.mappings[record]
		if existing == nil || !existing.a.Equal(ip) {
			log.Debugf("Mapped %s -> %s", record, client.IP)
		}

		u.mappings[record] = &UnifiConfigEntry{
			a:   ip,
			ttl: u.Config.ttl,
		}
		keepClients[record] = true
	}

	// Delete old mappings
	for key := range u.mappings {
		if !keepClients[key] {
			delete(u.mappings, key)
		}
	}

	return nil
}
