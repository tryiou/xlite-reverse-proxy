package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

var mu sync.Mutex

var config *Config

// Global HTTP client with connection pooling
var httpClient *http.Client

func startGoroutines(servers *Servers, rp_port int) error {
	var wg sync.WaitGroup

	// Start reverse proxy goroutine with error handling
	reverseProxyErr := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				reverseProxyErr <- fmt.Errorf("reverse proxy panic: %v", r)
			}
		}()
		reverseProxy(rp_port, servers)
		reverseProxyErr <- nil
	}()

	// Start server update routines with error handling
	updateErr := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				updateErr <- fmt.Errorf("server update panic: %v", r)
			}
		}()
		servers.UpdateAllServersData(&wg)
		servers.timer_UpdateAllServersData(&wg)
		updateErr <- nil
	}()

	// Check for immediate startup errors
	select {
	case rpErr := <-reverseProxyErr:
		if rpErr != nil {
			return fmt.Errorf("failed to start reverse proxy: %w", rpErr)
		}
	case updErr := <-updateErr:
		if updErr != nil {
			return fmt.Errorf("failed to start server updates: %w", updErr)
		}
	default:
		// No immediate errors, continue normally
	}

	return nil
}

func (servers *Servers) timer_UpdateAllServersData(wg *sync.WaitGroup) {
	ticker := time.NewTicker(UpdateIntervalDefault)
	defer ticker.Stop()

	for range ticker.C {
		var wgUpdate sync.WaitGroup
		servers.UpdateAllServersData(&wgUpdate)
		wgUpdate.Wait()
	}
}

func init() {
	initLogger()
}

func main() {
	defer logFile.Close()

	dynlist := flag.Bool("dynlist", false, "Set to true to use dynamic server list & update routine")
	configFile := flag.String("config", "xlite-reverse-proxy-config.yaml", "Path to the configuration file")
	flag.Parse()

	var err error
	config, err = newConfig(*configFile)
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	// Initialize HTTP client with config values and validate configuration
	if err := initHTTPClient(); err != nil {
		log.Fatalf("Failed to initialize HTTP client: %v", err)
	}

	// Initialize optimized block cache with config value after config is loaded
	optimizedBlockCache = NewOptimizedBlockCache(config.MaxStoredBlocks)

	// Create a new instance of Servers
	servers := Servers{
		GlobalFees:          getDefaultJSONResponse(),
		GlobalHeights:       getDefaultJSONResponse(),
		GlobalCoinServerIDs: getEmptyJSONResponse(),
	}

	if *dynlist {
		// Dynamic servers list from Dynlist_servers_providers and goroutine updating it every 5 min, remove/add servers on the fly
		startServerUpdateRoutine(&servers)
	} else {
		// Static servers list
		UpdateServersFromJSON(&servers)
	}

	// Start goroutines with improved error handling
	if err := startGoroutines(&servers, DefaultPort); err != nil {
		log.Fatalf("Failed to start goroutines: %v", err)
	}

	// Keep the main goroutine running
	select {}
}
