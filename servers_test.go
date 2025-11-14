package main

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fastjson"
)

func TestServersUpdateGlobalFeesConsensus(t *testing.T) {
	defer recordTestResult("TestServersUpdateGlobalFeesConsensus", t.Failed())
	log.Printf("TEST_UNIT: Starting TestServersUpdateGlobalFeesConsensus (threshold=%.16f)", 0.6666666666666666)
	defer log.Printf("TEST_UNIT: Finished TestServersUpdateGlobalFeesConsensus")

	oldConfig := config
	defer func() { config = oldConfig }()
	config = &Config{ConsensusThreshold: 0.6666666666666666}

	servers := &Servers{Slice: []*Server{
		{
			id: 1,
			coinsMap: map[string]Coin{
				"BTC": {fee: 0.00001},
				"LTC": {fee: 0.00002},
			},
		},
		{
			id: 2,
			coinsMap: map[string]Coin{
				"BTC": {fee: 0.00001},
				"LTC": {fee: 0.00002},
			},
		},
		{
			id: 3,
			coinsMap: map[string]Coin{
				"BTC": {fee: 0.00002}, // Should not reach consensus
				"LTC": {fee: 0.00003},
			},
		},
	}}

	servers.UpdateGlobalFees()
	if servers.GlobalFees == nil {
		log.Printf("TEST_UNIT: Updated global fees is nil")
	} else {
		log.Printf("TEST_UNIT: Updated global fees: %s", servers.GlobalFees.String())
	}

	// Check BTC consensus: 2/3 = 66.6666% which meets threshold
	btc := servers.GlobalFees.Get("result", "BTC")
	log.Printf("TEST_UNIT: BTC consensus: %.8f", btc.GetFloat64())
	assert.Equal(t, 0.00001, btc.GetFloat64(), "BTC fee should be 0.00001")

	// Check LTC consensus: 2/3 = 66.6666% which meets threshold
	ltc := servers.GlobalFees.Get("result", "LTC")
	log.Printf("TEST_UNIT: LTC consensus: %.8f", ltc.GetFloat64())
	assert.Equal(t, 0.00002, ltc.GetFloat64(), "LTC fee should be 0.00002")
}

func TestServersUpdateGlobalFees_4servers(t *testing.T) {
	defer recordTestResult("TestServersUpdateGlobalFees_4servers", t.Failed())
	log.Printf("TEST_UNIT: Starting TestServersUpdateGlobalFees_4servers (threshold=%.16f)", 0.6666666666666666)
	defer log.Printf("TEST_UNIT: Finished TestServersUpdateGlobalFees_4servers")

	oldConfig := config
	defer func() { config = oldConfig }()
	config = &Config{ConsensusThreshold: 0.6666666666666666}

	servers := &Servers{Slice: []*Server{
		{
			id: 1,
			coinsMap: map[string]Coin{
				"BTC": {fee: 0.00001},
				"LTC": {fee: 0.00002},
			},
		},
		{
			id: 2,
			coinsMap: map[string]Coin{
				"BTC": {fee: 0.00001},
				"LTC": {fee: 0.00002},
			},
		},
		{
			id: 3,
			coinsMap: map[string]Coin{
				"BTC": {fee: 0.00001},
				"LTC": {fee: 0.00003},
			},
		},
		{
			id: 4,
			coinsMap: map[string]Coin{
				"BTC": {fee: 0.00002},
				"LTC": {fee: 0.00002},
			},
		},
	}}

	servers.UpdateGlobalFees()
	log.Printf("TEST_UNIT: Updated global fees: %s", servers.GlobalFees.String())

	// BTC: 3/4 (75%) agree on 0.00001 -> meets threshold
	btc := servers.GlobalFees.Get("result", "BTC")
	log.Printf("TEST_UNIT: BTC consensus: %.8f", btc.GetFloat64())
	assert.Equal(t, 0.00001, btc.GetFloat64(), "BTC fee should be 0.00001")

	// LTC: 3/4 (75%) agree on 0.00002? Actually 2 servers have 0.00002, 1 has 0.00003, 1 has 0.00002 -> total 3/4 (75%) for 0.00002
	ltc := servers.GlobalFees.Get("result", "LTC")
	log.Printf("TEST_UNIT: LTC consensus: %.8f", ltc.GetFloat64())
	assert.Equal(t, 0.00002, ltc.GetFloat64(), "LTC fee should be 0.00002")
}

