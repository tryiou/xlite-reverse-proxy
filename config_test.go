package main

import (
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
