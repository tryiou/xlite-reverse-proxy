// consensus & health checks calculations on servers data

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

// Add server to the map
func (servers *Servers) serverAdd(s *Server) int {
	if servers.Slice == nil {
		servers.Slice = []*Server{}
	}
	// Allocate the id field
	s.id = len(servers.Slice) + 1
	//logger.Print("s.id = ", s.id)

	servers.Slice = append(servers.Slice, s)
	//servers.Map[s.id] = *s
	s.getfees = fastjson.MustParse(`{"result": null, "error": null}`)
	s.getheights = fastjson.MustParse(`{"result": null, "error": null}`)
	s.coinsMap = make(map[string]Coin)
	s.hashesStorage = make(map[string]map[int]string)
	return s.id
}

// serverRemove removes the specified server from the Servers struct
func (servers *Servers) serverRemove(server *Server) {
	for i, srv := range servers.Slice {
		if srv == server {
			mu.Lock()
			servers.Slice = append(servers.Slice[:i], servers.Slice[i+1:]...)
			mu.Unlock()
			break
		}
	}
}

// Get server by id
func (servers *Servers) serverGet(id int) (Server, bool) {
	for _, server := range servers.Slice {
		if server.id == id {
			return *server, true
		}
	}
	return Server{}, false
}

// Get server by URL
func (servers *Servers) serverGetByURL(url string) (Server, bool) {
	for _, server := range servers.Slice {
		if server.url == url {
			return *server, true
		}
	}
	return Server{}, false
}

// update servers.g_getfees with every server.getfees data, working out consensus on answers
// servers.g_getfees will be used as cache and be answered to any client requesting 'fees'
func (servers *Servers) getGlobalFees() {
	counts := make(map[string]map[string]int)

	for _, server := range servers.Slice {
		for coin, coinObj := range server.coinsMap {
			//fmt.Println(reflect.TypeOf(coinObj.fee), coinObj.fee)
			if coinObj.fee == 0 {
				continue
			}
			valueTruncated := math.Floor(coinObj.fee*1e8) / 1e8
			valueStr := strconv.FormatFloat(valueTruncated, 'f', -1, 64)
			identifier := coin + ":" + valueStr
			if _, ok := counts[coin]; !ok {
				counts[coin] = make(map[string]int)
			}
			counts[coin][identifier]++
		}
	}
	consensus := calculateConsensusFees(counts)
	servers.g_getfees = consensus
	//logger.Print("getGlobalFees, servers.g_getfees servers.g_getfees = ", servers.g_getfees)
}

