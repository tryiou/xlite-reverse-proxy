package main

import (
	"log"
	"os"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Dynlist_servers_providers []string                 `yaml:"dynlist_servers_providers"`
	ServersMap                []map[string]interface{} `yaml:"servers_map"`
	AcceptedPaths             []string                 `yaml:"accepted_paths"`
	AcceptedMethods           []string                 `yaml:"accepted_methods"`
	MaxStoredBlocks           int                      `yaml:"max_stored_blocks"`
	MaxBlockTimeDiff          int                      `yaml:"max_block_time_diff"`
	HttpTimeout               int                      `yaml:"http_timeout"`
	RateLimit                 int                      `yaml:"rate_limit"`
	MaxLogSize                int                      `yaml:"max_log_size"`
	ConsensusThreshold        float64                  `yaml:"consensus_threshold"`
}

var config Config

func initConfig() {
	configFile := "xlite-reverse-proxy-config.yaml"

	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		log.Println("Config file does not exist. Creating default config.")
		createDefaultConfig(configFile)
	} else {
		log.Println("Loading existing config from file.")
		loadConfig(configFile)
	}
}

func createDefaultConfig(configFile string) {
	defaultConfig := Config{
		Dynlist_servers_providers: []string{
			"https://utils.blocknet.org",
			"http://exrproxy1.airdns.org:42114",
		},

		ServersMap: []map[string]interface{}{
			{"url": "http://exrproxy1.airdns.org:42114", "exr": true},
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

	// log.Printf("Default config before marshalling: %+v\n", defaultConfig)

	data, err := yaml.Marshal(defaultConfig)
	if err != nil {
		log.Fatalf("Failed to create default config: %v", err)
	}
	// log.Printf("Marshalled YAML data: %s\n", string(data))

	err = os.WriteFile(configFile, data, 0644)
	if err != nil {
		log.Fatalf("Failed to write default config to file: %v", err)
	}

	config = defaultConfig
	log.Println("Default config created and written to file.")
}

func loadConfig(filePath string) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		log.Fatalf("Failed to parse config file: %v", err)
	}

	log.Println("Config loaded from file.")
}