func TestServersUpdateGlobalFees_10servers(t *testing.T) {
	defer recordTestResult("TestServersUpdateGlobalFees_10servers", t.Failed())
	log.Printf("TEST_UNIT: Starting TestServersUpdateGlobalFees_10servers (threshold=%.16f)", 0.6666666666666666)
	defer log.Printf("TEST_UNIT: Finished TestServersUpdateGlobalFees_10servers")

	oldConfig := config
	defer func() { config = oldConfig }()
	config = &Config{ConsensusThreshold: 0.6666666666666666}

	servers := &Servers{Slice: make([]*Server, 10)}
	for i := 0; i < 10; i++ {
		btcFee := 0.00001
		ltcFee := 0.00002
		if i >= 7 { // 7 servers agree, 3 disagree
			btcFee = 0.00002
		}
		if i < 6 { // 6 servers for LTC consensus, 4 disagree - FIXED: was i < 7
			ltcFee = 0.00002
		} else {
			ltcFee = 0.00003
		}

		servers.Slice[i] = &Server{
			id: i + 1,
			coinsMap: map[string]Coin{
				"BTC": {fee: btcFee},
				"LTC": {fee: ltcFee},
			},
		}
	}

	servers.UpdateGlobalFees()

	btc := servers.GlobalFees.Get("result", "BTC")
	log.Printf("TEST_UNIT: BTC consensus: %.8f", btc.GetFloat64())
	assert.Equal(t, 0.00001, btc.GetFloat64(), "BTC fee should be 0.00001 (7 servers)")

	ltc := servers.GlobalFees.Get("result", "LTC")
	if ltc != nil {
		t.Errorf("LTC should not have reached consensus: %.8f", ltc.GetFloat64())
	} else {
		log.Printf("TEST_UNIT: LTC failed to reach consensus as expected")
	}
}

func TestServersUpdateGlobalHeightsConsensus(t *testing.T) {
	defer recordTestResult("TestServersUpdateGlobalHeightsConsensus", t.Failed())
	log.Printf("TEST_UNIT: Starting TestServersUpdateGlobalHeightsConsensus")
	defer log.Printf("TEST_UNIT: Finished TestServersUpdateGlobalHeightsConsensus")

	oldConfig := config
	defer func() { config = oldConfig }()
	config = &Config{ConsensusThreshold: 0.6666666666666666}

	servers := &Servers{Slice: []*Server{
		{
			id: 1,
			coinsMap: map[string]Coin{
				"BTC": {getBlockCount: 800000},
				"LTC": {getBlockCount: 2000000},
			},
		},
		{
			id: 2,
			coinsMap: map[string]Coin{
				"BTC": {getBlockCount: 800000},
				"LTC": {getBlockCount: 2000002},
			},
		},
		{
			id: 3,
			coinsMap: map[string]Coin{
				"BTC": {getBlockCount: 800005},
				"LTC": {getBlockCount: 2000000},
			},
		},
	}}
	log.Printf("TEST_UNIT: Created test servers with block heights")

	servers.UpdateGlobalHeights()
	if servers.GlobalHeights == nil {
		log.Printf("TEST_UNIT: Updated global heights is nil")
	} else {
		log.Printf("TEST_UNIT: Updated global heights: %s", servers.GlobalHeights.String())
	}
	log.Printf("TEST_UNIT: GlobalCoinServerIDs: %s", servers.GlobalCoinServerIDs.String())

	// BTC heights: 800000, 800000, 800005 - all in common range? Yes, with 3 servers
	btcHeight := servers.GlobalHeights.Get("result", "BTC").GetInt()
	log.Printf("TEST_UNIT: BTC height: %d (expected: %d)", btcHeight, 800000)
	assert.Equal(t, 800000, btcHeight, "BTC height should be consensus minimum")

	// LTC heights: 2000000, 2000002, 2000000 - 2 identical -> ratio 66.67% meets threshold
	ltcHeight := servers.GlobalHeights.Get("result", "LTC").GetInt()
	log.Printf("TEST_UNIT: LTC height: %d (expected: %d)", ltcHeight, 2000000)
	assert.Equal(t, 2000000, ltcHeight, "LTC height should be consensus minimum")

	// Verify which server IDs are included
	idsObj := servers.GlobalCoinServerIDs.Get("BTC").Get("ids")
	idsLen := len(idsObj.GetArray())
	log.Printf("TEST_UNIT: BTC servers count: %d", idsLen)
	assert.Equal(t, 3, idsLen, "Should have 3 servers for BTC")
}

