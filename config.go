package main

import (
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v2"
)

// ServerConfig defines the structure for a single server in the config.
type ServerConfig struct {
	URL string `yaml:"url"`
	EXR bool   `yaml:"exr"`
}

// Config holds all configuration for the application.
type Config struct {
	// DynlistServersProviders is a list of remote URLs to fetch dynamic server lists from.
	// Enabled with `-dynlist=true`.
	DynlistServersProviders []string `yaml:"dynlist_servers_providers"`
	// ServersMap is a static list of servers to use as remote endpoints.
	// Used by default if `-dynlist=true` is not provided.
	ServersMap []ServerConfig `yaml:"servers_map"`
	// AcceptedPaths is a whitelist of API paths the proxy will accept.
	AcceptedPaths []string `yaml:"accepted_paths"`
	// AcceptedMethods is a whitelist of RPC methods the proxy will relay.
	AcceptedMethods []string `yaml:"accepted_methods"`
	// MaxStoredBlocks is the maximum number of blocks to cache for validation purposes.
	MaxStoredBlocks int `yaml:"max_stored_blocks"`
	// MaxBlockTimeDiff is the maximum allowed time difference (in seconds) between a block's timestamp
	// and system time for a server to be considered healthy.
	MaxBlockTimeDiff int `yaml:"max_block_time_diff"`
	// HttpTimeout is the timeout in seconds for HTTP requests to backend servers.
	HttpTimeout int `yaml:"http_timeout"`
	// RateLimit is the maximum number of requests allowed per minute.
	RateLimit int `yaml:"rate_limit"`
	// MaxLogSize is the maximum size in bytes for the log file before it is rotated.
	MaxLogSize int `yaml:"max_log_size"`
	// ConsensusThreshold is the minimum ratio of servers that must agree for a consensus to be reached.
	ConsensusThreshold float64 `yaml:"consensus_threshold"`
}

// newConfig loads the configuration from the given file.
// If the file doesn't exist, it creates a default one.
func newConfig(configFile string) (*Config, error) {
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		log.Printf("Config file %q does not exist. Creating default config.", configFile)
		return createDefaultConfig(configFile)
	}
	log.Printf("Loading existing config from %q.", configFile)
	return loadConfig(configFile)
}

// createDefaultConfig creates a default configuration, writes it to a file, and returns it.
func createDefaultConfig(configFile string) (*Config, error) {
	defaultConfig := &Config{
		DynlistServersProviders: []string{
			"https://utils.blocknet.org",
			"http://exrproxy1.airdns.org:42114",
		},

		ServersMap: []ServerConfig{
			{URL: "http://exrproxy1.airdns.org:42114", EXR: true},
		},
		AcceptedPaths: []string{
			"/",
			"/height",
			"/heights",
			"/fees",
			"/ping",
			"/servers",
		},
		AcceptedMethods: []string{
			"getutxos",
			"getrawtransaction",
			"getrawmempool",
			"getblockcount",
			"sendrawtransaction",
			"gettransaction",
			"getblock",
			"getblockhash",
			"heights",
			"fees",
			"getbalance",
			"gethistory",
			"ping",
		},
		MaxStoredBlocks:    3,
		MaxBlockTimeDiff:   7200,
		HttpTimeout:        8,
		RateLimit:          100,
		MaxLogSize:         50 * 1024 * 1024, // 50 MB
		ConsensusThreshold: 2.0 / 3.0,        // 66% consensus rule
	}

	data, err := yaml.Marshal(defaultConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal default config: %w", err)
	}

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write default config to file: %w", err)
	}

	log.Println("Default config created and written to file.")
	return defaultConfig, nil
}

// loadConfig reads a configuration file and unmarshals it.
func loadConfig(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	log.Println("Config loaded from file.")
	return &cfg, nil
}
