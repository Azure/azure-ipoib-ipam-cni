// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Azure/azure-ipoib-ipam-cni/pkg/webhook"
)

func main() {
	var (
		bindAddress string
		kvpPath     string
		profile     string
	)

	flag.StringVar(&bindAddress, "bind-address", ":8080",
		"Address for the webhook server to listen on. Either a TCP address "+
			"(e.g. \":8080\", \"127.0.0.1:8080\") or a Unix socket path prefixed "+
			"with \"unix://\" (e.g. \"unix:///var/run/dranet/webhook.sock\").")
	flag.StringVar(&kvpPath, "kvp-path", webhook.DefaultKVPStorePath,
		"Path to the HyperV KVP pool file holding the IPoIB address mapping.")
	flag.StringVar(&profile, "profile", "",
		"If set, only answer GetProfileConfig requests whose NetworkConfig.profile "+
			"matches this value. If empty, all profiles are accepted.")
	flag.Parse()

	server := webhook.NewServer(kvpPath, profile)

	// Read the KVP store once at startup. The store is only re-read on SIGHUP
	// (or a process restart), so a missing/unreadable file at startup is fatal.
	if err := server.Load(); err != nil {
		log.Fatalf("failed to load KVP store %q: %v", kvpPath, err)
	}

	// Reload the KVP store on SIGHUP without restarting the process.
	reloadCh := make(chan os.Signal, 1)
	signal.Notify(reloadCh, syscall.SIGHUP)
	go func() {
		for range reloadCh {
			if err := server.Reload(); err != nil {
				log.Printf("failed to reload KVP store %q: %v (keeping previous content)", kvpPath, err)
				continue
			}
			log.Printf("reloaded KVP store %q on SIGHUP", kvpPath)
		}
	}()

	listener, err := listen(bindAddress)
	if err != nil {
		log.Fatalf("failed to listen on %q: %v", bindAddress, err)
	}

	log.Printf("starting azure-ipoib IPoIB webhook provider on %q (kvp-path=%q, profile=%q); send SIGHUP to reload the KVP store",
		bindAddress, kvpPath, profile)
	if err := http.Serve(listener, server.Handler()); err != nil {
		log.Fatalf("webhook server failed: %v", err)
	}
}

// listen creates a listener for either a TCP address or a "unix://" socket path.
func listen(bindAddress string) (net.Listener, error) {
	if socketPath, ok := strings.CutPrefix(bindAddress, "unix://"); ok {
		// Remove a stale socket left behind by a previous run so bind succeeds.
		if _, err := os.Stat(socketPath); err == nil {
			if err := os.Remove(socketPath); err != nil {
				return nil, err
			}
		}
		return net.Listen("unix", socketPath)
	}
	return net.Listen("tcp", bindAddress)
}