func TestServersUpdateGlobalHeights_4servers(t *testing.T) {
	defer recordTestResult("TestServersUpdateGlobalHeights_4servers", t.Failed())
	log.Printf("TEST_UNIT: Starting TestServersUpdateGlobalHeights_4servers")
	defer log.Printf("TEST_UNIT: Finished TestServersUpdateGlobalHeights_4servers")

	oldConfig := config
	defer func() { config = oldConfig }()
	config = &Config{ConsensusThreshold: 0.6666666666666666}

	servers := &Servers{Slice: []*Server{
		{
			id: 1,
			coinsMap: map[string]Coin{
				"BTC": {getBlockCount: 800000},
				"LTC": {getBlockCount: 2000000},
			},
		},
		{
			id: 2,
			coinsMap: map[string]Coin{
				"BTC": {getBlockCount: 800000},
				"LTC": {getBlockCount: 2000000},
			},
		},
		{
			id: 3,
			coinsMap: map[string]Coin{
				"BTC": {getBlockCount: 800005},
				"LTC": {getBlockCount: 2000000},
			},
		},
		{
			id: 4,
			coinsMap: map[string]Coin{
				"BTC": {getBlockCount: 800005},
				"LTC": {getBlockCount: 2000004},
			},
		},
	}}

	servers.UpdateGlobalHeights()

	// BTC: 4 servers - minimal value 800000
	btcHeight := servers.GlobalHeights.Get("result", "BTC").GetInt()
	log.Printf("TEST_UNIT: BTC height: %d (expected: %d)", btcHeight, 800000)
	assert.Equal(t, 800000, btcHeight, "BTC height should be consensus minimum")

	// LTC: 2000000 x3 and 2000004 -> consensus as min(2000000) is 2000000
	ltcHeight := servers.GlobalHeights.Get("result", "LTC").GetInt()
	log.Printf("TEST_UNIT: LTC height: %d (expected: %d)", ltcHeight, 2000000)
	assert.Equal(t, 2000000, ltcHeight, "LTC height should be consensus minimum")
}

