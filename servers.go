// Package main contains the logic for the xlite-reverse-proxy.
// This file, servers.go, is responsible for managing the list of backend servers,
// performing health checks, and calculating consensus on the data they provide.
// This includes consensus on block heights, transaction fees, and block hashes
// to ensure the data served by the proxy is reliable and consistent.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fastjson"
)

// AddServer adds a new server to the list of servers managed by the proxy.
// It assigns a unique ID to the server and initializes its data structures.
// This function is safe for concurrent use.
func (servers *Servers) AddServer(s *Server) int {
	mu.Lock()
	defer mu.Unlock()

	if servers.Slice == nil {
		servers.Slice = []*Server{}
	}
	// Create URL-ID map if needed
	if servers.urlToID == nil {
		servers.urlToID = make(map[string]int)
	}

	// Check if URL exists in map and assign stored ID
	if id, exists := servers.urlToID[s.url]; exists {
		s.id = id
	} else {
		// New server: find next available ID
		maxID := 0
		for _, server := range servers.Slice {
			if server.id > maxID {
				maxID = server.id
			}
		}
		s.id = maxID + 1
		servers.urlToID[s.url] = s.id
	}

	servers.Slice = append(servers.Slice, s)
	s.getfees = getDefaultJSONResponse()
	s.getheights = getDefaultJSONResponse()
	s.coinsMap = make(map[string]Coin)
	s.hashesStorage = make(map[string]map[int]string)
	return s.id
}

// RemoveServer removes a server from the list of managed servers.
// This function is safe for concurrent use.
func (servers *Servers) RemoveServer(server *Server) {
	mu.Lock()
	defer mu.Unlock()

	for i, srv := range servers.Slice {
		if srv == server {
			servers.Slice = append(servers.Slice[:i], servers.Slice[i+1:]...)

			// Remove URL from map to allow new ID assignment.
			delete(servers.urlToID, server.url)

			break
		}
	}
}

// GetServerByID retrieves a server by its unique ID.
// It returns the server and a boolean indicating if the server was found.
func (servers *Servers) GetServerByID(id int) (Server, bool) {
	for _, server := range servers.Slice {
		if server.id == id {
			return *server, true
		}
	}
	return Server{}, false
}

// GetServerByURL retrieves a server by its URL.
// It returns the server and a boolean indicating if the server was found.
func (servers *Servers) GetServerByURL(url string) (*Server, bool) {
	for _, server := range servers.Slice {
		if server.url == url {
			return server, true
		}
	}
	return nil, false
}

// UpdateGlobalFees calculates the consensus for transaction fees across all healthy servers.
// The result is stored in `servers.GlobalFees` and used to serve cached fee requests.
func (servers *Servers) UpdateGlobalFees() {
	feeCountsPerCoin := make(map[string]map[string]int)

	for _, server := range servers.Slice {
		for coin, coinObj := range server.coinsMap {
			//fmt.Println(reflect.TypeOf(coinObj.fee), coinObj.fee)
			if coinObj.fee == 0 {
				continue
			}
			valueTruncated := math.Floor(coinObj.fee*1e8) / 1e8
			valueStr := strconv.FormatFloat(valueTruncated, 'f', -1, 64)
			coinFeeIdentifier := coin + ":" + valueStr
			if _, ok := feeCountsPerCoin[coin]; !ok {
				feeCountsPerCoin[coin] = make(map[string]int)
			}
			feeCountsPerCoin[coin][coinFeeIdentifier]++
		}
	}
	consensusFees := calculateConsensusFees(feeCountsPerCoin)
	servers.GlobalFees = consensusFees
}

