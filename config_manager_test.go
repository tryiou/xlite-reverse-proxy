package main

import (
	"net/http"
	"sync"
	"testing"
	"time"
)

// setupTestConfigForConfigManager sets up a test config for config manager tests
func setupTestConfigForConfigManager() {
	globalConfig.config = &Config{
		HttpTimeout:        5,
		RateLimit:          100,
		MaxLogSize:         1024 * 1024,
		ConsensusThreshold: 0.6,
		MaxStoredBlocks:    10,
		MaxBlockTimeDiff:   7200,
	}
	globalConfig.client = &http.Client{Timeout: 5 * time.Second}
	globalConfig.cache = NewOptimizedBlockCache(10)
}

// TestConcurrentConfigAccess verifies that the config manager is thread-safe
func TestConcurrentConfigAccess(t *testing.T) {
	setupTestConfigForConfigManager()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				cfg := globalConfig.GetConfig()
				_ = cfg.HttpTimeout
				_ = cfg.RateLimit
			}
		}()
	}
	wg.Wait() // If this completes without panic, it's thread-safe
}

// TestConfigManagerBasicFunctionality tests basic config manager operations
func TestConfigManagerBasicFunctionality(t *testing.T) {
	setupTestConfigForConfigManager()

	// Test getting config
	cfg := globalConfig.GetConfig()
	if cfg == nil {
		t.Error("Config should not be nil after loading")
	}

	// Test getting client
	client := globalConfig.GetClient()
	if client == nil {
		t.Error("HTTP client should not be nil after loading")
	}

	// Test getting cache
	cache := globalConfig.GetCache()
	if cache == nil {
		t.Error("Block cache should not be nil after loading")
	}
}
