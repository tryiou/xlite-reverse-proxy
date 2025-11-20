package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// Application represents the main application with centralized state management
type Application struct {
	configManager *ConfigManager
	servers       *Servers
	logger        *log.Logger
	wg            *sync.WaitGroup
	shutdown      chan struct{}
}

// NewApplication creates a new Application instance
func NewApplication() *Application {
	return &Application{
		configManager: &globalConfig,
		wg:           &sync.WaitGroup{},
		shutdown:     make(chan struct{}),
	}
}

// Initialize sets up the application with the given configuration file
func (a *Application) Initialize(configFile string) error {
	if a.configManager == nil {
		return fmt.Errorf("config manager not initialized")
	}
	
	if err := a.configManager.Load(configFile); err != nil {
		return err
	}

	a.initializeServers()
	a.initializeLogger()
	return nil
}

// initializeServers sets up the servers with default responses
func (a *Application) initializeServers() {
	a.servers = &Servers{
		GlobalFees:          getDefaultJSONResponse(),
		GlobalHeights:       getDefaultJSONResponse(),
		GlobalCoinServerIDs: getEmptyJSONResponse(),
	}
}

// initializeLogger sets up the logger
func (a *Application) initializeLogger() {
	a.logger = logger
}

// Start starts the application
func (a *Application) Start(dynlist bool, port int) error {
	if dynlist {
		// Dynamic servers list from Dynlist_servers_providers and goroutine updating it every 5 min, remove/add servers on the fly
		a.startServerUpdateRoutine()
	} else {
		// Static servers list
		a.UpdateServersFromJSON()
	}

	// Start goroutines with improved error handling
	if err := a.startGoroutines(port); err != nil {
		return err
	}

	return nil
}

// startGoroutines starts the main application goroutines
func (a *Application) startGoroutines(port int) error {
	var wg sync.WaitGroup

	// Start reverse proxy goroutine with error handling
	reverseProxyErr := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				reverseProxyErr <- fmt.Errorf("reverse proxy panic: %v", r)
			}
		}()
		reverseProxy(port, a.servers)
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
		a.servers.UpdateAllServersData(&wg)
		a.timer_UpdateAllServersData(&wg)
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

// timer_UpdateAllServersData runs periodic server updates
func (a *Application) timer_UpdateAllServersData(wg *sync.WaitGroup) {
	ticker := time.NewTicker(UpdateIntervalDefault)
	defer ticker.Stop()

	for range ticker.C {
		var wgUpdate sync.WaitGroup
		a.servers.UpdateAllServersData(&wgUpdate)
		wgUpdate.Wait()
	}
}

// startServerUpdateRoutine starts the dynamic server update routine
func (a *Application) startServerUpdateRoutine() {
	startServerUpdateRoutine(a.servers)
}

// UpdateServersFromJSON updates servers from JSON configuration
func (a *Application) UpdateServersFromJSON() {
	UpdateServersFromJSON(a.servers)
}

// GetConfig returns the current configuration
func (a *Application) GetConfig() *Config {
	if a.configManager == nil {
		return nil
	}
	return a.configManager.GetConfig()
}

// GetClient returns the HTTP client
func (a *Application) GetClient() *http.Client {
	if a.configManager == nil {
		return nil
	}
	return a.configManager.GetClient()
}

// GetCache returns the block cache
func (a *Application) GetCache() *OptimizedBlockCache {
	if a.configManager == nil {
		return nil
	}
	return a.configManager.GetCache()
}