func TestServersUpdateGlobalHeights_10servers(t *testing.T) {
	defer recordTestResult("TestServersUpdateGlobalHeights_10servers", t.Failed())
	log.Printf("TEST_UNIT: Starting TestServersUpdateGlobalHeights_10servers")
	defer log.Printf("TEST_UNIT: Finished TestServersUpdateGlobalHeights_10servers")

	oldConfig := config
	defer func() { config = oldConfig }()
	config = &Config{ConsensusThreshold: 0.6666666666666666}

	servers := &Servers{Slice: make([]*Server, 10)}
	for i := 0; i < 10; i++ {
		btcHeight := 800000
		ltcHeight := 2000000
		if i >= 7 { // 3 servers have different BTC height
			btcHeight = 800005
		}
		if i < 7 { // 7 servers have L2000000, 3 have 2000000
			ltcHeight = 2000000
		} else {
			ltcHeight = 2000004
		}

		servers.Slice[i] = &Server{
			id: i + 1,
			coinsMap: map[string]Coin{
				"BTC": {getBlockCount: btcHeight},
				"LTC": {getBlockCount: ltcHeight},
			},
		}
	}

	servers.UpdateGlobalHeights()

	btcHeight := servers.GlobalHeights.Get("result", "BTC").GetInt()
	log.Printf("TEST_UNIT: BTC height: %d (expected: %d)", btcHeight, 800000)
	assert.Equal(t, 800000, btcHeight, "BTC height should be consensus minimum")

	ltcHeight := servers.GlobalHeights.Get("result", "LTC").GetInt()
	log.Printf("TEST_UNIT: LTC height: %d (expected: %d)", ltcHeight, 2000000)
	assert.Equal(t, 2000000, ltcHeight, "LTC height should be consensus minimum")
}

func TestServersHashConsensusDetection(t *testing.T) {
	defer recordTestResult("TestServersHashConsensusDetection", t.Failed())
	log.Printf("TEST_UNIT: Starting TestServersHashConsensusDetection")
	defer log.Printf("TEST_UNIT: Finished TestServersHashConsensusDetection")

	oldConfig := config
	defer func() { config = oldConfig }()
	config = &Config{ConsensusThreshold: 0.6666666666666666}

	// Initialize global block cache with BTC block 800000
	blockCache["BTC_consensus_hash"] = &BlockCache{
		BlockHash: "0000...abc",
		timeDiff:  15,
		cachedAt:  time.Now(),
	}
	log.Printf("TEST_UNIT: Set up block cache for BTC block 800000")

	servers := &Servers{Slice: []*Server{
		{
			id:       1,
			coinsMap: map[string]Coin{},
			hashesStorage: map[string]map[int]string{
				"BTC": {
					800000: "0000...abc", // Consensus
				},
			},
		},
		{
			id:       2,
			coinsMap: map[string]Coin{},
			hashesStorage: map[string]map[int]string{
				"BTC": {
					800000: "0000...def", // Non-consensus
				},
			},
		},
		{
			id:       3,
			coinsMap: map[string]Coin{},
			hashesStorage: map[string]map[int]string{
				"BTC": {
					800000: "0000...abc", // Consensus
				},
			},
		},
	}}

	// Set up GlobalHeights with BTC consensus height
	arena := fastjson.Arena{}
	serverIDsValue := arena.NewArray()
	serverIDsValue.SetArrayItem(0, arena.NewNumberInt(1))
	serverIDsValue.SetArrayItem(1, arena.NewNumberInt(2))
	serverIDsValue.SetArrayItem(2, arena.NewNumberInt(3))

	servers.GlobalCoinServerIDs = arena.NewObject()
	btcCoin := arena.NewObject()
	btcCoin.Set("ids", serverIDsValue)
	servers.GlobalCoinServerIDs.Set("BTC", btcCoin)

	servers.GlobalHeights = arena.NewObject()
	heights := arena.NewObject()
	heights.Set("BTC", arena.NewNumberInt(800000))
	servers.GlobalHeights.Set("result", heights)

	nonConsensus := FindServersFailingHashConsensus(servers)
	log.Printf("TEST_UNIT: Non-consensus servers: %v", nonConsensus)
	assert.Equal(t, 1, len(nonConsensus["BTC"]), "Should have 1 non-consensus server")
	assert.Equal(t, 2, nonConsensus["BTC"][0], "Server ID 2 should be non-consensus")
	log.Printf("TEST_UNIT: Found non-consensus server: id=%d", nonConsensus["BTC"][0])
}

