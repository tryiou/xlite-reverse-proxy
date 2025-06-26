package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

type JsonElement struct {
	NodePubKey     string            `json:"nodepubkey"`
	PaymentAddress string            `json:"paymentaddress"`
	Config         string            `json:"config"`
	Plugins        map[string]string `json:"plugins"`
}

type JsonResponse struct {
	Result string  `json:"result"`
	Error  *string `json:"error"`
	Id     int     `json:"id"`
}

func getRawContentFromURL(url string) (string, error) {
	logger.Printf("|SERVERS_UPDATE| Fetching content from URL: %s", url)
	resp, err := http.Get(url)

	if err != nil {
		logger.Printf("|SERVERS_UPDATE| Error fetching config from URL %s: %v", url, err)
		return "", err
	}
	defer resp.Body.Close()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Printf("|SERVERS_UPDATE| Error reading response body from URL %s: %v", url, err)
		return "", err
	}

	logger.Printf("|SERVERS_UPDATE| Successfully fetched content from URL: %s", url)
	return string(content), nil
}

func parseConfig(config string) map[string]map[string]string {
	configMap := make(map[string]map[string]string)
	currentSection := ""

	scanner := bufio.NewScanner(strings.NewReader(config))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) != "" {
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				currentSection = strings.TrimPrefix(strings.TrimSuffix(line, "]"), "[")
				continue
			}

			if fields := strings.SplitN(line, "=", 2); len(fields) == 2 {
				key := strings.TrimSpace(fields[0])
				value := strings.TrimSpace(fields[1])

				sectionMap, ok := configMap[currentSection]
				if !ok {
					sectionMap = make(map[string]string)
					configMap[currentSection] = sectionMap
				}

				sectionMap[key] = value
			}
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Printf("|SERVERS_UPDATE| Error parsing config: %v", err)
	}

	if configMap["Main"] == nil || configMap["Main"]["plugins"] == "" {
		logger.Printf("|SERVERS_UPDATE| Error: Missing 'plugins' key in config for server")
	}

	return configMap
}

func areAllPluginsPresent(acceptedMethods, plugins_lst []string) bool {
	pluginMap := make(map[string]bool)

	// Mark all plugins in plugins_lst as present in the map
	for _, plugin := range plugins_lst {
		pluginMap[plugin] = true
	}

	// Check if every accepted method is present in the plugin map
	for _, method := range acceptedMethods {
		if !pluginMap[method] {
			return false
		}
	}

	return true
}

func findMissingPlugins(acceptedMethods, plugins_lst []string) []string {
	missingPlugins := make([]string, 0)

	for _, plugin := range acceptedMethods {
		found := false
		for _, p := range plugins_lst {
			if plugin == p {
				found = true
				break
			}
		}
		if !found {
			missingPlugins = append(missingPlugins, plugin)
		}
	}

	return missingPlugins
}

func filterServersByPlugins(elements []JsonElement, serverURL string) (string, error) {
	logger.Printf("|SERVERS_UPDATE| Filtering servers by plugins with base URL: %s", serverURL)
	var serversJSON []map[string]interface{}
	serverObj := map[string]interface{}{
		"url": serverURL,
		"exr": true, // Assuming all filtered servers have "exr" as true
	}
	serversJSON = append(serversJSON, serverObj)
	for _, elem := range elements {
		configMap := parseConfig(elem.Config)
		plugins_lst := strings.Split(configMap["Main"]["plugins"], ",")
		if areAllPluginsPresent(config.AcceptedMethods, plugins_lst) {
			url := configMap["Main"]["host"]
			port := configMap["Main"]["port"]
			host := "http://" + url + ":" + port
			serverObj := map[string]interface{}{
				"url": host,
				"exr": true, // Assuming all filtered servers have "exr" as true
			}
			serversJSON = append(serversJSON, serverObj)
		}
	}

	jsonBytes, err := json.Marshal(serversJSON)
	if err != nil {
		logger.Printf("|SERVERS_UPDATE| Error marshalling JSON: %v", err)
		return "", err
	}

	logger.Printf("|SERVERS_UPDATE| Successfully filtered servers by plugins")
	return string(jsonBytes), nil
}

