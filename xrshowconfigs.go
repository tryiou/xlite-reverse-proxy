// Parsing the xrshowconfigs call

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
	resp, err := http.Get(url)

	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

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
		fmt.Println("Error parsing config:", err)
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
		return "", err
	}

	return string(jsonBytes), nil
}

func parseXrshowconfigs(xrShowConfigs_serverURL string) string {
	var rawContent string
	var err error
	var successfulServerURL string

	fullURL := xrShowConfigs_serverURL + "/xrs/xrshowconfigs"
	rawContent, err = getRawContentFromURL(fullURL)
	if err == nil {
		successfulServerURL = xrShowConfigs_serverURL
		fmt.Println("Xrshowconfigs URL:", successfulServerURL)
	}
	fmt.Println("Error getting content from URL:", xrShowConfigs_serverURL, err)

	if err != nil {
		return ""
	}

	var response JsonResponse
	err = json.Unmarshal([]byte(rawContent), &response)
	if err != nil {
		fmt.Println("Error unmarshalling JSON:", err)
		return ""
	}

	jsonArrayOfString := response.Result

	var elements []JsonElement
	err = json.Unmarshal([]byte(jsonArrayOfString), &elements)
	if err != nil {
		fmt.Println("Error unmarshalling result JSON:", err)
		return ""
	}

	for i, elem := range elements {
		configMap := parseConfig(elem.Config)
		plugins_lst := strings.Split(configMap["Main"]["plugins"], ",")
		fmt.Printf("SNODE %d:\n", i+1)
		fmt.Printf("NodePubKey: %s\n", elem.NodePubKey)
		fmt.Println(configMap["Main"]["host"], configMap["Main"]["port"])
		if areAllPluginsPresent(config.AcceptedMethods, plugins_lst) {
			fmt.Println("READY FOR XLITE")
			//fmt.Printf("PaymentAddress: %s\n", elem.PaymentAddress)
			//fmt.Println("Config:")
		} else {
			fmt.Println("NOT READY FOR XLITE")
			//fmt.Println(plugins_lst)
			// Find missing plugins
			missingPlugins := findMissingPlugins(config.AcceptedMethods, plugins_lst)
			fmt.Println("Missing Plugins:")
			fmt.Println(missingPlugins)
		}
		fmt.Println()
	}
	filteredServersJson, err := filterServersByPlugins(elements, successfulServerURL)
	if err != nil {
		fmt.Println("Error filtering servers:", err)
	}

	return string(filteredServersJson)
}

func UpdateServersFromJSON(servers *Servers) {
	// Now using config.serversJsonList directly as slice
	serverConfigs := config.ServersMap

	existingServers := make(map[string]bool)
	for _, server := range servers.Slice {
		existingServers[server.url] = false
	}

	// Iterate over slice directly
	for _, serverCfg := range serverConfigs {
		url, _ := serverCfg["url"].(string)
		exr, _ := serverCfg["exr"].(bool)

		if server, ok := servers.serverGetByURL(url); ok {
			logger.Printf("|SERVERS|_UpdateServersFromJSON, Updating server: URL=%s, EXR=%v\n", url, exr)
			mu.Lock()
			server.url = url
			server.exr = exr
			mu.Unlock()
		} else {
			newServer := &Server{
				url: url,
				exr: exr,
			}
			id := servers.serverAdd(newServer)
			logger.Printf("|SERVERS|_UpdateServersFromJSON, Adding new server[%d]: URL=%s, EXR=%v\n", id, url, exr)
		}

		existingServers[url] = true
	}

	for i := len(servers.Slice) - 1; i >= 0; i-- {
		server := servers.Slice[i]
		if !existingServers[server.url] {
			logger.Printf("|SERVERS|_UpdateServersFromJSON, Removing server: URL=%s\n", server.url)
			servers.serverRemove(server)
		}
	}
}

func updateServers(servers *Servers) {
	for _, xrShowConfigs_serverURL := range config.XrshowconfigsServers {
		dynServersJson := parseXrshowconfigs(xrShowConfigs_serverURL)
		if dynServersJson == "" {
			continue
		}

		var serverConfigs []map[string]interface{}
		if err := json.Unmarshal([]byte(dynServersJson), &serverConfigs); err != nil {
			logger.Println("|SERVERS|_updateServers, JSON unmarshal error:", err)
			continue
		}

		config.ServersMap = serverConfigs
		UpdateServersFromJSON(servers)
		break
	}
}

func startServerUpdateRoutine(servers *Servers) {

	updateServers(servers)
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			updateServers(servers)
		}
	}()
}