func TestServersConcurrentServerUpdates(t *testing.T) {
	defer recordTestResult("TestServersConcurrentServerUpdates", t.Failed())
	log.Printf("TEST_UNIT: Starting TestServersConcurrentServerUpdates")
	defer log.Printf("TEST_UNIT: Finished TestServersConcurrentServerUpdates")

	servers := &Servers{Slice: make([]*Server, 0, 20)}

	// Add servers concurrently
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			servers.AddServer(&Server{
				url: "http://server" + strconv.Itoa(id),
				exr: true,
			})
		}(i)
	}
	wg.Wait()

	serverCount := len(servers.Slice)
	log.Printf("TEST_UNIT: Total servers: %d", serverCount)
	assert.Equal(t, 20, serverCount, "Should have 20 servers")

	// Verify unique IDs
	idSet := make(map[int]bool)
	for i, s := range servers.Slice {
		log.Printf("TEST_UNIT: Server %d: id=%d url=%s", i+1, s.id, s.url)
		if idSet[s.id] {
			log.Printf("TEST_UNIT: Duplicate server ID found: %d", s.id)
			t.Errorf("Duplicate server ID found: %d", s.id)
		}
		idSet[s.id] = true
	}
	log.Printf("TEST_UNIT: Verified %d unique server IDs", serverCount)
}

func TestServersUpdateGlobalFees_SingleServer(t *testing.T) {
	defer recordTestResult("TestServersUpdateGlobalFees_SingleServer", t.Failed())
	
	oldConfig := config
	defer func() { config = oldConfig }()
	config = &Config{ConsensusThreshold: 0.6666666666666666}
	log.Printf("TEST_UNIT: Starting TestServersUpdateGlobalFees_SingleServer (threshold=%.16f)", config.ConsensusThreshold)
	defer log.Printf("TEST_UNIT: Finished TestServersUpdateGlobalFees_SingleServer")

	servers := &Servers{Slice: []*Server{
		{
			id: 1,
			coinsMap: map[string]Coin{
				"BTC": {fee: 0.00001},
				"LTC": {fee: 0.00002},
			},
		},
	}}

	servers.UpdateGlobalFees()
	if servers.GlobalFees == nil {
		log.Printf("TEST_UNIT: Updated global fees is nil")
	} else {
		log.Printf("TEST_UNIT: Updated global fees: %s", servers.GlobalFees.String())
	}

	btc := servers.GlobalFees.Get("result", "BTC")
	if btc != nil {
		log.Printf("TEST_UNIT: BTC consensus: %.8f", btc.GetFloat64())
		assert.Equal(t, 0.00001, btc.GetFloat64(), "BTC fee should match single server value")
	} else {
		t.Fatal("BTC is nil in global fees")
	}

	ltc := servers.GlobalFees.Get("result", "LTC")
	if ltc != nil {
		log.Printf("TEST_UNIT: LTC consensus: %.8f", ltc.GetFloat64())
		assert.Equal(t, 0.00002, ltc.GetFloat64(), "LTC fee should match single server value")
	} else {
		t.Fatal("LTC is nil in global fees")
	}
}

func TestServersUpdateGlobalHeights_SingleServer(t *testing.T) {
	defer recordTestResult("TestServersUpdateGlobalHeights_SingleServer", t.Failed())
	
	oldConfig := config
	defer func() { config = oldConfig }()
	config = &Config{ConsensusThreshold: 0.6666666666666666}
	log.Printf("TEST_UNIT: Starting TestServersUpdateGlobalHeights_SingleServer")
	defer log.Printf("TEST_UNIT: Finished TestServersUpdateGlobalHeights_SingleServer")

	servers := &Servers{Slice: []*Server{
		{
			id: 1,
			coinsMap: map[string]Coin{
				"BTC": {getBlockCount: 800000},
				"LTC": {getBlockCount: 2000000},
			},
		},
	}}

	servers.UpdateGlobalHeights()
	log.Printf("TEST_UNIT: Updated global heights: %s", servers.GlobalHeights.String())

	btcHeight := servers.GlobalHeights.Get("result", "BTC").GetInt()
	log.Printf("TEST_UNIT: BTC height: %d", btcHeight)
	assert.Equal(t, 800000, btcHeight, "BTC height should match single server value")

	ltcHeight := servers.GlobalHeights.Get("result", "LTC").GetInt()
	log.Printf("TEST_UNIT: LTC height: %d", ltcHeight)
	assert.Equal(t, 2000000, ltcHeight, "LTC height should match single server value")
}

