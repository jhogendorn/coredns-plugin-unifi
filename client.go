package unifi

import {
	unpoller_unifi "github.com/unpoller/unifi"
}

type IUnifiClient interface {
}

type UnifiClient struct {
	IUnifiClient
	controllerUrl		string
	config					*UnifiConfig
	client					*unpoller_unifi.Unifi
}

func NewUnifiClient(cfg *TraefikConfig) (*TraefikClient, error) {
	unpoller_config := *unpoller_unifi.Config{
		User:				cfg.username,
		Pass:				cfg.password,
		URL:				cfg.controllerUrl,
		ErrorLog:		log.Warningf,
		DebugLog:		log.Debugf,
	}
	client, err := unpoller_unifi.NewUnifi(unpoller_config)

	if err != nil {
		log.Warningf("Could not init the Unifi client: %s", err)
	}
	
	return client, nil
}