// update every server server.coinsMap
func (servers *Servers) updatesCoinsMaps() error {
	for _, server := range servers.Slice {
		heightsObj := server.getheights.Get("result")
		//logger.Print("heightsObj = ", heightsObj)
		if heightsObj == nil {
			// "result": "null"
			server.coinsMap = make(map[string]Coin)
		} else {
			if heightsObj.Type() == fastjson.TypeObject {
				obj, err := heightsObj.Object()
				if err == nil {
					obj.Visit(func(coin []byte, height *fastjson.Value) {
						coinStr := string(coin)
						heightInt := -1
						if height.String() != "null" {
							heightInt, err = height.Int()
							if err != nil {
								logger.Print("updatesCoinsMaps error height.Int()")
								heightInt = -1
							}
						}
						coinMap := server.coinsMap[coinStr]
						if coinMap.getBlockCount != heightInt {
							coinMap.getBlockCount = heightInt
							if heightInt > 0 {
								getBlockHash, err := server.server_GetBlockHash(coinStr, heightInt)
								if err != nil {
									logger.Printf("[server%d] server_GetBlockHash failed, err: %v", server.id, err)
								}
								if getBlockHash != "" {
									blockCacheKey := fmt.Sprintf("%s_%s", coinStr, getBlockHash)
									blockDataCache := blockCache[blockCacheKey]
									if blockDataCache == nil {
										// Fetch the block data if not available in the cache.
										getBlock, err := server.server_GetBlock(coinStr, getBlockHash)
										if err != nil {
											logger.Printf("[server%d] server_GetBlock failed, err: %v", server.id, err)
										} else {
											timestamp, err := getBlock.Get("time").Int64()
											if err != nil {
												logger.Printf("[server%d] getBlock.Get('time').Int64() failed, err: %v", server.id, err)
												// Handle error
											} else {
												// Convert timestamp to time.Time object
												blockTime := time.Unix(timestamp, 0)

												// Get current desktop time
												desktopTime := time.Now()

												// Calculate the time difference in seconds
												timeDifference := desktopTime.Sub(blockTime).Seconds()
												logger.Printf("%s, %d, Time difference between block time and desktop time: %.2f seconds", coinStr, server.id, timeDifference)

												// Update the cache with the new block data for the latest block only.
												blockCache[blockCacheKey] = &BlockCache{
													BlockHash: getBlockHash,
													timeDiff:  timeDifference,
												}
												purgeCache(blockCache, config.MaxStoredBlocks)

											}
										}
									}

								}

								// exclude coin with too high timeDiff between blocktime and actual desktop time
								blockCacheKey := fmt.Sprintf("%s_%s", coinStr, getBlockHash)
								blockDataCache := blockCache[blockCacheKey]
								if blockDataCache != nil {
									if blockDataCache.timeDiff < float64(config.MaxBlockTimeDiff) {
										coinMap.getBlockHash = getBlockHash
										coinMap.timeDiff = blockDataCache.timeDiff
									} else {
										coinMap.getBlockHash = blockDataCache.BlockHash
										coinMap.timeDiff = -1000
										logger.Printf("[server%d] timeDiff too high: %f %d", server.id, blockDataCache.timeDiff, heightInt)
									}
								} else {
									coinMap.getBlockHash = ""
									coinMap.timeDiff = -1000
								}

							} else {
								coinMap.getBlockHash = ""
								coinMap.timeDiff = -1000
							}
							server.coinsMap[coinStr] = coinMap

							//logger.Printf("server.coinsMap[%s] :%v", coinStr, server.coinsMap[coinStr])
							if server.coinsMap[coinStr].getBlockHash != "" {
								if server.hashesStorage[coinStr] == nil {
									server.hashesStorage[coinStr] = make(map[int]string)
								}
								server.hashesStorage[coinStr][heightInt] = server.coinsMap[coinStr].getBlockHash
							}
							logger.Printf("[server%d]_New_Block %-5s: %-9d:%s\n", server.id, coinStr, server.coinsMap[coinStr].getBlockCount, server.coinsMap[coinStr].getBlockHash)
						}
					})
				} else {
					logger.Printf("[server%d]_updatesCoinsMaps, error with heights object: %v", server.id, err)
					server.coinsMap = make(map[string]Coin)
					return err
				}
			} else {
				//logger.Printf("[server%d] lost contact with this server // NULL answer", server.id)
				//logger.Print("heightsObj.Type() = ", heightsObj.Type())
				server.coinsMap = make(map[string]Coin)
			}
			feesObj := server.getfees.Get("result")
			if feesObj != nil && feesObj.Type() == fastjson.TypeObject {
				obj, err := feesObj.Object()
				if err == nil {
					obj.Visit(func(coin []byte, fee *fastjson.Value) {
						coinStr := string(coin)
						if fee != nil && fee.Type() == fastjson.TypeNumber {
							feeFloat, err := fee.Float64()
							if err == nil {
								coinMap := server.coinsMap[coinStr]
								coinMap.fee = feeFloat
								server.coinsMap[coinStr] = coinMap
								server.pruneHashStorage()
							}
						}
					})
				} else {
					logger.Printf("updatesCoinsMaps, error with fees object: %v", err)
					return err
				}
			}
		}
	}
	return nil
}