func parseXrshowconfigs(xrShowConfigs_serverURL string) string {
	var rawContent string
	var err error
	var successfulServerURL string

	fullURL := xrShowConfigs_serverURL + "/xrs/xrshowconfigs"
	logger.Printf("|SERVERS_UPDATE| Parsing Xrshowconfigs from URL: %s", fullURL)
	rawContent, err = getRawContentFromURL(fullURL)
	if err == nil {
		successfulServerURL = xrShowConfigs_serverURL
		logger.Printf("|SERVERS_UPDATE| Xrshowconfigs URL: %s", successfulServerURL)
	} else {
		return ""
	}

	var response JsonResponse
	err = json.Unmarshal([]byte(rawContent), &response)
	if err != nil {
		logger.Printf("|SERVERS_UPDATE| Error unmarshalling JSON: %v", err)
		return ""
	}

	jsonArrayOfString := response.Result

	var elements []JsonElement
	err = json.Unmarshal([]byte(jsonArrayOfString), &elements)
	if err != nil {
		logger.Printf("|SERVERS_UPDATE| Error unmarshalling result JSON: %v", err)
		return ""
	}

	readyCount, notReadyCount := 0, 0

	for i, elem := range elements {
		serverID := i + 1
		configMap := parseConfig(elem.Config)
		plugins_lst := strings.Split(configMap["Main"]["plugins"], ",")

		logger.Printf("|SERVERS_UPDATE| Server %d: NodePubKey=%s, Host=%s:%s, Plugins=%v, Status: %s",
			serverID, elem.NodePubKey, configMap["Main"]["host"], configMap["Main"]["port"],
			plugins_lst,
			func() string {
				if areAllPluginsPresent(config.AcceptedMethods, plugins_lst) {
					readyCount++
					return "READY FOR XLITE"
				}
				notReadyCount++
				return "NOT READY FOR XLITE"
			}())

		if !areAllPluginsPresent(config.AcceptedMethods, plugins_lst) {
			missingPlugins := findMissingPlugins(config.AcceptedMethods, plugins_lst)
			logger.Printf("|SERVERS_UPDATE| Server %d: Missing Plugins: %v", serverID, missingPlugins)
		}
	}

	filteredServersJson, err := filterServersByPlugins(elements, successfulServerURL)
	if err != nil {
		logger.Printf("|SERVERS_UPDATE| Error filtering servers: %v", err)
		return ""
	}

	logger.Printf("|SERVERS_UPDATE| Parsed %d servers. %d are READY FOR XLITE, %d are NOT READY FOR XLITE",
		len(elements), readyCount, notReadyCount)

	logger.Printf("|SERVERS_UPDATE| Successfully parsed Xrshowconfigs")
	return string(filteredServersJson)
}

func UpdateServersFromJSON(servers *Servers) {
	serverConfigs := config.ServersMap

	existingServers := make(map[string]bool)
	for _, server := range servers.Slice {
		existingServers[server.url] = false
	}

	logger.Printf("|SERVERS_UPDATE|_UpdateServersFromJSON, Received server map: %v", serverConfigs)

	// Iterate over slice directly
	for _, serverCfg := range serverConfigs {
		url, _ := serverCfg["url"].(string)
		exr, _ := serverCfg["exr"].(bool)

		if server, ok := servers.GetServerByURL(url); ok {
			logger.Printf("|SERVERS_UPDATE|_UpdateServersFromJSON, Updating server[%d]: URL=%s, EXR=%v", server.id, url, exr)
			mu.Lock()
			server.url = url
			server.exr = exr
			mu.Unlock()
		} else {
			newServer := &Server{
				url: url,
				exr: exr,
			}
			id := servers.AddServer(newServer)
			logger.Printf("|SERVERS_UPDATE|_UpdateServersFromJSON, Adding new server[%d]: URL=%s, EXR=%v", id, url, exr)
		}

		existingServers[url] = true
	}

	for i := len(servers.Slice) - 1; i >= 0; i-- {
		server := servers.Slice[i]
		if !existingServers[server.url] {
			logger.Printf("|SERVERS_UPDATE|_UpdateServersFromJSON, Removing server[%d]: URL=%s", server.id, server.url)
			servers.RemoveServer(server)
		}
	}

	logger.Printf("Successfully updated servers from JSON")
}

func updateServers(servers *Servers) {
	for _, xrShowConfigs_serverURL := range config.Dynlist_servers_providers {
		dynServersJson := parseXrshowconfigs(xrShowConfigs_serverURL)
		if dynServersJson == "" {
			logger.Printf("|SERVERS_UPDATE| No valid servers found from URL: %s", xrShowConfigs_serverURL)
			continue
		}

		var serverConfigs []map[string]interface{}
		if err := json.Unmarshal([]byte(dynServersJson), &serverConfigs); err != nil {
			logger.Printf("|SERVERS_UPDATE|_updateServers, JSON unmarshal error: %v", err)
			continue
		}

		config.ServersMap = serverConfigs
		UpdateServersFromJSON(servers)
		break
	}
}

func startServerUpdateRoutine(servers *Servers) {
	logger.Printf("|SERVERS_UPDATE| Starting server update routine")

	updateServers(servers)
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			logger.Printf("|SERVERS_UPDATE| Sleeping for 5 minutes before next update")
			updateServers(servers)
		}
	}()
}
