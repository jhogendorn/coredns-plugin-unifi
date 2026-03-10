package unifi

import (
	unpoller_unifi "github.com/unpoller/unifi"
)

// UnifiAPI defines the methods we use from the unpoller client,
// allowing us to mock the controller in tests.
type UnifiAPI interface {
	GetSites() ([]*unpoller_unifi.Site, error)
	GetClients(sites []*unpoller_unifi.Site) ([]*unpoller_unifi.Client, error)
	GetNetworks(sites []*unpoller_unifi.Site) ([]unpoller_unifi.Network, error)
}

type UnifiClient struct {
	controllerUrl string
	config        *UnifiConfig
	api           UnifiAPI
}

func NewUnifiClient(cfg *UnifiConfig) (*UnifiClient, error) {
	unpoller_config := &unpoller_unifi.Config{
		User:     cfg.username,
		Pass:     cfg.password,
		URL:      cfg.controllerUrl,
		ErrorLog: log.Warningf,
		DebugLog: log.Debugf,
	}
	client, err := unpoller_unifi.NewUnifi(unpoller_config)
	if err != nil {
		return nil, err
	}

	return &UnifiClient{
		controllerUrl: cfg.controllerUrl,
		config:        cfg,
		api:           client,
	}, nil
}