// updateCoinWithNewBlock handles the logic for updating a coin's data when a new block is detected.
// It fetches the block hash, retrieves block data (from cache or network), calculates the time
// difference, and updates the coin's information accordingly.
func (s *Server) updateCoinWithNewBlock(coinStr string, heightInt int, coinMap *Coin) {
	getBlockHash, err := s.server_GetBlockHash(coinStr, heightInt)
	if err != nil {
		logger.Printf("[server%d] server_GetBlockHash failed for %s at height %d, err: %v", s.id, coinStr, heightInt, err)
		coinMap.getBlockHash = ""
		coinMap.timeDiff = -1000
		return
	}

	if getBlockHash == "" {
		coinMap.getBlockHash = ""
		coinMap.timeDiff = -1000
		return
	}

	// Check for the block in the cache or fetch it if not present.
	blockCacheKey := fmt.Sprintf("%s_%s", coinStr, getBlockHash)
	if _, exists := blockCache[blockCacheKey]; !exists {
		getBlock, err := s.server_GetBlock(coinStr, getBlockHash)
		if err != nil {
			logger.Printf("[server%d] server_GetBlock failed, err: %v", s.id, err)
		} else {
			timestamp, err := getBlock.Get("time").Int64()
			if err != nil {
				logger.Printf("[server%d] getBlock.Get('time').Int64() failed, err: %v", s.id, err)
			} else {
				blockTime := time.Unix(timestamp, 0)
				desktopTime := time.Now()
				timeDifference := desktopTime.Sub(blockTime).Seconds()
				logger.Printf("%s, %d, Time difference between block time and desktop time: %.2f seconds", coinStr, s.id, timeDifference)

				// Update the cache with the new block data.
				blockCache[blockCacheKey] = &BlockCache{
					BlockHash: getBlockHash,
					timeDiff:  timeDifference,
				}
				purgeCache(blockCache, config.MaxStoredBlocks)
			}
		}
	}

	// Exclude coin if the time difference between block time and desktop time is too high.
	if blockDataCache, exists := blockCache[blockCacheKey]; exists {
		if blockDataCache.timeDiff < float64(config.MaxBlockTimeDiff) {
			coinMap.getBlockHash = getBlockHash
			coinMap.timeDiff = blockDataCache.timeDiff
		} else {
			coinMap.getBlockHash = blockDataCache.BlockHash
			coinMap.timeDiff = -1000 // Mark as unhealthy due to time diff
			logger.Printf("[server%d] timeDiff too high for %s: %f at height %d", s.id, coinStr, blockDataCache.timeDiff, heightInt)
		}
	} else {
		// Block data could not be fetched or found in cache.
		coinMap.getBlockHash = ""
		coinMap.timeDiff = -1000
	}
}

// updateCoinData updates the coin map for a single server.
// It fetches block hashes for new block heights, calculates block time differences,
// and caches relevant block data. It also populates fee information.
func (s *Server) updateCoinData() error {
	heightsObj := s.getheights.Get("result")
	if heightsObj == nil || heightsObj.Type() != fastjson.TypeObject {
		s.coinsMap = make(map[string]Coin)
		return nil
	}

	obj, err := heightsObj.Object()
	if err != nil {
		logger.Printf("[server%d]_updateCoinData, error with heights object: %v", s.id, err)
		s.coinsMap = make(map[string]Coin)
		return err
	}

	obj.Visit(func(coin []byte, height *fastjson.Value) {
		coinStr := string(coin)
		heightInt := -1
		if height.String() != "null" {
			var err error
			heightInt, err = height.Int()
			if err != nil {
				logger.Printf("updateCoinData: error parsing height for coin %s: %v", coinStr, err)
				heightInt = -1
			}
		}

		coinMap := s.coinsMap[coinStr]
		if coinMap.getBlockCount != heightInt {
			coinMap.getBlockCount = heightInt
			if heightInt > 0 {
				s.updateCoinWithNewBlock(coinStr, heightInt, &coinMap)
			} else {
				coinMap.getBlockHash = ""
				coinMap.timeDiff = -1000
			}
			s.coinsMap[coinStr] = coinMap

			if s.coinsMap[coinStr].getBlockHash != "" {
				if s.hashesStorage[coinStr] == nil {
					s.hashesStorage[coinStr] = make(map[int]string)
				}
				s.hashesStorage[coinStr][heightInt] = s.coinsMap[coinStr].getBlockHash
			}
			logger.Printf("[server%d]_New_Block %-5s: %-9d:%s\n", s.id, coinStr, s.coinsMap[coinStr].getBlockCount, s.coinsMap[coinStr].getBlockHash)
		}
	})

	// Update fee information
	feesObj := s.getfees.Get("result")
	if feesObj != nil && feesObj.Type() == fastjson.TypeObject {
		obj, err := feesObj.Object()
		if err != nil {
			logger.Printf("updateCoinData, error with fees object: %v", err)
			return err
		}
		obj.Visit(func(coin []byte, fee *fastjson.Value) {
			coinStr := string(coin)
			if fee != nil && fee.Type() == fastjson.TypeNumber {
				feeFloat, err := fee.Float64()
				if err == nil {
					coinMap := s.coinsMap[coinStr]
					coinMap.fee = feeFloat
					s.coinsMap[coinStr] = coinMap
				}
			}
		})
		s.pruneHashStorage()
	}
	return nil
}

