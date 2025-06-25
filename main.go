package main

import (
	"flag"
	"sync"
	"time"

	"github.com/valyala/fastjson"
)

var mu sync.Mutex
var blockCache = make(map[string]*BlockCache)

func startGoroutines(servers *Servers, rp_port int) {
	var wg sync.WaitGroup

	go func() {
		reverseProxy(rp_port, servers)
	}()

	servers.updateServersData(&wg)
	servers.timer_UpdateServersData(&wg)
}

func (servers *Servers) timer_UpdateServersData(wg *sync.WaitGroup) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop() // Stop the ticker when the function returns

	for range ticker.C {
		servers.updateServersData(wg)
	}
}

func init() {
	initLogger()
	initConfig()
}

func main() {
	defer logFile.Close()

	// Define a command-line flag for the launch argument
	dynlist_bool := flag.Bool("dynlist", false, "Set to true to use dynamic server list & update routine")
	flag.Parse()

	// Create a new instance of Servers
	servers := Servers{
		g_getfees:         fastjson.MustParse(`{"result": null, "error": null}`),
		g_getheights:      fastjson.MustParse(`{"result": null, "error": null}`),
		g_coinsServersIDs: fastjson.MustParse(`{}`),
	}

	if *dynlist_bool {
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