// servers.g_getheights will be be answered to any client requesting 'height'/'heights'
func (servers *Servers) getGlobalHeights() {
	heightsMap, err := BuildsHeightsMap(servers)
	if err != nil {
		logger.Printf(" error extracting results: %v", err)
		// Set g_getheights to empty value
		servers.g_getheights = nil
		servers.g_coinsServersIDs = nil
		// Handle the error according to your requirements
		return
	}
	//logger.Print("heightsMap", heightsMap)
	mostCommonHeightsRanges := computeMostCommonHeightsRanges(heightsMap)
	//logger.Print("mostCommonHeightsRanges:", mostCommonHeightsRanges)
	ratioMap := calculateRatio(heightsMap, mostCommonHeightsRanges)
	//	logger.Print("ratioMap:", ratioMap)
	commonHeightServers := findCommonHeightServers(servers, mostCommonHeightsRanges, ratioMap)
	//	logger.Print("commonHeightServers:", commonHeightServers)
	jsonCommonHeightServers, err := createFastJSONCommonHeightsServers(commonHeightServers)
	//	logger.Print("jsonCommonHeightServers:", jsonCommonHeightServers)
	if err != nil {
		logger.Printf(" error creating JSON for common height servers: %v", err)
		servers.g_getheights = nil
		servers.g_coinsServersIDs = nil
		return
	}
	jsonGGetHeights, err := createFastJSONGGetheights(mostCommonHeightsRanges)
	//	logger.Print("jsonGGetHeights:", jsonGGetHeights)
	if err != nil {
		logger.Printf(" error creating JSON for GGet heights: %v", err)
		servers.g_getheights = nil
		servers.g_coinsServersIDs = nil
		return
	}
	servers.g_getheights = jsonGGetHeights
	sortedJSON, err := sortG_coinsidsMap(jsonCommonHeightServers)
	if err != nil {
		logger.Printf(" error sorting coins map: %v", err)
		servers.g_coinsServersIDs = nil
	}
	servers.g_coinsServersIDs = sortedJSON

	servers.updatesCoinsMaps()

	nonConsensusServersMap := findNonConsensusGetBlockHashes(servers)
	if len(nonConsensusServersMap) > 0 {
		logger.Printf("getGlobalHeights, nonConsensusServersMap = %v", nonConsensusServersMap)

		servers.updateGCoinsIDs(nonConsensusServersMap)
	}
	/*// TESTING SET!!
	// JSON string representing the initial value of g_coinsids
	jsonStr := `{
			"BLOCK": {"ids": [1, 2, 3]},
			"BTC": {"ids": [2, 3]},
			"DOGE": {"ids": [1, 2, 3]},
			"LTC": {"ids": [1, 2, 3]}
		}`

	// Parse the JSON string to create a fastjson.Value
	parsedValue, err := fastjson.Parse(jsonStr)
	if err != nil {
		fmt.Println("Error parsing JSON:", err)
		return
	}
	nonConsensusServersMapTest := map[string][]int{
		"BLOCK": {1},
		"BTC":   {},
		"DOGE":  {2},
		"LTC":   {3},
	}
	servers.g_coinsids = parsedValue
	//fmt.Printf("FIRST, servers.g_coinsids = %v\n", servers.g_coinsids)
	//fmt.Printf("nonConsensusServersMapTest = %v\n", nonConsensusServersMapTest)
	// TESTING SET!!*/
}

func (servers *Servers) removeIDFromCoinsServersIDs(coin string, id int) {
	gCoinsIDs := servers.g_coinsServersIDs
	if gCoinsIDs != nil {
		coinInfo := gCoinsIDs.Get(coin)
		if coinInfo != nil {
			ids := coinInfo.Get("ids")
			if ids != nil && ids.Type() == fastjson.TypeArray {
				updatedIDs := make([]int, 0)
				idsArray := ids.GetArray()
				for _, idValue := range idsArray {
					if idValue.Type() == fastjson.TypeNumber {
						serverID := int(idValue.GetInt())
						if serverID != id {
							updatedIDs = append(updatedIDs, serverID)
						}
					}
				}
				// Create a new fastjson.Array with the updated IDs
				newIDs := fastjson.MustParse("[]")
				for _, updatedID := range updatedIDs {
					newIDs.SetArrayItem(len(newIDs.GetArray()), fastjson.MustParse(strconv.Itoa(updatedID)))
				}
				// Create a new coinInfo object with the updated IDs
				newCoinInfo := fastjson.MustParse(fmt.Sprintf(`{"ids": %s}`, newIDs.String()))
				// Update the gCoinsIDs object with the new coinInfo
				mu.Lock()
				gCoinsIDs.Set(coin, newCoinInfo)
				mu.Unlock()
				logger.Printf("Removed ID %d from coin '%s' IDs: %v\n", id, coin, updatedIDs)
			}
		}
	}
}

