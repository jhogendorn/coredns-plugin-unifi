// Package example is a CoreDNS plugin that prints "example" to stdout on every packet received.
//
// It serves as an example CoreDNS plugin with numerous code comments.
package unifi

import (
	"context"
	"net"
	"net/url"
	"strings"
	"maps"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/plugin/pkg/fall"
	clog "github.com/coredns/coredns/plugin/pkg/log"

	"github.com/miekg/dns"
)

var log = clog.NewWithPlugin("unifi")

type UnifiConfigEntry struct {
	a				*[]net.IP
	ttl			uint32
}

type UnifiConfigEntryMap map[string]*UnifiConfigEntry

type UnifiConfig struct {
	controllerUrl		*url.URL
	username				*string
	password				*string
	ttl							uint32
	refreshInterval uint32
}

type Unifi struct {
	Next						plugin.Handler
	Config					*UnifiConfig
	Client					*UnifiClient

	mappings				UnifiConfigEntryMap
	ready						bool
	mutex						sync.RWMutex
	fall						fall.F
}

func (u *Unifi) Name() string { return "unifi" }


func (u *Unifi) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {

	state := request.Request{W: w, Req: r}

	if state.QClass() != dns.ClassINET || state.QType() != dns.TypeA {
		return plugin.NextOrFailure(u.Name(), u.Next, ctx, w, r)
	}

	requestCount.WithLabelValues(metrics.WithServer(ctx)).Inc()

	qname := state.QName()
	answers := []dns.RR{}
	for _, q:= range state.Req.Question {
			find := strings.ToLower(q.Name[:len(q.Name)-1])

			// @TODO should this do any kind of filtering against the zone etc?
			result := u.getEntry(find)
			if result != nil {
				r := new(dns.A)
				r.Hdr = dns.RR_Header{Name: qname, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: result.ttl}
				r.A = result.a
				append(answers, r)
			}
	}

	m := new(dns.Msg)
	if len(answers) == 0 {
		if u.Fall.Through(qname) && u.Next != nil {
			log.Debug("Falling through. 0 answers")
			return plugin.NextOrFailure(u.Name(), u.Next, ctx, w, r)
		}

		log.Debug("Returning NXDOMAIN")
		m.Rcode = dns.RcodeNameError
	}

	m.setReply(r)
	m.Authoritative = true
	m.Answer = answers

	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

func (u *Unifi) start() error {
	log.Info("Starting Unifi Query")
	err := u.refresh(true)

	if err != nil {
		log.Warningf("Failed to load clients from Unifi Controller, will retry: %s", err)
	}

	uptimeTicker := time.NewTicker(time.Duration(t.Config.refreshInterval) * time.Second)

	for {
		select {
			case <-uptimeTicker.C:
				log.Debug("Refreshing from Unifi Controller")
				err := t.refresh(false)
				if err != nil {
					log.Warningf("Error loading Unifi Clients: %s", err)
				}
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

func (u *Unifi) refresh(first bool) error {
	if first {
		log.Infof("Querying the Unifi Controller")
	}

	u.mutex.Lock()
	defer u.mutex.Unlock()

	sites, err := u.Client.GetSites()
	if err != nil {
		log.Warningf("Could not retrieve Sites: %s", err)
	}

	//@TODO some way to filter/limit the list of sites.

	clients, err := u.Client.getClients(&sites)
	if err != nil {
		log.Warningf("Could not retrieve Clients: %s", err)
	}

	networks, err := u.Client.getNetworks(&sites)
	if err != nil {
		log.Warningf("Could not retrieve Networks: %s", err)
	}
	
	domains := map[string]string{}
	for _, network := range *networks {
		domains[network.ID] = network.DomainName
	}

	// @NOTE either of these, or the delete below.
	//maps.Clear(*u.mappings)
	//u.mappings = make(UnifiConfigEntryMap)

	keepClients := []string{}
	for _, client := range *clients {
		// client.Name
		// client.Hostname
		// @NOTE I think unifi copies hostname to name unless you
		//				have an override alias
		// client.IP
		// client.FixedIP // Maybe a filter for only fixed ips?
		// client.Network
		// client.NetworkID
		// @TODO do we need to do lookup on the network to
		//       determine what the network dhcp dns name is?

		record := client.Name + "." + domains[client.NetworkID]

		*u.mappings[record] = &UnifiConfigEntry{
			a: client.IP
			ttl: u.Config.ttl
		}
		append(keepClients, record)

	}

	// @TODO delete old mappings out
	maps.DeleteFunc(*u.mappings, func(key string, value *UnifiConfigEntry) bool {
			_, shouldKeep := keepClients[key]
			return !shouldKeep
	})

	// @TODO Should we interrogate devices?
}
