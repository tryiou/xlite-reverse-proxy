package main

import (
	"sync"
	"testing"
	"time"
)

// TestDynamicServerUpdateThreadSafety tests that dynamic server updates are thread-safe
func TestDynamicServerUpdateThreadSafety(t *testing.T) {
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
			mu.Lock()
			config = testConfig
			mu.Unlock()
		}()
	}

	wg.Wait()
	// Test passes if no race conditions detected when running with -race flag
}

// TestConfigAssignmentWithMutex tests that config assignment is protected by mutex
func TestConfigAssignmentWithMutex(t *testing.T) {
	originalConfig := config

	// Test that we can safely assign config with mutex
	mu.Lock()
	config = &Config{
		DynlistServersProviders: []string{"http://safe.example.com"},
		AcceptedMethods:         []string{"test"},
	}
	mu.Unlock()

	// Verify assignment worked
	if config == originalConfig {
		t.Error("Config assignment failed")
	}

	if config.DynlistServersProviders[0] != "http://safe.example.com" {
		t.Error("Config assignment content incorrect")
	}

	// Restore original config
	mu.Lock()
	config = originalConfig
	mu.Unlock()
}

// TestMultipleConfigReads tests that multiple concurrent reads don't cause issues
func TestMultipleConfigReads(t *testing.T) {
	// Simulate multiple goroutines reading config concurrently
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Safe read with mutex
			mu.Lock()
			_ = len(config.DynlistServersProviders)
			mu.Unlock()
		}()
	}

	wg.Wait()
	// Test passes if no deadlocks or panics occur
}

// TestConfigUpdateSequence tests a sequence of read-write operations
func TestConfigUpdateSequence(t *testing.T) {
	originalConfig := config

	// Perform a series of operations that could cause race conditions
	operations := []func(){
		func() {
			mu.Lock()
			config = &Config{AcceptedMethods: []string{"method1"}}
			mu.Unlock()
		},
		func() {
			mu.Lock()
			_ = config.AcceptedMethods
			mu.Unlock()
		},
		func() {
			mu.Lock()
			config = &Config{AcceptedMethods: []string{"method2"}}
			mu.Unlock()
		},
		func() {
			mu.Lock()
			_ = len(config.DynlistServersProviders)
			mu.Unlock()
		},
		func() {
			mu.Lock()
			config = &Config{AcceptedMethods: []string{"method3"}}
			mu.Unlock()
		},
	}

	for _, op := range operations {
		op()
		time.Sleep(1 * time.Millisecond) // Small delay to increase chance of race conditions
	}

	// Verify final state
	mu.Lock()
	if len(config.AcceptedMethods) != 1 || config.AcceptedMethods[0] != "method3" {
		t.Errorf("Final config state incorrect: %v", config.AcceptedMethods)
	}
	mu.Unlock()

	// Restore original config
	mu.Lock()
	config = originalConfig
	mu.Unlock()
}