// remove any non-consensus server id from g_coinsids
func (servers *Servers) updateGCoinsIDs(nonConsensusMap map[string][]int) {
	gCoinsIDs := servers.g_coinsServersIDs
	for coin, serverIDs := range nonConsensusMap {
		coinInfo := gCoinsIDs.Get(coin)
		if coinInfo != nil {
			ids := coinInfo.Get("ids")
			if ids != nil && ids.Type() == fastjson.TypeArray {
				updatedIDs := make([]*fastjson.Value, 0)
				idsArray := ids.GetArray()
				for i := range idsArray {
					id := idsArray[i]
					if id.Type() == fastjson.TypeNumber {
						serverID := int(id.GetInt())
						if !containsID(serverIDs, serverID) {
							updatedIDs = append(updatedIDs, id)
						}
					}
				}
				// Create a new array without the non-consensus server IDs
				newIDs := make([]int, 0)
				for _, id := range updatedIDs {
					if id.Type() == fastjson.TypeNumber {
						newIDs = append(newIDs, int(id.GetInt()))
					}
				}
				// Marshal the new array to a JSON string
				newIDsJSON, _ := json.Marshal(newIDs)
				// Parse the JSON string to create a new fastjson.Value
				newIDsValue, _ := fastjson.ParseBytes(newIDsJSON)
				// Set the new array as the value of the "ids" field
				mu.Lock()
				coinInfo.Set("ids", newIDsValue)
				mu.Unlock()
				logger.Printf("Updated 'ids' array for coin: %s %v\n", coin, newIDsValue)
				// delete the non-consensus coin(s) from each server.coinMap
				for _, serverID := range serverIDs {
					if server, exists := servers.serverGet(serverID); exists {
						logger.Printf("[server%d] Removing %s from coinMap\n", server.id, coin)
						delete(server.coinsMap, coin)
					}
				}
			}
		}
	}
}

func (servers *Servers) getRandomValidServerId(coin string) (int, error) {
	coinObj := servers.g_coinsServersIDs.GetObject(coin)
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
	//logger.Print("rp servers.g_coinsids:", servers.g_coinsids, " coinObj: ", coinObj, "\n")
	//os.Exit(1)
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

func (servers *Servers) updateServersData(wg *sync.WaitGroup) {
	// Use a wait group to wait for all goroutines to finish

	// Iterate over all servers in the servers slice
	for i := range servers.Slice {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			server := servers.Slice[index]

			err := server.server_GetPing()
			if err != nil {
				logger.Printf("[server%d]_error   : %v", server.id, err)
				// Handle the error as needed
			}
			if server.ping == 1 {
				startTimer := time.Now()
				err := server.server_GetHeights()
				elapsedTimer := time.Since(startTimer)
				if err != nil {
					logger.Printf("[server%d]_error   : %v", server.id, err)
					// Handle the error as needed
				}
				//startTimer2 := time.Now()
				err = server.server_GetFees()
				//elapsedTimer2 := time.Since(startTimer2)
				if err != nil {
					logger.Printf("[server%d]_error   : %v", server.id, err)
					// Handle the error as needed
				}
				logger.Printf("[server%d]_Heights : %v %v", server.id, server.getheights, elapsedTimer)
				//fmt.Printf("  timer: %s\n", elapsedTimer)
				//logger.Printf("[server%d]_GetFees     :\n %v", server.id, server.getfees)
				//logger.Printf("  timer: %s\n", elapsedTimer2)
			}
		}(i)
	}
	wg.Wait()

	// Update the global heights and fees after all servers are updated
	servers.updatesCoinsMaps()

	servers.getGlobalFees()
	servers.getGlobalHeights()
	// Print the updated values (optional)               {"BLOCK":{
	logger.Printf("|SERVERS|_Heights : %v", servers.g_getheights)
	logger.Printf("|SERVERS|_Fees    : %v", servers.g_getfees)
	logger.Printf("|SERVERS|_Srv_IDs : %v", servers.g_coinsServersIDs)

}

func purgeCache(blockCache map[string]*BlockCache, maxStoredBlocks int) {
	// Create a map to keep track of the number of entries for each coin.
	coinCountMap := make(map[string]int)

	// Iterate through the blockCache to count the number of entries for each coin.
	for _, v := range blockCache {
		coinCountMap[v.BlockHash]++
	}

	// If the number of entries for a coin exceeds maxStoredBlocks, remove the oldest entries until only maxStoredBlocks are left.
	for coinHash, count := range coinCountMap {
		if count > maxStoredBlocks {
			var oldestEntriesToDelete = count - maxStoredBlocks
			for k, v := range blockCache {
				if v.BlockHash == coinHash {
					delete(blockCache, k)
					oldestEntriesToDelete--
					if oldestEntriesToDelete == 0 {
						break
					}
				}
			}
		}
	}
}

func removeNonPrintableChars(s string) string {
	var result []rune
	for _, c := range s {
		if c >= 32 && c <= 126 && c != '\\' && c != '"' && c != '\x00' {
			result = append(result, c)
		}
	}
	return string(result)
}