func TestServersHashConsensusDetection_SingleServer(t *testing.T) {
	defer recordTestResult("TestServersHashConsensusDetection_SingleServer", t.Failed())
	log.Printf("TEST_UNIT: Starting TestServersHashConsensusDetection_SingleServer")
	defer log.Printf("TEST_UNIT: Finished TestServersHashConsensusDetection_SingleServer")

	oldConfig := config
	defer func() { config = oldConfig }()
	config = &Config{ConsensusThreshold: 0.6666666666666666}

	blockCache["BTC_single"] = &BlockCache{
		BlockHash: "0000...abc",
		timeDiff:  15,
		cachedAt:  time.Now(),
	}

	servers := &Servers{Slice: []*Server{
		{
			id:       1,
			coinsMap: map[string]Coin{},
			hashesStorage: map[string]map[int]string{
				"BTC": {
					800000: "0000...abc",
				},
			},
		},
	}}

	// Set up GlobalHeights with BTC consensus height
	arena := fastjson.Arena{}
	serverIDsValue := arena.NewArray()
	serverIDsValue.SetArrayItem(0, arena.NewNumberInt(1))
	
	servers.GlobalCoinServerIDs = arena.NewObject()
	btcCoin := arena.NewObject()
	btcCoin.Set("ids", serverIDsValue)
	servers.GlobalCoinServerIDs.Set("BTC", btcCoin)

	servers.GlobalHeights = arena.NewObject()
	heights := arena.NewObject()
	heights.Set("BTC", arena.NewNumberInt(800000))
	servers.GlobalHeights.Set("result", heights)

	nonConsensus := FindServersFailingHashConsensus(servers)
	assert.Equal(t, 0, len(nonConsensus["BTC"]), "Single server should always be in consensus")
	log.Printf("TEST_UNIT: No non-consensus servers detected as expected")
}

