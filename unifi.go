package unifi

import (
	"context"
	"net"
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
	Next   plugin.Handler
	Config *UnifiConfig
	Client *UnifiClient

	mappings UnifiConfigEntryMap
	ready    bool
	mutex    sync.RWMutex
	fall     fall.F
	done     chan struct{}
}

func (u *Unifi) Name() string { return "unifi" }

func (u *Unifi) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {

	state := request.Request{W: w, Req: r}

	if state.QClass() != dns.ClassINET || state.QType() != dns.TypeA {
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

	qname := state.QName()
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

	w.WriteMsg(m)
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

// clientName returns the best available name for a client,
// preferring Name (UI alias) over Hostname (DHCP-reported).
func clientName(name, hostname string) string {
	if name != "" {
		return name
	}
	return hostname
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

	keepClients := map[string]bool{}
	for _, client := range clients {
		name := clientName(client.Name, client.Hostname)
		if name == "" {
			continue
		}

		domain := domains[client.NetworkID]
		if domain == "" {
			continue
		}

		record := name + "." + domain

		u.mappings[record] = &UnifiConfigEntry{
			a:   net.ParseIP(client.IP),
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
