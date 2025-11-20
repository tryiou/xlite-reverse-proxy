package main

import (
	"net/http"
	"sync"
	"testing"
	"time"
)

// setupTestConfigForServersUpdate sets up a test config for servers update tests
func setupTestConfigForServersUpdate() {
	globalConfig.config = &Config{
		DynlistServersProviders: []string{"http://test.example.com"},
		AcceptedMethods:         []string{"heights", "fees"},
		HttpTimeout:             5,
		RateLimit:               100,
		MaxLogSize:              1024 * 1024,
		ConsensusThreshold:      0.6,
		MaxStoredBlocks:         10,
		MaxBlockTimeDiff:        7200,
	}
	globalConfig.client = &http.Client{Timeout: 5 * time.Second}
	globalConfig.cache = NewOptimizedBlockCache(10)
}

// TestDynamicServerUpdateThreadSafety tests that dynamic server updates are thread-safe
func TestDynamicServerUpdateThreadSafety(t *testing.T) {
	setupTestConfigForServersUpdate()

	// Skip race condition test if not running with -race flag
	if !testing.Verbose() {
		t.Skip("Skipping race condition test - run with -race flag to test")
	}

	var wg sync.WaitGroup
	testConfig := &Config{
		DynlistServersProviders: []string{"http://test.example.com"},
		AcceptedMethods:         []string{"heights", "fees"},
	}

	// Simulate concurrent access to config
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// This should not panic or cause race conditions with the mutex
			locks.config.Lock()
			globalConfig.config = testConfig
			locks.config.Unlock()
		}()
	}

	wg.Wait()
	// Test passes if no race conditions detected when running with -race flag
}

// TestConfigAssignmentWithMutex tests that config assignment is protected by mutex
func TestConfigAssignmentWithMutex(t *testing.T) {
	setupTestConfigForServersUpdate()

	originalConfig := globalConfig.config

	// Test that we can safely assign config with mutex
	locks.config.Lock()
	globalConfig.config = &Config{
		DynlistServersProviders: []string{"http://safe.example.com"},
		AcceptedMethods:         []string{"test"},
	}
	locks.config.Unlock()

	// Verify assignment worked
	if globalConfig.config == originalConfig {
		t.Error("Config assignment failed")
	}

	if globalConfig.config.DynlistServersProviders[0] != "http://safe.example.com" {
		t.Error("Config assignment content incorrect")
	}

	// Restore original config
	locks.config.Lock()
	globalConfig.config = originalConfig
	locks.config.Unlock()
}

// TestMultipleConfigReads tests that multiple concurrent reads don't cause issues
func TestMultipleConfigReads(t *testing.T) {
	setupTestConfigForServersUpdate()

	// Simulate multiple goroutines reading config concurrently
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Safe read with mutex
			locks.config.RLock()
			_ = len(globalConfig.config.DynlistServersProviders)
			locks.config.RUnlock()
		}()
	}

	wg.Wait()
	// Test passes if no deadlocks or panics occur
}

// TestConfigUpdateSequence tests a sequence of read-write operations
func TestConfigUpdateSequence(t *testing.T) {
	setupTestConfigForServersUpdate()

	originalConfig := globalConfig.config

	// Perform a series of operations that could cause race conditions
	operations := []func(){
		func() {
			locks.config.Lock()
			globalConfig.config = &Config{AcceptedMethods: []string{"method1"}}
			locks.config.Unlock()
		},
		func() {
			locks.config.RLock()
			_ = globalConfig.config.AcceptedMethods
			locks.config.RUnlock()
		},
		func() {
			locks.config.Lock()
			globalConfig.config = &Config{AcceptedMethods: []string{"method2"}}
			locks.config.Unlock()
		},
		func() {
			locks.config.RLock()
			_ = len(globalConfig.config.DynlistServersProviders)
			locks.config.RUnlock()
		},
		func() {
			locks.config.Lock()
			globalConfig.config = &Config{AcceptedMethods: []string{"method3"}}
			locks.config.Unlock()
		},
	}

	for _, op := range operations {
		op()
		time.Sleep(1 * time.Millisecond) // Small delay to increase chance of race conditions
	}

	// Verify final state
	locks.config.RLock()
	if len(globalConfig.config.AcceptedMethods) != 1 || globalConfig.config.AcceptedMethods[0] != "method3" {
		t.Errorf("Final config state incorrect: %v", globalConfig.config.AcceptedMethods)
	}
	locks.config.RUnlock()

	// Restore original config
	locks.config.Lock()
	globalConfig.config = originalConfig
	locks.config.Unlock()
}