// updateCoinDataForAllServers iterates through all servers and updates their
// respective coin data, including block counts, block hashes, and fees.
func (servers *Servers) updateCoinDataForAllServers() {
	for _, server := range servers.Slice {
		if err := server.updateCoinData(); err != nil {
			logger.Printf("[server%d] failed to update coin data: %v", server.id, err)
		}
	}
}

// UpdateGlobalHeights calculates the consensus for block heights across all healthy servers.
// It determines the most common block height for each coin and identifies which servers
// are in consensus. The results are stored in `servers.GlobalHeights` and `servers.GlobalCoinServerIDs`.
func (servers *Servers) UpdateGlobalHeights() {
	// 1. Build a map of heights for each coin from all servers.
	heightsMap, err := buildCoinHeightsMap(servers)
	if err != nil {
		logger.Printf(" error extracting results: %v", err)
		servers.GlobalHeights = getDefaultJSONResponse()
		servers.GlobalCoinServerIDs = getEmptyJSONResponse()
		return
	}

	// 2. Compute the most common height range for each coin.
	mostCommonHeightsRanges := computeMostCommonHeightRanges(heightsMap)

	// 3. Calculate the consensus ratio for each coin's height.
	ratioMap := calculateRatio(heightsMap, mostCommonHeightsRanges)

	// 4. Find servers that are within the consensus height range.
	commonHeightServers := findServersInConsensusHeightRange(servers, mostCommonHeightsRanges, ratioMap)

	// 5. Create the JSON response for servers in consensus.
	commonHeightServersJSON, err := createCommonHeightServersJSON(commonHeightServers)
	if err != nil {
		logger.Printf(" error creating JSON for common height servers: %v", err)
		servers.GlobalHeights = getDefaultJSONResponse()
		servers.GlobalCoinServerIDs = getEmptyJSONResponse()
		return
	}

	// 6. Create the global heights JSON response.
	globalHeightsJSON, err := createGlobalHeightsJSON(mostCommonHeightsRanges)
	if err != nil {
		logger.Printf(" error creating JSON for GGet heights: %v", err)
		servers.GlobalHeights = getDefaultJSONResponse()
		servers.GlobalCoinServerIDs = getEmptyJSONResponse()
		return
	}
	servers.GlobalHeights = globalHeightsJSON

	// 7. Sort the coin server IDs map alphabetically by coin.
	sortedGlobalCoinServerIDsJSON, err := sortGlobalCoinServerIDs(commonHeightServersJSON)
	if err != nil {
		logger.Printf(" error sorting coins map: %v", err)
		servers.GlobalCoinServerIDs = getEmptyJSONResponse()
	}
	servers.GlobalCoinServerIDs = sortedGlobalCoinServerIDsJSON

	// 8. Update individual server coin data based on new heights.
	servers.updateCoinDataForAllServers()

	// 9. Find and remove servers with non-consensus block hashes.
	nonConsensusServersMap := FindServersFailingHashConsensus(servers)
	if len(nonConsensusServersMap) > 0 {
		logger.Printf("getGlobalHeights, nonConsensusServersMap = %v", nonConsensusServersMap)
		servers.removeNonConsensusServersFromGlobalList(nonConsensusServersMap)
	}
}

