// Package main contains the logic for the xlite-reverse-proxy.
// This file, servers_update.go, is responsible for dynamically updating the
// list of backend servers from remote providers. It fetches server lists,
// filters them based on required plugins, and syncs the active server pool.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// fetchURLContent performs an HTTP GET request to the specified URL and returns the response body.
func fetchURLContent(url string) ([]byte, error) {
	logger.Printf("|SERVERS_UPDATE| Fetching content from URL: %s", url)
	resp, err := http.Get(url)
	if err != nil {
		logger.Printf("|SERVERS_UPDATE| Error fetching from URL %s: %v", url, err)
		return nil, fmt.Errorf("failed to fetch from URL %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-200 status code %d from %s", resp.StatusCode, url)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Printf("|SERVERS_UPDATE| Error reading response body from URL %s: %v", url, err)
		return nil, fmt.Errorf("failed to read response body from %s: %w", url, err)
	}

	logger.Printf("|SERVERS_UPDATE| Successfully fetched content from URL: %s", url)
	return content, nil
}

// parseINIConfig parses a string in a simple INI-like format into a map.
// The format consists of sections in [section] brackets followed by key=value pairs.
func parseINIConfig(config string) map[string]map[string]string {
	configMap := make(map[string]map[string]string)
	currentSection := ""

	scanner := bufio.NewScanner(strings.NewReader(config))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.Trim(line, "[]")
			if _, ok := configMap[currentSection]; !ok {
				configMap[currentSection] = make(map[string]string)
			}
			continue
		}

		if fields := strings.SplitN(line, "=", 2); len(fields) == 2 {
			key := strings.TrimSpace(fields[0])
			value := strings.TrimSpace(fields[1])
			if sectionMap, ok := configMap[currentSection]; ok {
				sectionMap[key] = value
			}
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Printf("|SERVERS_UPDATE| Error scanning config string: %v", err)
	}

	return configMap
}

// allPluginsPresent checks if all required plugins are available in the provided list.
func allPluginsPresent(requiredPlugins, availablePlugins []string) bool {
	availableSet := make(map[string]struct{}, len(availablePlugins))
	for _, plugin := range availablePlugins {
		availableSet[plugin] = struct{}{}
	}

	for _, required := range requiredPlugins {
		if _, ok := availableSet[required]; !ok {
			return false
		}
	}

	return true
}

// findMissingPlugins identifies which of the required plugins are not in the available list.
func findMissingPlugins(requiredPlugins, availablePlugins []string) []string {
	availableSet := make(map[string]struct{}, len(availablePlugins))
	for _, p := range availablePlugins {
		availableSet[p] = struct{}{}
	}

	var missing []string
	for _, req := range requiredPlugins {
		if _, ok := availableSet[req]; !ok {
			missing = append(missing, req)
		}
	}
	return missing
}

// fetchAndFilterRemoteServers fetches a list of servers from an xrshowconfigs endpoint,
// filters them based on required plugins, and returns a list of valid server configurations.
// The providerURL itself is also included as a valid server.
func fetchAndFilterRemoteServers(providerURL string) ([]ServerConfig, error) {
	fullURL := providerURL + "/xrs/xrshowconfigs"
	logger.Printf("|SERVERS_UPDATE| Fetching remote server list from: %s", fullURL)

	rawContent, err := fetchURLContent(fullURL)
	if err != nil {
		return nil, fmt.Errorf("could not fetch server list from %s: %w", fullURL, err)
	}

	var response JsonResponse
	if err := json.Unmarshal(rawContent, &response); err != nil {
		logger.Printf("|SERVERS_UPDATE| Error unmarshalling initial JSON response: %v", err)
		return nil, fmt.Errorf("failed to unmarshal initial response: %w", err)
	}

	if response.Error != nil && *response.Error != "null" {
		return nil, fmt.Errorf("remote provider returned an error: %s", *response.Error)
	}

	// The 'result' field is a JSON string that needs to be unmarshalled separately.
	var remoteServers []JsonElement
	if err := json.Unmarshal([]byte(response.Result), &remoteServers); err != nil {
		logger.Printf("|SERVERS_UPDATE| Error unmarshalling nested result JSON: %v", err)
		return nil, fmt.Errorf("failed to unmarshal nested server list: %w", err)
	}

	var validServerConfigs []ServerConfig
	// The provider itself is considered a valid server.
	validServerConfigs = append(validServerConfigs, ServerConfig{URL: providerURL, EXR: true})

	readyCount, notReadyCount := 0, 0
	requiredPlugins := config.AcceptedMethods

	for i, serverInfo := range remoteServers {
		serverID := i + 1
		configMap := parseINIConfig(serverInfo.Config)
		mainSection, ok := configMap["Main"]
		if !ok || mainSection["plugins"] == "" || mainSection["host"] == "" || mainSection["port"] == "" {
			logger.Printf("|SERVERS_UPDATE| Server %d: Incomplete config, skipping. NodePubKey=%s", serverID, serverInfo.NodePubKey)
			notReadyCount++
			continue
		}

		availablePlugins := strings.Split(mainSection["plugins"], ",")
		host := "http://" + mainSection["host"] + ":" + mainSection["port"]

		if allPluginsPresent(requiredPlugins, availablePlugins) {
			logger.Printf("|SERVERS_UPDATE| Server %d: READY FOR XLITE. Host=%s, NodePubKey=%s", serverID, host, serverInfo.NodePubKey)
			validServerConfigs = append(validServerConfigs, ServerConfig{URL: host, EXR: true})
			readyCount++
		} else {
			notReadyCount++
			missing := findMissingPlugins(requiredPlugins, availablePlugins)
			logger.Printf("|SERVERS_UPDATE| Server %d: NOT READY FOR XLITE. Host=%s, NodePubKey=%s, Missing Plugins: %v", serverID, host, serverInfo.NodePubKey, missing)
		}
	}

	logger.Printf("|SERVERS_UPDATE| Parsed %d remote servers. %d READY, %d NOT READY.", len(remoteServers), readyCount, notReadyCount)
	return validServerConfigs, nil
}

