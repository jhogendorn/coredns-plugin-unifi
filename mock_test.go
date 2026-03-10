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

func (m *mockUnifiAPI) GetClients(sites []*unpoller_unifi.Site) ([]*unpoller_unifi.Client, error) {
	return m.clients, m.clientsErr
}

func (m *mockUnifiAPI) GetNetworks(sites []*unpoller_unifi.Site) ([]unpoller_unifi.Network, error) {
	return m.networks, m.networksErr
}