// RemoveServerFromGlobalCoinList removes a specific server ID from a coin's list of valid servers.
// This is typically called when a server fails a health check or request.
func (servers *Servers) RemoveServerFromGlobalCoinList(coin string, id int) {
	mu.Lock()
	defer mu.Unlock()

	if servers.GlobalCoinServerIDs == nil {
		return
	}
	coinInfo := servers.GlobalCoinServerIDs.Get(coin)
	if coinInfo == nil {
		return
	}
	ids := coinInfo.Get("ids")
	if ids == nil || ids.Type() != fastjson.TypeArray {
		return
	}

	idsArray := ids.GetArray()
	newIdsValues := make([]*fastjson.Value, 0, len(idsArray))
	var updatedIDsForLog []int
	idFound := false

	for _, idValue := range idsArray {
		serverID, _ := idValue.Int()
		if serverID != id {
			newIdsValues = append(newIdsValues, idValue)
			updatedIDsForLog = append(updatedIDsForLog, serverID)
		} else {
			idFound = true
		}
	}

	if !idFound {
		return // ID was not in the list.
	}

	// Create a new fastjson.Array with the updated IDs.
	newArrayValue := fastjson.MustParse("[]")
	for i, v := range newIdsValues {
		newArrayValue.SetArrayItem(i, v)
	}

	// Update the "ids" field in coinInfo.
	coinInfo.Set("ids", newArrayValue)
	logger.Printf("Removed ID %d from coin '%s' IDs: %v\n", id, coin, updatedIDsForLog)
}

// removeNonConsensusServersFromGlobalList removes server IDs from the global list for a given coin
// if they are part of a non-consensus group. It also cleans up the coin data from the affected servers.
func (servers *Servers) removeNonConsensusServersFromGlobalList(nonConsensusMap map[string][]int) {
	mu.Lock()
	defer mu.Unlock()

	gCoinsIDs := servers.GlobalCoinServerIDs
	for coin, serverIDs := range nonConsensusMap {
		coinInfo := gCoinsIDs.Get(coin)
		if coinInfo != nil {
			ids := coinInfo.Get("ids")
			if ids != nil && ids.Type() == fastjson.TypeArray {
				updatedIDs := make([]*fastjson.Value, 0)
				idsArray := ids.GetArray()
				for _, idValue := range idsArray {
					serverID, _ := idValue.Int()
					if !slices.Contains(serverIDs, serverID) {
						updatedIDs = append(updatedIDs, idValue)
					}
				}

				// Create a new fastjson.Array with the updated IDs.
				newIDsValue := fastjson.MustParse("[]")
				for i, v := range updatedIDs {
					newIDsValue.SetArrayItem(i, v)
				}

				coinInfo.Set("ids", newIDsValue)
				logger.Printf("Updated 'ids' array for coin: %s %v\n", coin, newIDsValue)

				// Delete the non-consensus coin(s) from each affected server's coinMap.
				for _, serverID := range serverIDs {
					if server, exists := servers.GetServerByID(serverID); exists {
						logger.Printf("[server%d] Removing %s from coinMap\n", server.id, coin)
						delete(server.coinsMap, coin)
					}
				}
			}
		}
	}
}

// GetRandomValidServerID selects a random, healthy server for a given coin from the list of
// servers that are in consensus.
func (servers *Servers) GetRandomValidServerID(coin string) (int, error) {
	coinObj := servers.GlobalCoinServerIDs.GetObject(coin)
	if coinObj == nil {
		errMsg := fmt.Sprintf("Coin '%s' not found", coin)
		return -1, errors.New(errMsg)
	}
	coinArrayValue := coinObj.Get("ids")
	if coinArrayValue == nil {
		errMsg := fmt.Sprintf("Server IDs array not found for coin '%s'", coin)
		return -1, errors.New(errMsg)
	}
	coinArray, err := coinArrayValue.Array()
	if err != nil {
		return -1, err
	}
	// Get the length of the coinArray
	coinArrayLen := len(coinArray)
	if coinArrayLen < 1 {
		errMsg := fmt.Sprintf("No server for %s: %d", coin, coinArrayLen)
		return -1, errors.New(errMsg)
	}

	// Create a private instance of rand.Rand with a custom seed
	seed := time.Now().UnixNano()
	source := rand.NewSource(seed)
	rng := rand.New(source)

	// Get a random index within the range of the coinArray
	randomIndex := rng.Intn(coinArrayLen)

	// Pick the element at the random index
	randomValidServerID := coinArray[randomIndex].GetInt()

	return randomValidServerID, nil
}

