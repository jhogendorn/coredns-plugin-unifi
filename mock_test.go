package unifi

import (
	unpoller_unifi "github.com/unpoller/unifi"
)

type mockUnifiAPI struct {
	sites    []*unpoller_unifi.Site
	clients  []*unpoller_unifi.Client
	networks []unpoller_unifi.Network

	sitesErr    error
	clientsErr  error
	networksErr error
}

func (m *mockUnifiAPI) GetSites() ([]*unpoller_unifi.Site, error) {
	return m.sites, m.sitesErr
}

// GetClients filters clients by matching client.SiteID against site.Name for
// each site in the provided list. Clients with an empty SiteID are always
// included so that existing tests that don't set SiteID continue to work.
func (m *mockUnifiAPI) GetClients(sites []*unpoller_unifi.Site) ([]*unpoller_unifi.Client, error) {
	if m.clientsErr != nil {
		return nil, m.clientsErr
	}

	// Build a set of allowed site names from the provided sites slice.
	allowed := make(map[string]bool, len(sites))
	for _, s := range sites {
		allowed[s.Name] = true
	}

	var result []*unpoller_unifi.Client
	for _, c := range m.clients {
		// Clients with no SiteID are included unconditionally (backward compat).
		if c.SiteID == "" || allowed[c.SiteID] {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *mockUnifiAPI) GetNetworks(sites []*unpoller_unifi.Site) ([]unpoller_unifi.Network, error) {
	return m.networks, m.networksErr
}
