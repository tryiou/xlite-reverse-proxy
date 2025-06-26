package main

import (
	"flag"
	"log"
	"sync"
	"time"
)

var mu sync.Mutex
var blockCache = make(map[string]*BlockCache)

var config *Config

func startGoroutines(servers *Servers, rp_port int) {
	var wg sync.WaitGroup

	go func() {
		reverseProxy(rp_port, servers)
	}()

	servers.UpdateAllServersData(&wg)
	servers.timer_UpdateAllServersData(&wg)
}

func (servers *Servers) timer_UpdateAllServersData(wg *sync.WaitGroup) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop() // Stop the ticker when the function returns

	for range ticker.C {
		servers.UpdateAllServersData(wg)
	}
}

func init() {
	initLogger()
}

func main() {
	defer logFile.Close()

	// Define a command-line flag for the launch argument
	dynlist := flag.Bool("dynlist", false, "Set to true to use dynamic server list & update routine")
	configFile := flag.String("config", "xlite-reverse-proxy-config.yaml", "Path to the configuration file")
	flag.Parse()

	var err error
	config, err = newConfig(*configFile)
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

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

	go startGoroutines(&servers, 11111)

	// Keep the main goroutine running
	select {}
}