// UpdateAllServersData fetches the latest data (ping, heights, fees) from all registered servers concurrently.
// After all servers have been updated, it calculates the global consensus for heights and fees.
func (servers *Servers) UpdateAllServersData(wg *sync.WaitGroup) {
	// Use a wait group to wait for all goroutines to finish.
	// Iterate over all servers in the servers slice
	for i := range servers.Slice {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			server := servers.Slice[index]

			err := server.server_GetPing()
			if err != nil {
				logger.Printf("[server%d]_error   : %v", server.id, err)
			}
			if server.ping == 1 {
				startTimer := time.Now()
				err := server.server_GetHeights()
				elapsedTimer := time.Since(startTimer)
				if err != nil {
					logger.Printf("[server%d]_error getting heights: %v", server.id, err)
				}
				err = server.server_GetFees()
				if err != nil {
					logger.Printf("[server%d]_error getting fees: %v", server.id, err)
				}
				logger.Printf("[server%d]_Heights : %v %v", server.id, server.getheights, elapsedTimer)
			}
		}(i)
	}
	wg.Wait()

	// After all servers are updated, calculate the global consensus state.
	servers.updateCoinDataForAllServers()
	servers.UpdateGlobalFees()
	servers.UpdateGlobalHeights()

	logger.Printf("|SERVERS|_Heights : %v", servers.GlobalHeights)
	logger.Printf("|SERVERS|_Fees    : %v", servers.GlobalFees)
	logger.Printf("|SERVERS|_Srv_IDs : %v", servers.GlobalCoinServerIDs)
}

// buildCoinHeightsMap extracts block heights for each coin from all healthy servers
// and returns a map where each key is a coin ticker and the value is a slice of heights.
func buildCoinHeightsMap(servers *Servers) (map[string][]int, error) {
	heightsMap := make(map[string][]int)

	for _, server := range servers.Slice {
		for coin, coinObj := range server.coinsMap {
			// Only include data from healthy coins (valid height and acceptable time diff).
			if coinObj.getBlockCount > 0 && coinObj.timeDiff != -1000 {
				heightsMap[coin] = append(heightsMap[coin], coinObj.getBlockCount)
			}
		}
	}
	return heightsMap, nil
}

// CollectBlockHashVotesForConsensusHeight gathers block hash "votes" from each server
// for the globally agreed-upon consensus height of each coin. If a server does not have
// the hash for the consensus height cached, it will attempt to fetch it.
// This function has a side-effect: it populates the `hashesStorage` for servers.
// It returns a map where keys are coin tickers, and values are maps of block hashes to
// the list of server IDs that reported that hash.
// e.g., {"BTC": {"0000...abc": [1, 3], "0000...def": [2]}}
func CollectBlockHashVotesForConsensusHeight(servers *Servers) map[string]map[string][]int {
	votes := make(map[string]map[string][]int)
	globalCoinServerIDs, err := servers.GlobalCoinServerIDs.Object()
	if err != nil || globalCoinServerIDs == nil {
		return votes
	}

	globalHeightsResult := servers.GlobalHeights.Get("result")
	if globalHeightsResult == nil {
		return votes
	}

	globalCoinServerIDs.Visit(func(coinBytes []byte, coinValue *fastjson.Value) {
		coin := string(coinBytes)
		consensusHeightValue := globalHeightsResult.Get(coin)
		if consensusHeightValue == nil {
			return // No consensus height for this coin.
		}
		consensusHeight, err := consensusHeightValue.Int()
		if err != nil {
			return // Invalid height format.
		}

		serverIDsValue := coinValue.Get("ids")
		if serverIDsValue == nil {
			return
		}
		serverIDs, err := serverIDsValue.Array()
		if err != nil {
			return
		}

		for _, idValue := range serverIDs {
			serverID, _ := idValue.Int()
			server, exists := servers.GetServerByID(serverID)
			if !exists {
				logger.Printf("Cannot find server with ID %d for coin %s", serverID, coin)
				continue
			}

			// If no map for the coin, can't have a hash.
			if server.hashesStorage[coin] == nil {
				continue
			}

			// Fetch and cache the hash if it's missing or empty for the consensus height.
			// This preserves the original logic of re-fetching if a previous attempt failed and stored "".
			if server.hashesStorage[coin][consensusHeight] == "" {
				hash, err := server.server_GetBlockHash(coin, consensusHeight)
				if err != nil {
					logger.Printf("CollectBlockHashVotes: error from server_GetBlockHash for server %d, coin %s: %v", server.id, coin, err)
					// Store an empty string to prevent re-fetching on this cycle, but it won't be counted as a vote.
					hash = ""
				}
				server.hashesStorage[coin][consensusHeight] = hash
			}

			// If a valid hash exists, record the vote.
			if hash := server.hashesStorage[coin][consensusHeight]; hash != "" {
				if votes[coin] == nil {
					votes[coin] = make(map[string][]int)
				}
				votes[coin][hash] = append(votes[coin][hash], server.id)
			}
		}
	})

	return votes
}

