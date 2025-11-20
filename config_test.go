package main

import (
	"strings"
	"testing"
)

// TestValidateURLFormat tests the enhanced URL validation function
func TestValidateURLFormat(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid https", "https://example.com", false},
		{"valid http", "http://example.com:8080", false},
		{"valid with path", "https://api.example.com/v1/endpoint", false},
		{"empty url", "", true},
		{"missing scheme", "example.com", true},
		{"missing host", "https://", true},
		{"invalid format", "not-a-url", true},
		{"missing scheme only", ":example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURLFormat(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateURLFormat() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateNumericRange tests the numeric range validation function
func TestValidateNumericRange(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		min     int
		max     int
		wantErr bool
	}{
		{"valid value in range", 50, 1, 100, false},
		{"value at minimum", 1, 1, 100, false},
		{"value at maximum", 100, 1, 100, false},
		{"value below minimum", 0, 1, 100, true},
		{"value above maximum", 101, 1, 100, true},
		{"negative value", -5, 1, 100, true},
		{"zero value", 0, 1, 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNumericRange("test value", tt.value, tt.min, tt.max)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateNumericRange() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateNumericRangeErrorMessage tests that error messages are properly formatted
func TestValidateNumericRangeErrorMessage(t *testing.T) {
	// Test below minimum
	err := validateNumericRange("HTTP timeout", 0, 1, 300)
	if err == nil {
		t.Error("Expected error for value below minimum")
	}
	if !strings.Contains(err.Error(), "HTTP timeout too small: 0 (minimum 1)") {
		t.Errorf("Expected specific error message, got: %v", err)
	}

	// Test above maximum
	err = validateNumericRange("rate limit", 1001, 1, 1000)
	if err == nil {
		t.Error("Expected error for value above maximum")
	}
	if !strings.Contains(err.Error(), "rate limit too large: 1001 (maximum 1000)") {
		t.Errorf("Expected specific error message, got: %v", err)
	}
}

// TestConfigValidationWithEnhancedURLValidation tests configuration validation with the new URL validation
func TestConfigValidationWithEnhancedURLValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config with proper URLs",
			config: &Config{
				ServersMap: []ServerConfig{
					{URL: "https://api.example.com", EXR: true},
					{URL: "http://localhost:8080", EXR: false},
				},
				DynlistServersProviders: []string{
					"https://provider.example.com",
					"http://provider2.example.com:3000",
				},
				AcceptedPaths:      []string{"/test"},
				AcceptedMethods:    []string{"test"},
				HttpTimeout:        30,
				RateLimit:          100,
				MaxLogSize:         1024 * 1024,
				ConsensusThreshold: 0.6,
				MaxStoredBlocks:    10,
				MaxBlockTimeDiff:   7200,
			},
			wantErr: false,
		},
		{
			name: "invalid server URL - missing scheme",
			config: &Config{
				ServersMap: []ServerConfig{
					{URL: "example.com", EXR: true},
				},
				AcceptedPaths:   []string{"/test"},
				AcceptedMethods: []string{"test"},
				HttpTimeout:     30,
			},
			wantErr: true,
		},
		{
			name: "invalid server URL - missing host",
			config: &Config{
				ServersMap: []ServerConfig{
					{URL: "https://", EXR: true},
				},
				AcceptedPaths:   []string{"/test"},
				AcceptedMethods: []string{"test"},
				HttpTimeout:     30,
			},
			wantErr: true,
		},
		{
			name: "invalid provider URL",
			config: &Config{
				DynlistServersProviders: []string{
					"not-a-url",
				},
				AcceptedPaths:   []string{"/test"},
				AcceptedMethods: []string{"test"},
				HttpTimeout:     30,
			},
			wantErr: true,
		},
		{
			name: "valid config with various URL formats",
			config: &Config{
				ServersMap: []ServerConfig{
					{URL: "https://secure.example.com", EXR: true},
					{URL: "http://localhost:8080", EXR: false},
					{URL: "https://api.example.com/v1", EXR: true},
					{URL: "http://192.168.1.1:3000", EXR: false},
				},
				DynlistServersProviders: []string{
					"https://provider1.example.com",
					"https://provider2.example.com:443",
					"http://localhost:8080",
				},
				AcceptedPaths:      []string{"/test"},
				AcceptedMethods:    []string{"test"},
				HttpTimeout:        30,
				RateLimit:          100,
				MaxLogSize:         1024 * 1024,
				ConsensusThreshold: 0.6,
				MaxStoredBlocks:    10,
				MaxBlockTimeDiff:   7200,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestMaxLogSizeUpperBoundValidation tests the maximum log size validation
func TestMaxLogSizeUpperBoundValidation(t *testing.T) {
	tests := []struct {
		name       string
		maxLogSize int
		wantErr    bool
	}{
		{"valid max log size - 50MB", 50 * 1024 * 1024, false},
		{"valid max log size - 100MB", 100 * 1024 * 1024, false},
		{"invalid max log size - 101MB", 101 * 1024 * 1024, true},
		{"invalid max log size - 200MB", 200 * 1024 * 1024, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				ServersMap:         []ServerConfig{{URL: "http://example.com", EXR: true}},
				AcceptedPaths:      []string{"/test"},
				AcceptedMethods:    []string{"test"},
				HttpTimeout:        30,
				MaxLogSize:         tt.maxLogSize,
				ConsensusThreshold: 0.6,
				MaxStoredBlocks:    10,
				MaxBlockTimeDiff:   7200,
				RateLimit:          100, // Add required RateLimit field
			}
			err := validateConfig(config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestMaxBlockTimeDiffUpperBoundValidation tests the maximum block time difference validation
func TestMaxBlockTimeDiffUpperBoundValidation(t *testing.T) {
	tests := []struct {
		name             string
		maxBlockTimeDiff int
		wantErr          bool
	}{
		{"valid max block time diff - 7200s", 7200, false},
		{"valid max block time diff - 86400s", 86400, false},
		{"invalid max block time diff - 86401s", 86401, true},
		{"invalid max block time diff - 172800s", 172800, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				ServersMap:         []ServerConfig{{URL: "http://example.com", EXR: true}},
				AcceptedPaths:      []string{"/test"},
				AcceptedMethods:    []string{"test"},
				HttpTimeout:        30,
				MaxLogSize:         1024 * 1024,
				ConsensusThreshold: 0.6,
				MaxStoredBlocks:    10,
				MaxBlockTimeDiff:   tt.maxBlockTimeDiff,
				RateLimit:          100, // Add required RateLimit field
			}
			err := validateConfig(config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestConfigValidationEdgeCases tests edge cases and boundary conditions
func TestConfigValidationEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config at HTTP timeout boundaries",
			config: &Config{
				ServersMap:         []ServerConfig{{URL: "http://example.com", EXR: true}},
				AcceptedPaths:      []string{"/test"},
				AcceptedMethods:    []string{"test"},
				HttpTimeout:        1,           // minimum
				RateLimit:          1,           // minimum
				MaxLogSize:         1024 * 1024, // minimum
				ConsensusThreshold: 0.6,
				MaxStoredBlocks:    1, // minimum
				MaxBlockTimeDiff:   1, // minimum
			},
			wantErr: false,
		},
		{
			name: "valid config at maximum boundaries",
			config: &Config{
				ServersMap:         []ServerConfig{{URL: "http://example.com", EXR: true}},
				AcceptedPaths:      []string{"/test"},
				AcceptedMethods:    []string{"test"},
				HttpTimeout:        300,               // maximum
				RateLimit:          1000,              // maximum
				MaxLogSize:         100 * 1024 * 1024, // maximum
				ConsensusThreshold: 0.6,
				MaxStoredBlocks:    100,   // maximum
				MaxBlockTimeDiff:   86400, // maximum
			},
			wantErr: false,
		},
		{
			name: "invalid config - HTTP timeout too low",
			config: &Config{
				ServersMap:         []ServerConfig{{URL: "http://example.com", EXR: true}},
				AcceptedPaths:      []string{"/test"},
				AcceptedMethods:    []string{"test"},
				HttpTimeout:        0, // too low
				RateLimit:          100,
				MaxLogSize:         1024 * 1024,
				ConsensusThreshold: 0.6,
				MaxStoredBlocks:    10,
				MaxBlockTimeDiff:   7200,
			},
			wantErr: true,
		},
		{
			name: "invalid config - HTTP timeout too high",
			config: &Config{
				ServersMap:         []ServerConfig{{URL: "http://example.com", EXR: true}},
				AcceptedPaths:      []string{"/test"},
				AcceptedMethods:    []string{"test"},
				HttpTimeout:        301, // too high
				RateLimit:          100,
				MaxLogSize:         1024 * 1024,
				ConsensusThreshold: 0.6,
				MaxStoredBlocks:    10,
				MaxBlockTimeDiff:   7200,
			},
			wantErr: true,
		},
		{
			name: "invalid config - empty accepted paths",
			config: &Config{
				ServersMap:         []ServerConfig{{URL: "http://example.com", EXR: true}},
				AcceptedPaths:      []string{}, // empty
				AcceptedMethods:    []string{"test"},
				HttpTimeout:        30,
				RateLimit:          100,
				MaxLogSize:         1024 * 1024,
				ConsensusThreshold: 0.6,
				MaxStoredBlocks:    10,
				MaxBlockTimeDiff:   7200,
			},
			wantErr: true,
		},
		{
			name: "invalid config - empty accepted methods",
			config: &Config{
				ServersMap:         []ServerConfig{{URL: "http://example.com", EXR: true}},
				AcceptedPaths:      []string{"/test"},
				AcceptedMethods:    []string{}, // empty
				HttpTimeout:        30,
				RateLimit:          100,
				MaxLogSize:         1024 * 1024,
				ConsensusThreshold: 0.6,
				MaxStoredBlocks:    10,
				MaxBlockTimeDiff:   7200,
			},
			wantErr: true,
		},
		{
			name: "invalid config - no servers configured",
			config: &Config{
				ServersMap:              []ServerConfig{}, // empty
				DynlistServersProviders: []string{},       // empty
				AcceptedPaths:           []string{"/test"},
				AcceptedMethods:         []string{"test"},
				HttpTimeout:             30,
				RateLimit:               100,
				MaxLogSize:              1024 * 1024,
				ConsensusThreshold:      0.6,
				MaxStoredBlocks:         10,
				MaxBlockTimeDiff:        7200,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