// UpdateServersFromJSON synchronizes the proxy's active server list with a new
// set of server configurations. It adds new servers, updates existing ones,
// and removes any that are no longer in the provided configuration.
func UpdateServersFromJSON(servers *Servers) {
	serverConfigs := config.ServersMap

	logger.Printf("|SERVERS_UPDATE| Syncing server list with %d configurations.", len(serverConfigs))

	// Use a map to track which servers from the new config are present.
	// This helps identify which old servers to remove.
	configServerUrls := make(map[string]bool)

	for _, serverCfg := range serverConfigs {
		configServerUrls[serverCfg.URL] = true
		// Check if the server already exists.
		if server, ok := servers.GetServerByURL(serverCfg.URL); ok {
			// Server exists, update its properties if necessary.
			// Note: GetServerByURL now returns a pointer, so changes are reflected.
			mu.Lock()
			if server.exr != serverCfg.EXR {
				logger.Printf("|SERVERS_UPDATE| Updating server[%d]: URL=%s, EXR=%v -> %v", server.id, serverCfg.URL, server.exr, serverCfg.EXR)
				server.exr = serverCfg.EXR
			}
			mu.Unlock()
		} else {
			// Server is new, add it to the list.
			newServer := &Server{
				url: serverCfg.URL,
				exr: serverCfg.EXR,
			}
			id := servers.AddServer(newServer)
			logger.Printf("|SERVERS_UPDATE| Adding new server[%d]: URL=%s, EXR=%v", id, serverCfg.URL, serverCfg.EXR)
		}
	}

	// Remove servers that are no longer in the configuration.
	// We build a new slice to do this efficiently in one pass (O(N)).
	var updatedServerSlice []*Server
	for _, server := range servers.Slice {
		if configServerUrls[server.url] {
			updatedServerSlice = append(updatedServerSlice, server)
		} else {
			logger.Printf("|SERVERS_UPDATE| Removing server[%d]: URL=%s", server.id, server.url)
		}
	}

	mu.Lock()
	servers.Slice = updatedServerSlice
	mu.Unlock()

	logger.Printf("Successfully updated servers from JSON. Active servers: %d", len(servers.Slice))
}

// updateServersFromProviders iterates through the dynamic list providers,
// fetches and filters servers, and updates the global server configuration.
// It stops after the first provider that returns a valid list of servers.
func updateServersFromProviders(servers *Servers) {
	for _, providerURL := range config.DynlistServersProviders {
		serverConfigs, err := fetchAndFilterRemoteServers(providerURL)
		if err != nil {
			logger.Printf("|SERVERS_UPDATE| Failed to get servers from provider %s: %v", providerURL, err)
			continue
		}

		if len(serverConfigs) == 0 {
			logger.Printf("|SERVERS_UPDATE| No valid servers found from provider: %s", providerURL)
			continue
		}

		// NOTE: The dynamic update of config.ServersMap is not thread-safe.
		// A mutex should protect config reads/writes to prevent race conditions.
		config.ServersMap = serverConfigs
		UpdateServersFromJSON(servers)

		// Successfully updated from a provider, so we can stop.
		logger.Printf("|SERVERS_UPDATE| Successfully updated server list from provider: %s", providerURL)
		return
	}
	logger.Printf("|SERVERS_UPDATE| Failed to update server list from any provider.")
}

// startServerUpdateRoutine launches a goroutine that periodically updates the
// server list from dynamic providers. It performs an initial update immediately.
func startServerUpdateRoutine(servers *Servers) {
	logger.Printf("|SERVERS_UPDATE| Starting server update routine")

	// Perform an initial update on startup.
	updateServersFromProviders(servers)

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			<-ticker.C
			logger.Printf("|SERVERS_UPDATE| Triggering periodic server list update.")
			updateServersFromProviders(servers)
		}
	}()
}