// DetermineConsensusHashes analyzes the collected block hash votes to find the consensus hash
// for each coin. A consensus is reached if a single hash is reported by a ratio of servers
// that meets or exceeds `config.ConsensusThreshold`.
// Note: Since map iteration order is not guaranteed, if multiple hashes meet the threshold,
// the one that is processed first will be chosen. This behavior is preserved from the original implementation.
func DetermineConsensusHashes(votesPerCoin map[string]map[string][]int) map[string]string {
	consensusHashes := make(map[string]string)

	for coin, votesForHash := range votesPerCoin {
		// Calculate the total number of servers that submitted a valid hash for this coin.
		totalVotes := 0
		for _, serverIDs := range votesForHash {
			totalVotes += len(serverIDs)
		}

		if totalVotes == 0 {
			continue
		}

		// Find the first hash that meets the consensus threshold.
		for hash, serverIDsWithHash := range votesForHash {
			ratio := float64(len(serverIDsWithHash)) / float64(totalVotes)
			if ratio >= config.ConsensusThreshold {
				consensusHashes[coin] = hash
				break // Consensus found for this coin.
			}
		}
	}
	return consensusHashes
}

// FindServersFailingHashConsensus identifies servers that do not have the consensus block hash
// for a given coin. It orchestrates the process of collecting votes, determining consensus,
// and then finding the divergent servers.
// It returns a map where the key is the coin ticker and the value is a slice of
// server IDs that failed the hash consensus check.
func FindServersFailingHashConsensus(servers *Servers) map[string][]int {
	// 1. Collect block hash votes from all servers for the consensus height of each coin.
	// This step may also involve fetching and caching hashes if they are not already stored.
	votes := CollectBlockHashVotesForConsensusHeight(servers)

	// 2. Determine the consensus hash for each coin based on the votes.
	consensusHashes := DetermineConsensusHashes(votes)

	// 3. Identify servers that do not have the consensus hash in their storage for each coin.
	failingServers := make(map[string][]int)
	globalCoinServerIDs, err := servers.GlobalCoinServerIDs.Object()
	if err != nil || globalCoinServerIDs == nil {
		return failingServers
	}

	globalCoinServerIDs.Visit(func(coinBytes []byte, coinValue *fastjson.Value) {
		coin := string(coinBytes)
		consensusHash, hasConsensus := consensusHashes[coin]
		if !hasConsensus {
			return // No consensus was reached for this coin, so no servers can fail.
		}

		serverIDs, _ := coinValue.Get("ids").Array()
		for _, idValue := range serverIDs {
			serverID, _ := idValue.Int()
			server, exists := servers.GetServerByID(serverID)
			if !exists {
				continue
			}

			// A server fails consensus if the consensus hash is not present in its
			// hash storage for the given coin. Note that this checks against all hashes
			// stored for the coin on that server, not just the hash at the consensus height.
			// This is the original, intended behavior.
			if !isHashPresentInCoinStorage(server.hashesStorage[coin], consensusHash) {
				failingServers[coin] = append(failingServers[coin], server.id)
			}
		}
	})

	return failingServers
}