// extract data from heights json result per server, return a map[coin] with each server getBlockCount
func BuildsHeightsMap(servers *Servers) (map[string][]int, error) {
	heightsMap := make(map[string][]int)
	// sample :map[BLOCK:[3059452 3059452 3059452] BTC:[793349 793349]]
	for _, server := range servers.Slice {
		for coin, coinObj := range server.coinsMap {
			if coinObj.getBlockCount > 0 && coinObj.timeDiff != -1000 {
				heightsMap[coin] = append(heightsMap[coin], coinObj.getBlockCount)
			}
		}
	}
	return heightsMap, nil
}

// compare every server blockhashes for the last getblockcount of each coins,
// procude a non-consensus map with every server id to remove if any, per coin.
// return nonConsensusServersMap
func findNonConsensusGetBlockHashes(servers *Servers) map[string][]int {
	consensusMap := make(map[string]string)
	nonConsensusMap := make(map[string]string)
	consensusServersMap := make(map[string][]int)
	consensusCounts := make(map[string]map[string][]int)
	nonConsensusServersMap := make(map[string][]int)
	// Step 1: Collect getBlockHash values for mostComonHeight, for each coins from each server
	gCoinsIDsObj := servers.g_coinsServersIDs.GetObject()
	coins := make([]string, 0)
	gCoinsIDsObj.Visit(func(key []byte, v *fastjson.Value) {
		coins = append(coins, string(key))
	})
	for _, coin := range coins {
		idsJson := gCoinsIDsObj.Get(coin).Get("ids")
		// Retrieve the array of values and the error
		idsArray, err := idsJson.Array()
		if err != nil {
			// Handle the error
			fmt.Printf("Error occurred while retrieving ids array: %v\n", err)
			continue
		}
		// Iterate over the ids array
		for _, id := range idsArray {
			server, exist := servers.serverGet(id.GetInt())
			if !exist {
				logger.Printf("Can't find server %v", id)
			}
			json := servers.g_getheights.Get("result").GetObject()
			commonHeightJson := json.Get(coin)
			if commonHeightJson == nil {
				continue
			}

			commonHeight := commonHeightJson.GetInt()
			if server.hashesStorage[coin] == nil {
				continue
			}
			if server.hashesStorage[coin][commonHeight] == "" {
				getBlockHash, err := server.server_GetBlockHash(coin, commonHeight)
				if err != nil {
					logger.Printf("findConsensusGetBlockHashes, error with server_GetBlockHash: %v", err)
					getBlockHash = ""
				}
				server.hashesStorage[coin][commonHeight] = getBlockHash
			}
			if server.hashesStorage[coin][commonHeight] != "" {
				if consensusCounts[coin] == nil {
					consensusCounts[coin] = make(map[string][]int)
				}
				hash := server.hashesStorage[coin][commonHeight]
				consensusCounts[coin][hash] = append(consensusCounts[coin][hash], server.id)
			}

		}
	}
	// Step 2: Determine the consensus getBlockHash value for each coin
	for coin, counts := range consensusCounts {
		total := 0
		for _, ids := range counts {
			total += len(ids)
		}
		for hash, count := range counts {
			ratio := float64(len(count)) / float64(total)
			//logger.Print("count", count, ratio)
			if ratio >= config.ConsensusThreshold {
				consensusMap[coin] = hash
				//logger.Printf("consensus! %s: %s", coin, hash)
			} else {
				nonConsensusMap[coin] = hash
				if len(nonConsensusMap[coin]) > 0 {
					logger.Printf("Non consensus! %s: %s", coin, hash)
				}
			}
		}
	}
	for _, coin := range coins {
		idsJson := gCoinsIDsObj.Get(coin).Get("ids")
		idsArray, err := idsJson.Array()
		if err != nil {
			// Handle the error
			fmt.Printf("Error occurred while retrieving ids array: %v\n", err)
			continue
		}
		for _, id := range idsArray {
			server, _ := servers.serverGet(id.GetInt())
			if hashExistsInStorage(server.hashesStorage[coin], consensusMap[coin]) {
				// Consensus case: consensusMap[coin] exists in server.hashesStorage[coin]
				consensusServersMap[coin] = append(consensusServersMap[coin], server.id)
			} else if consensusMap[coin] != "" {
				// Non-consensus case: consensusMap[coin] doesn't exist in server.hashesStorage[coin]
				nonConsensusServersMap[coin] = append(nonConsensusServersMap[coin], server.id)
			}
		}
	}
	//logger.Print("nonConsensusServersMap:", nonConsensusServersMap)
	//logger.Printf("consensusMap: %v\n", consensusMap)
	return nonConsensusServersMap
}