func TestServersBlockCacheManagement(t *testing.T) {
	defer recordTestResult("TestServersBlockCacheManagement", t.Failed())
	log.Printf("TEST_UNIT: Starting TestServersBlockCacheManagement")
	defer log.Printf("TEST_UNIT: Finished TestServersBlockCacheManagement")

	// Reset global cache
	blockCache = make(map[string]*BlockCache)
	log.Printf("TEST_UNIT: Reset global block cache")

	// Test per-coin cache limits with multiple coins
	maxStoredBlocks := 3
	
	// Create BTC entries - should be limited to maxStoredBlocks per coin
	// Entry 0 should be oldest, entry 4 should be newest
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("BTC_hash_%d", i)
		blockCache[key] = &BlockCache{
			BlockHash: "btc_block_hash_" + strconv.Itoa(i),
			timeDiff:  float64(i),
			cachedAt:  time.Now().Add(time.Duration(-5+i) * time.Minute), // Earlier entries are older
		}
	}

	// Create LTC entries - should be limited independently
	// Entry 0 should be oldest, entry 3 should be newest
	for i := 0; i < 4; i++ {
		key := fmt.Sprintf("LTC_hash_%d", i)
		blockCache[key] = &BlockCache{
			BlockHash: "ltc_block_hash_" + strconv.Itoa(i),
			timeDiff:  float64(i),
			cachedAt:  time.Now().Add(time.Duration(-4+i) * time.Minute), // Earlier entries are older
		}
	}

	// Create DOGE entries - should be under limit, not affected
	for i := 0; i < 2; i++ {
		key := fmt.Sprintf("DOGE_hash_%d", i)
		blockCache[key] = &BlockCache{
			BlockHash: "doge_block_hash_" + strconv.Itoa(i),
			timeDiff:  float64(i),
			cachedAt:  time.Now().Add(time.Duration(-2+i) * time.Minute), // Earlier entries are older
		}
	}

	initialCacheSize := len(blockCache)
	log.Printf("TEST_UNIT: Initial cache size: %d (BTC:5, LTC:4, DOGE:2)", initialCacheSize)
	assert.Equal(t, 11, initialCacheSize, "Should start with 11 entries")

	// Purge cache with per-coin limit
	purgeCache(blockCache, maxStoredBlocks)
	
	finalCacheSize := len(blockCache)
	log.Printf("TEST_UNIT: Final cache size after purge: %d", finalCacheSize)
	
	// Verify per-coin counts
	btcCount := 0
	ltcCount := 0
	dogeCount := 0
	
	for key := range blockCache {
		if strings.HasPrefix(key, "BTC_") {
			btcCount++
		} else if strings.HasPrefix(key, "LTC_") {
			ltcCount++
		} else if strings.HasPrefix(key, "DOGE_") {
			dogeCount++
		}
	}
	
	log.Printf("TEST_UNIT: Final counts - BTC: %d, LTC: %d, DOGE: %d", btcCount, ltcCount, dogeCount)
	
	// BTC: 5 entries -> should be reduced to maxStoredBlocks (3)
	assert.Equal(t, maxStoredBlocks, btcCount, "BTC should have exactly %d entries after purge", maxStoredBlocks)
	
	// LTC: 4 entries -> should be reduced to maxStoredBlocks (3)  
	assert.Equal(t, maxStoredBlocks, ltcCount, "LTC should have exactly %d entries after purge", maxStoredBlocks)
	
	// DOGE: 2 entries -> should remain unchanged (under limit)
	assert.Equal(t, 2, dogeCount, "DOGE should remain unchanged at 2 entries")
	
	// Total should be 3 + 3 + 2 = 8
	expectedTotal := maxStoredBlocks + maxStoredBlocks + 2
	assert.Equal(t, expectedTotal, finalCacheSize, "Final cache size should be %d", expectedTotal)
	
	// Verify oldest entries were removed (first entries in slice should be gone)
	// For BTC: entries 0,1 should be removed, leaving 2,3,4
	// For LTC: entry 0 should be removed, leaving 1,2,3
	remainingBTC := make([]int, 0)
	remainingLTC := make([]int, 0)
	
	for key := range blockCache {
		if strings.HasPrefix(key, "BTC_hash_") {
			if suffix := strings.TrimPrefix(key, "BTC_hash_"); suffix != "" {
				if idx, err := strconv.Atoi(suffix); err == nil {
					remainingBTC = append(remainingBTC, idx)
				}
			}
		} else if strings.HasPrefix(key, "LTC_hash_") {
			if suffix := strings.TrimPrefix(key, "LTC_hash_"); suffix != "" {
				if idx, err := strconv.Atoi(suffix); err == nil {
					remainingLTC = append(remainingLTC, idx)
				}
			}
		}
	}
	
	sort.Ints(remainingBTC)
	sort.Ints(remainingLTC)
	
	log.Printf("TEST_UNIT: Remaining BTC indices: %v", remainingBTC)
	log.Printf("TEST_UNIT: Remaining LTC indices: %v", remainingLTC)
	
	// BTC should have indices 2,3,4 (oldest 0,1 removed)
	if !equalIntSlices(remainingBTC, []int{2, 3, 4}) {
		t.Fatalf("BTC should retain newest 3 entries, got %v", remainingBTC)
	}
	
	// LTC should have indices 1,2,3 (oldest 0 removed) 
	if !equalIntSlices(remainingLTC, []int{1, 2, 3}) {
		t.Fatalf("LTC should retain newest 3 entries, got %v", remainingLTC)
	}
}