// isHashPresentInCoinStorage checks if a given hash exists in a server's hash storage for a specific coin.
// Importantly, it checks against all hashes stored for that coin, regardless of block height.
func isHashPresentInCoinStorage(storage map[int]string, hashToFind string) bool {
	for _, storedHash := range storage {
		if storedHash == hashToFind {
			return true
		}
	}
	return false
}

// calculateConsensusFees determines the consensus fee for each coin based on the fees
// reported by all servers. A fee is considered consensus if it's reported by a
// sufficient ratio of servers (defined by `config.ConsensusThreshold`).
func calculateConsensusFees(counts map[string]map[string]int) *fastjson.Value {
	consensus := fastjson.MustParse(`{"result": {}, "error": null}`)

	// Extract the keys from the "result" object
	keys := make([]string, 0, len(counts))
	for coin := range counts {
		keys = append(keys, coin)
	}

	// Sort the keys in alphabetical order
	sort.Strings(keys)

	// Iterate over the sorted keys and update the "result" object
	for _, coin := range keys {
		elementCounts := counts[coin]
		totalServers := 0
		for _, count := range elementCounts {
			totalServers += count
		}

		for identifier, count := range elementCounts {
			threshold := float64(count) / float64(totalServers)
			if threshold >= config.ConsensusThreshold {
				parts := strings.Split(identifier, ":")
				valueParsed, _ := fastjson.Parse(parts[1])
				consensus.Get("result").Set(parts[0], valueParsed)
			}
		}
	}

	return consensus
}

// createGlobalHeightsJSON creates a `fastjson.Value` object representing the global
// consensus on block heights for all coins. The keys are sorted alphabetically.
func createGlobalHeightsJSON(mostCommonHeightsRanges map[string][]int) (*fastjson.Value, error) {
	// Create a new JSON arena
	arena := &fastjson.Arena{}
	// Create the result object
	resultObj := arena.NewObject()
	for coin, heights := range mostCommonHeightsRanges {
		minHeight := heights[0]
		for _, height := range heights {
			if height < minHeight {
				minHeight = height
			}
		}
		resultObj.Set(coin, arena.NewNumberInt(minHeight))
	}
	// Get the keys from the resultObj
	keys := make([]string, 0, len(mostCommonHeightsRanges))
	for key := range mostCommonHeightsRanges {
		keys = append(keys, key)
	}
	// Sort the keys
	sort.Strings(keys)
	// Create the sorted result object
	sortedResultObj := arena.NewObject()
	for _, key := range keys {
		value := resultObj.Get(key)
		sortedResultObj.Set(key, value)
	}
	// Create the main JSON object
	jsonObj := arena.NewObject()
	jsonObj.Set("result", sortedResultObj)
	jsonObj.Set("error", arena.NewNull())
	return jsonObj, nil
}

// createCommonHeightServersJSON creates a `fastjson.Value` object that maps each coin
// to a list of server IDs that are in consensus for that coin's block height.
func createCommonHeightServersJSON(commonHeightServers map[string][]int) (*fastjson.Value, error) {
	// Create a new JSON object
	jsonObj := make(map[string]interface{})
	// Iterate over the map
	for key, values := range commonHeightServers {
		// Create a new JSON object for each key
		obj := make(map[string]interface{})
		obj["ids"] = values
		// Add the object to the main JSON object
		jsonObj[key] = obj
	}
	// Convert the JSON object to bytes
	jsonBytes, err := json.Marshal(jsonObj)
	if err != nil {
		return nil, err
	}
	// Create a new fastjson.Parser
	parser := &fastjson.Parser{}
	// Parse the bytes into a fastjson.Value
	value, err := parser.ParseBytes(jsonBytes)
	if err != nil {
		return nil, err
	}

	return value, nil
}

