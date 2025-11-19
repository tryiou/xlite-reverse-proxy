package main

import (
	"fmt"
	"log"
	"net/url"
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
	cfg, err := loadConfig(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration from %s: %w", configFile, err)
	}

	// Validate configuration after loading
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
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
		HttpTimeout:        30,
		RateLimit:          100,
		MaxLogSize:         50 * 1024 * 1024, // 50 MB
		ConsensusThreshold: 2.0 / 3.0,        // 66% consensus rule
	}

	data, err := yaml.Marshal(defaultConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal default config: %w", err)
	}

	if err := os.WriteFile(configFile, data, FilePermissionRWXRWXR); err != nil {
		return nil, fmt.Errorf("failed to write default config to file %s: %w", configFile, err)
	}

	log.Printf("Default config created and written to file: %s", configFile)
	return defaultConfig, nil
}

// validateConfig validates the configuration values for correctness
func validateConfig(cfg *Config) error {
	// Validate HTTP timeout
	if cfg.HttpTimeout <= 0 {
		return fmt.Errorf("HTTP timeout must be positive, got %d", cfg.HttpTimeout)
	}
	if cfg.HttpTimeout > 300 { // 5 minutes max
		return fmt.Errorf("HTTP timeout too high: %d seconds (max 300)", cfg.HttpTimeout)
	}

	// Validate rate limit
	if cfg.RateLimit <= 0 {
		return fmt.Errorf("rate limit must be positive, got %d", cfg.RateLimit)
	}
	if cfg.RateLimit > 1000 {
		return fmt.Errorf("rate limit too high: %d requests/minute (max 1000)", cfg.RateLimit)
	}

	// Validate consensus threshold
	if cfg.ConsensusThreshold <= 0 || cfg.ConsensusThreshold > 1 {
		return fmt.Errorf("consensus threshold must be between 0 and 1, got %.2f", cfg.ConsensusThreshold)
	}

	// Validate max stored blocks
	if cfg.MaxStoredBlocks <= 0 {
		return fmt.Errorf("max stored blocks must be positive, got %d", cfg.MaxStoredBlocks)
	}
	if cfg.MaxStoredBlocks > 100 {
		return fmt.Errorf("max stored blocks too high: %d (max 100)", cfg.MaxStoredBlocks)
	}

	// Validate max block time diff
	if cfg.MaxBlockTimeDiff <= 0 {
		return fmt.Errorf("max block time diff must be positive, got %d", cfg.MaxBlockTimeDiff)
	}
	if cfg.MaxBlockTimeDiff > 86400 { // 24 hours max
		return fmt.Errorf("max block time diff too high: %d seconds (max 86400)", cfg.MaxBlockTimeDiff)
	}

	// Validate max log size
	if cfg.MaxLogSize <= 0 {
		return fmt.Errorf("max log size must be positive, got %d", cfg.MaxLogSize)
	}
	if cfg.MaxLogSize < 1024*1024 { // 1MB minimum
		return fmt.Errorf("max log size too small: %d bytes (min 1048576)", cfg.MaxLogSize)
	}
	if cfg.MaxLogSize > 100*1024*1024 { // 100MB maximum
		return fmt.Errorf("max log size too high: %d bytes (max %d)", cfg.MaxLogSize, 100*1024*1024)
	}

	// Validate accepted paths is not empty
	if len(cfg.AcceptedPaths) == 0 {
		return fmt.Errorf("accepted paths list cannot be empty")
	}

	// Validate accepted methods is not empty
	if len(cfg.AcceptedMethods) == 0 {
		return fmt.Errorf("accepted methods list cannot be empty")
	}

	// Validate server configurations if present
	if len(cfg.ServersMap) == 0 && len(cfg.DynlistServersProviders) == 0 {
		return fmt.Errorf("no servers configured - either ServersMap or DynlistServersProviders must have entries")
	}

	// Validate server URLs
	for i, server := range cfg.ServersMap {
		if err := validateURLFormat(server.URL); err != nil {
			return fmt.Errorf("server %d URL validation failed: %v", i, err)
		}
	}

	// Validate dynamic server providers
	for i, provider := range cfg.DynlistServersProviders {
		if err := validateURLFormat(provider); err != nil {
			return fmt.Errorf("dynamic server provider %d URL validation failed: %v", i, err)
		}
	}

	return nil
}

// validateURLFormat validates a URL string using proper URL parsing
func validateURLFormat(urlStr string) error {
	if urlStr == "" {
		return fmt.Errorf("URL cannot be empty")
	}
	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %s", urlStr)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("URL missing protocol or host: %s", urlStr)
	}
	return nil
}

// loadConfig reads a configuration file and unmarshals it.
func loadConfig(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", filePath, err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("config file %s is empty", filePath)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML in config file %s: %w", filePath, err)
	}

	log.Printf("Config loaded from file: %s", filePath)
	return &cfg, nil
}
