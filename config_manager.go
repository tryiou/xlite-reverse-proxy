package main

import (
	"net/http"
	"sync"
	"time"
)

type ConfigManager struct {
	mu     sync.RWMutex
	config *Config
	client *http.Client
	cache  *OptimizedBlockCache
}

func (cm *ConfigManager) Load(configFile string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cfg, err := newConfig(configFile)
	if err != nil {
		return err
	}

	cm.config = cfg
	cm.client = &http.Client{
		Timeout: time.Duration(cfg.HttpTimeout) * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        HTTPMaxIdleConns,
			IdleConnTimeout:     time.Duration(cfg.HttpTimeout) * time.Second,
			DisableCompression:  false,
			MaxIdleConnsPerHost: HTTPMaxIdleConnsPerHost,
			MaxConnsPerHost:     HTTPMaxConnsPerHost,
		},
	}
	cm.cache = NewOptimizedBlockCache(cfg.MaxStoredBlocks)

	return nil
}

func (cm *ConfigManager) GetConfig() *Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config
}

func (cm *ConfigManager) GetClient() *http.Client {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.client
}

func (cm *ConfigManager) GetCache() *OptimizedBlockCache {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.cache
}

func (cm *ConfigManager) UpdateServerList(servers []ServerConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.config == nil {
		return nil
	}

	cm.config.ServersMap = servers

	return nil
}

var (
	globalConfig ConfigManager
)