// calculateRatio computes the ratio of servers that agree on the most common height range
// for each coin. This is used to determine if a height consensus has been reached.
func calculateRatio(heightsMap map[string][]int, mostCommonHeightsRanges map[string][]int) map[string]float64 {
	ratioMap := make(map[string]float64)
	for key := range heightsMap {
		heightsLen := len(heightsMap[key])
		mostCommonLen := len(mostCommonHeightsRanges[key])

		ratio := float64(mostCommonLen) / float64(heightsLen)
		// Min ratio: config.ConsensusThreshold = 0.66 = 66.67 % servers agreeing on range
		if ratio >= config.ConsensusThreshold {
			ratioMap[key] = ratio
		} else {
			ratioMap[key] = 0
		}
	}
	return ratioMap
}

// findServersInConsensusHeightRange identifies which servers have a reported block height that
// falls within the consensus range for each coin.
func findServersInConsensusHeightRange(servers *Servers, mostCommonHeightsRanges map[string][]int, ratioMap map[string]float64) map[string][]int {
	commonHeightServers := make(map[string][]int)
	for _, server := range servers.Slice {
		for coin, coinObj := range server.coinsMap {
			if heights, ok := mostCommonHeightsRanges[coin]; ok {
				if slices.Contains(heights, coinObj.getBlockCount) {
					// Only include servers for coins that have reached consensus.
					if ratioMap[coin] >= config.ConsensusThreshold {
						commonHeightServers[coin] = append(commonHeightServers[coin], server.id)
					}
				}
			}
		}
	}
	return commonHeightServers
}

// computeMostCommonHeightRanges calculates the most frequently occurring range of block heights
// for each coin across all servers. A small tolerance is allowed to account for minor discrepancies.
func computeMostCommonHeightRanges(heightsMap map[string][]int) map[string][]int {
	mostCommonRanges := make(map[string][]int)
	for coinStr, heights := range heightsMap {
		sort.Ints(heights)
		counts := make(map[int]int)
		for _, value := range heights {
			counts[value]++
		}
		var mostCommonRange []int
		var mostCommonCount int
		for _, height := range heights {
			// Define a tolerance range around the current height.
			rangeStart := height - 5
			rangeEnd := height + 5
			rangeCount := countRangeValues(counts, rangeStart, rangeEnd)
			if rangeCount > mostCommonCount || (rangeCount == mostCommonCount && height > mostCommonRange[len(mostCommonRange)-1]) {
				mostCommonRange = getValuesInRange(heights, rangeStart, rangeEnd)
				mostCommonCount = rangeCount
			}
		}
		if len(mostCommonRange) > 0 {
			mostCommonRanges[coinStr] = mostCommonRange
		}
	}
	return mostCommonRanges
}

// countRangeValues is a helper to count how many values in a map fall within a given range.
func countRangeValues(counts map[int]int, start, end int) int {
	count := 0
	for i := start; i <= end; i++ {
		count += counts[i]
	}
	return count
}

// getValuesInRange is a helper to extract values from a slice that fall within a given range.
func getValuesInRange(values []int, start, end int) []int {
	var result []int
	for _, value := range values {
		if value >= start && value <= end {
			result = append(result, value)
		}
	}
	return result
}

// sortGlobalCoinServerIDs sorts the provided `fastjson.Value` object (which maps coins to server IDs)
// alphabetically by coin ticker.
func sortGlobalCoinServerIDs(coinsmapids *fastjson.Value) (*fastjson.Value, error) {
	obj, err := coinsmapids.Object()
	if err != nil {
		return nil, fmt.Errorf("failed to get object from fastjson value: %w", err)
	}

	keys := make([]string, 0, obj.Len())
	obj.Visit(func(key []byte, v *fastjson.Value) {
		keys = append(keys, string(key))
	})

	sort.Strings(keys)

	// Create a new sorted object.
	var p fastjson.Parser
	sortedVal, err := p.Parse(`{}`)
	if err != nil {
		return nil, err // Should not happen with a static string.
	}
	sortedObj := sortedVal.GetObject()

	for _, key := range keys {
		sortedObj.Set(key, obj.Get(key))
	}

	return sortedVal, nil
}