func TestServersBlockCacheManagement_EmptyCache(t *testing.T) {
	defer recordTestResult("TestServersBlockCacheManagement_EmptyCache", t.Failed())
	log.Printf("TEST_UNIT: Starting TestServersBlockCacheManagement_EmptyCache")
	defer log.Printf("TEST_UNIT: Finished TestServersBlockCacheManagement_EmptyCache")

	// Test with empty cache
	blockCache = make(map[string]*BlockCache)
	initialSize := len(blockCache)
	assert.Equal(t, 0, initialSize, "Cache should start empty")
	
	purgeCache(blockCache, 5)
	finalSize := len(blockCache)
	assert.Equal(t, 0, finalSize, "Empty cache should remain empty after purge")
}

func TestServersBlockCacheManagement_ExactlyAtLimit(t *testing.T) {
	defer recordTestResult("TestServersBlockCacheManagement_ExactlyAtLimit", t.Failed())
	log.Printf("TEST_UNIT: Starting TestServersBlockCacheManagement_ExactlyAtLimit")
	defer log.Printf("TEST_UNIT: Finished TestServersBlockCacheManagement_ExactlyAtLimit")

	blockCache = make(map[string]*BlockCache)
	
	// Add exactly maxStoredBlocks entries for BTC
	maxStoredBlocks := 3
	for i := 0; i < maxStoredBlocks; i++ {
		key := fmt.Sprintf("BTC_hash_%d", i)
		blockCache[key] = &BlockCache{
			BlockHash: "btc_hash_" + strconv.Itoa(i),
			timeDiff:  float64(i),
			cachedAt:  time.Now().Add(time.Duration(-i) * time.Minute), // Earlier entries are older
		}
	}
	
	initialSize := len(blockCache)
	assert.Equal(t, maxStoredBlocks, initialSize, "Should start with exactly limit entries")
	
	purgeCache(blockCache, maxStoredBlocks)
	finalSize := len(blockCache)
	assert.Equal(t, maxStoredBlocks, finalSize, "Cache at limit should remain unchanged")
}

func TestServersBlockCacheManagement_SingleCoin(t *testing.T) {
	defer recordTestResult("TestServersBlockCacheManagement_SingleCoin", t.Failed())
	log.Printf("TEST_UNIT: Starting TestServersBlockCacheManagement_SingleCoin")
	defer log.Printf("TEST_UNIT: Finished TestServersBlockCacheManagement_SingleCoin")

	blockCache = make(map[string]*BlockCache)
	
	// Test with single coin having many entries
	maxStoredBlocks := 2
	totalEntries := 10
	
	// Create entries with different cachedAt times
	// Entry 0 should be oldest, entry 9 should be newest
	for i := 0; i < totalEntries; i++ {
		key := fmt.Sprintf("BTC_hash_%d", i)
		blockCache[key] = &BlockCache{
			BlockHash: "btc_hash_" + strconv.Itoa(i),
			timeDiff:  float64(i),
			cachedAt:  time.Now().Add(time.Duration(-10+i) * time.Minute), // Earlier entries are older
		}
	}
	
	initialSize := len(blockCache)
	assert.Equal(t, totalEntries, initialSize, "Should start with %d entries", totalEntries)
	
	purgeCache(blockCache, maxStoredBlocks)
	finalSize := len(blockCache)
	assert.Equal(t, maxStoredBlocks, finalSize, "Should be reduced to limit")
	
	// Verify only newest entries remain
	remainingIndices := make([]int, 0)
	for key := range blockCache {
		if suffix := strings.TrimPrefix(key, "BTC_hash_"); suffix != "" {
			if idx, err := strconv.Atoi(suffix); err == nil {
				remainingIndices = append(remainingIndices, idx)
			}
		}
	}
	
	sort.Ints(remainingIndices)
	expectedRemaining := []int{8, 9} // indices 8 and 9 (0-based), the newest 2
	if !equalIntSlices(remainingIndices, expectedRemaining) {
		t.Fatalf("Should retain newest 2 entries, got %v", remainingIndices)
	}
}

// Helper function to compare integer slices
func equalIntSlices(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
