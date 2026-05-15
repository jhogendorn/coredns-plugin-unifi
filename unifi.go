package unifi

import (
	"context"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/plugin/pkg/fall"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	unpoller_unifi "github.com/unpoller/unifi"
)

const (
	defaultTTL             = 30
	defaultRefreshInterval = 30
)

var log = clog.NewWithPlugin("unifi")

// UnifiConfigEntry holds all IPs published for a single DNS name. A
// client can legitimately appear on multiple interfaces (wired + wifi)
// under a single name -- we publish all of its IPs as A records and
// let DNS-level round-robin / client preference sort it out.
type UnifiConfigEntry struct {
	ips []net.IP
	ttl uint32
}

type UnifiConfigEntryMap map[string]*UnifiConfigEntry

// UnifiReverseEntry is the reverse-lookup answer: the FQDN that owns an IP,
// plus the TTL to publish for the PTR. Stored in UnifiReverseMap keyed by the
// IP's in-addr.arpa name (e.g. "5.1.168.192.in-addr.arpa").
type UnifiReverseEntry struct {
	fqdn string
	ttl  uint32
}

type UnifiReverseMap map[string]*UnifiReverseEntry

// reverseFromIP returns the IPv4 in-addr.arpa name for an IP (no trailing
// dot). Returns "" for non-IPv4 addresses.
func reverseFromIP(ip net.IP) string {
	v4 := ip.To4()
	if v4 == nil {
		return ""
	}
	return strings.Join([]string{
		strconv.Itoa(int(v4[3])),
		strconv.Itoa(int(v4[2])),
		strconv.Itoa(int(v4[1])),
		strconv.Itoa(int(v4[0])),
		"in-addr.arpa",
	}, ".")
}

type UnifiConfig struct {
	controllerUrl   string
	username        string
	password        string
	ttl             uint32
	refreshInterval uint32
	sites           []string
}

type Unifi struct {
	Next    plugin.Handler
	Config  *UnifiConfig
	Client  *UnifiClient
	Origins []string

	mappings        UnifiConfigEntryMap
	reverseMappings UnifiReverseMap // IP-in-addr.arpa -> FQDN. Built alongside mappings.
	seenSanitized   map[string]bool // tracks raw->sanitized mappings we've already logged
	mutex           sync.RWMutex
	fall            fall.F
	done            chan struct{}
}

func (u *Unifi) Name() string { return "unifi" }

func (u *Unifi) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {

	state := request.Request{W: w, Req: r}

	if state.QClass() != dns.ClassINET {
		return plugin.NextOrFailure(u.Name(), u.Next, ctx, w, r)
	}
	if state.QType() != dns.TypeA && state.QType() != dns.TypePTR {
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

		switch q.Qtype {
		case dns.TypeA:
			result := u.getEntry(find)
			if result != nil {
				for _, ip := range result.ips {
					rr := new(dns.A)
					rr.Hdr = dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: result.ttl}
					rr.A = ip
					answers = append(answers, rr)
				}
			}
		case dns.TypePTR:
			result := u.getReverse(find)
			if result != nil {
				rr := new(dns.PTR)
				rr.Hdr = dns.RR_Header{Name: q.Name, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: result.ttl}
				rr.Ptr = dns.Fqdn(result.fqdn)
				answers = append(answers, rr)
			}
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

func (u *Unifi) getReverse(name string) *UnifiReverseEntry {
	u.mutex.RLock()
	defer u.mutex.RUnlock()

	value, found := u.reverseMappings[name]
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

	if len(u.Config.sites) > 0 {
		filtered := make([]*unpoller_unifi.Site, 0, len(sites))
		for _, site := range sites {
			for _, allowed := range u.Config.sites {
				if site.Name == allowed {
					filtered = append(filtered, site)
					break
				}
			}
		}
		sites = filtered
	}

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
	if u.reverseMappings == nil {
		u.reverseMappings = make(UnifiReverseMap)
	}

	keepClients := map[string]bool{}
	keepReverse := map[string]bool{}
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

		ip := net.ParseIP(client.IP)
		if ip == nil {
			continue
		}

		// Log sanitization on first occurrence only
		raw := client.Name
		if raw == "" {
			raw = client.Hostname
		}
		if raw != name && !u.seenSanitized[raw] {
			log.Infof("Sanitized %q -> %q", raw, name)
			u.seenSanitized[raw] = true
		}

		// Append this client's IP to the record's IP set, deduping
		// against any already-collected IPs (the controller occasionally
		// surfaces the same lease twice; we want each IP listed once).
		existing := u.mappings[record]
		if existing == nil {
			u.mappings[record] = &UnifiConfigEntry{
				ips: []net.IP{ip},
				ttl: u.Config.ttl,
			}
			log.Debugf("Mapped %s -> %s", record, client.IP)
		} else {
			alreadyHave := false
			for _, have := range existing.ips {
				if have.Equal(ip) {
					alreadyHave = true
					break
				}
			}
			if !alreadyHave {
				existing.ips = append(existing.ips, ip)
				log.Debugf("Mapped %s -> %s (additional IP, %d total)", record, client.IP, len(existing.ips))
			}
		}
		keepClients[record] = true

		// Reverse mapping: every IP gets its own PTR pointing back to
		// the record. Multiple IPs -> multiple reverse entries, all
		// resolving to the same FQDN. A reverse-mapping collision (same
		// IP, different FQDN) is the only truly anomalous case and is
		// still logged.
		if rev := reverseFromIP(ip); rev != "" {
			if existingRev, ok := u.reverseMappings[rev]; ok && keepReverse[rev] && existingRev.fqdn != record {
				log.Errorf("Reverse-mapping collision: %s already mapped to %s, ignoring duplicate %s", rev, existingRev.fqdn, record)
			} else {
				u.reverseMappings[rev] = &UnifiReverseEntry{
					fqdn: record,
					ttl:  u.Config.ttl,
				}
				keepReverse[rev] = true
			}
		}
	}

	// Delete old mappings
	for key := range u.mappings {
		if !keepClients[key] {
			delete(u.mappings, key)
		}
	}
	for key := range u.reverseMappings {
		if !keepReverse[key] {
			delete(u.reverseMappings, key)
		}
	}

	return nil
}