func hashExistsInStorage(storage map[int]string, hash string) bool {
	for _, h := range storage {
		if h == hash {
			return true
		}
	}
	return false
}

// Helper function to check if a server ID is present in the slice
func containsID(ids []int, id int) bool {
	return slices.Contains(ids, id)
}

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

func createFastJSONGGetheights(mostCommonHeightsRanges map[string][]int) (*fastjson.Value, error) {
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
	//logger.Print(jsonObj)
	return jsonObj, nil
}

func createFastJSONCommonHeightsServers(commonHeightServers map[string][]int) (*fastjson.Value, error) {
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

func calculateRatio(heightsMap map[string][]int, mostCommonHeightsRanges map[string][]int) map[string]float64 {
	ratioMap := make(map[string]float64)
	for key := range heightsMap {
		heightsLen := len(heightsMap[key])
		mostCommonLen := len(mostCommonHeightsRanges[key])

		ratio := float64(mostCommonLen) / float64(heightsLen)
		// Min ratio: config.ConsensusThreshold = 66.67 % servers agreeing on range
		if ratio >= config.ConsensusThreshold {
			ratioMap[key] = ratio
		} else {
			ratioMap[key] = 0
		}
	}
	return ratioMap
}

func findCommonHeightServers(servers *Servers, mostCommonHeightsRanges map[string][]int, ratioMap map[string]float64) map[string][]int {
	commonHeightServers := make(map[string][]int)
	//logger.Print("commonHeightServers = ", commonHeightServers)
	for _, server := range servers.Slice {
		for coin, coinObj := range server.coinsMap {
			//logger.Print("coin = ", coin, ",coinObj = ", coinObj)
			if heights, ok := mostCommonHeightsRanges[coin]; ok {
				//logger.Print("heights = ", heights, ",ok = ", ok)
				if containsValue(heights, coinObj.getBlockCount) {
					//logger.Print("ratioMap(", coin, ") = ", ratioMap[coin])
					if ratioMap[coin] >= config.ConsensusThreshold {
						commonHeightServers[coin] = append(commonHeightServers[coin], server.id)
					}
				}
			}
		}
	}
	//logger.Print("commonHeightServers = ", commonHeightServers)
	return commonHeightServers
}

func containsValue(values []int, value int) bool {
	return slices.Contains(values, value)
}

func computeMostCommonHeightsRanges(heightsMap map[string][]int) map[string][]int {
	// heightsMap sample :map[BLOCK:[3059452 3059452 3059452] BTC:[793349 793349]]
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
			// max range tolerance
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

func countRangeValues(counts map[int]int, start, end int) int {
	count := 0
	for i := start; i <= end; i++ {
		count += counts[i]
	}
	return count
}

func getValuesInRange(values []int, start, end int) []int {
	var result []int
	for _, value := range values {
		if value >= start && value <= end {
			result = append(result, value)
		}
	}
	return result
}

func sortG_coinsidsMap(coinsmapids *fastjson.Value) (*fastjson.Value, error) {
	// Extract the key-value pairs from coinsmapids
	coinIDsMap := make(map[string]CoinData)
	err := json.Unmarshal(coinsmapids.MarshalTo([]byte{}), &coinIDsMap)
	if err != nil {
		return nil, err
	}
	// Retrieve the keys from coinsmapids
	keys := make([]string, 0, len(coinIDsMap))
	for key := range coinIDsMap {
		keys = append(keys, key)
	}
	// Sort the keys alphabetically
	sort.Strings(keys)
	// Create a new fastjson.Object to store the sorted key-value pairs
	sortedCoinsMap := fastjson.MustParse("{}").GetObject()
	for _, key := range keys {
		value := coinIDsMap[key]
		idsStr := strings.Join(strings.Fields(fmt.Sprint(value.Ids)), ", ")
		coinObjStr := fmt.Sprintf(`{"ids": %s}`, idsStr)
		coinObj := fastjson.MustParse(coinObjStr)
		sortedCoinsMap.Set(key, coinObj)
	}
	// Convert the sortedCoinsMap to a *fastjson.Value
	sortedCoinsMapValue, err := fastjson.Parse(sortedCoinsMap.String())
	if err != nil {
		return nil, err
	}
	return sortedCoinsMapValue, nil
}
