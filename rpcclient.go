package main

import (
	"fmt"
	"io/ioutil"

	"github.com/decred/dcrrpcclient"
)

func connectWalletRPC(cfg *config) (*dcrrpcclient.Client, error) {
	var dcrwCerts []byte
	var err error
	if !cfg.DisableClientTLS {
		dcrwCerts, err = ioutil.ReadFile(cfg.DcrwCert)
		if err != nil {
			log.Errorf("Failed to read dcrwallet cert file at %s: %s\n",
				cfg.DcrwCert, err.Error())
			return nil, err
		}
	}

	log.Debugf("Attempting to connect to dcrwallet RPC %s as user %s "+
		"using certificate located in %s",
		cfg.DcrwServ, cfg.DcrwUser, cfg.DcrwCert)

	connCfgWallet := &dcrrpcclient.ConnConfig{
		Host:         cfg.DcrwServ,
		Endpoint:     "ws",
		User:         cfg.DcrwUser,
		Pass:         cfg.DcrwPass,
		Certificates: dcrwCerts,
		DisableTLS:   cfg.DisableClientTLS,
	}

	ntfnHandlers := getWalletNtfnHandlers(cfg)
	dcrwClient, err := dcrrpcclient.New(connCfgWallet, ntfnHandlers)
	if err != nil {
		fmt.Printf("Failed to start dcrwallet RPC client: %s\nPerhaps you"+
			" wanted to start with --nostakeinfo?\n", err.Error())
		fmt.Printf("Verify that rpc.cert is for your wallet:\n\t%v",
			cfg.DcrwCert)
		return nil, err
	}
	return dcrwClient, nil
}

func connectNodeRPC(cfg *config) (*dcrrpcclient.Client, error) {
	var dcrdCerts []byte
	var err error
	if !cfg.DisableClientTLS {
		dcrdCerts, err = ioutil.ReadFile(cfg.DcrdCert)
		if err != nil {
			fmt.Printf("Failed to read dcrd cert file at %s: %s\n",
				cfg.DcrdCert, err.Error())
			return nil, err
		}
	}

	log.Debugf("Attempting to connect to dcrd RPC %s as user %s "+
		"using certificate located in %s",
		cfg.DcrdServ, cfg.DcrdUser, cfg.DcrdCert)

	connCfgDaemon := &dcrrpcclient.ConnConfig{
		Host:         cfg.DcrdServ,
		Endpoint:     "ws", // websocket
		User:         cfg.DcrdUser,
		Pass:         cfg.DcrdPass,
		Certificates: dcrdCerts,
		DisableTLS:   cfg.DisableClientTLS,
	}

	ntfnHandlers := getNodeNtfnHandlers(cfg)
	dcrdClient, err := dcrrpcclient.New(connCfgDaemon, ntfnHandlers)
	if err != nil {
		fmt.Printf("Failed to start dcrd RPC client: %s\n", err.Error())
		return nil, err
	}
	return dcrdClient, nil
}
