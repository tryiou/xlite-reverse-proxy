package main

import (
	"log"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fastjson"
)

func TestUpdateGlobalFeesConsensus(t *testing.T) {
	defer recordTestResult("TestUpdateGlobalFeesConsensus", t.Failed())
	log.Printf("TEST_UNIT: Starting TestUpdateGlobalFeesConsensus (threshold=%.16f)", 0.6666666666666666)
	defer log.Printf("TEST_UNIT: Finished TestUpdateGlobalFeesConsensus")

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

func TestUpdateGlobalFees_4servers(t *testing.T) {
	defer recordTestResult("TestUpdateGlobalFees_4servers", t.Failed())
	log.Printf("TEST_UNIT: Starting TestUpdateGlobalFees_4servers (threshold=%.16f)", 0.6666666666666666)
	defer log.Printf("TEST_UNIT: Finished TestUpdateGlobalFees_4servers")

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

func TestUpdateGlobalFees_10servers(t *testing.T) {
	defer recordTestResult("TestUpdateGlobalFees_10servers", t.Failed())
	log.Printf("TEST_UNIT: Starting TestUpdateGlobalFees_10servers (threshold=%.16f)", 0.6666666666666666)
	defer log.Printf("TEST_UNIT: Finished TestUpdateGlobalFees_10servers")

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
		if i >= 6 { // 6 servers for LTC consensus, 4 disagree
			ltcFee = 0.00003
		}

		servers.Slice[i] = &Server{
			id: i+1,
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

func TestUpdateGlobalHeightsConsensus(t *testing.T) {
	defer recordTestResult("TestUpdateGlobalHeightsConsensus", t.Failed())
	log.Printf("TEST_UNIT: Starting TestUpdateGlobalHeightsConsensus")
	defer log.Printf("TEST_UNIT: Finished TestUpdateGlobalHeightsConsensus")

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

func TestUpdateGlobalHeights_4servers(t *testing.T) {
	defer recordTestResult("TestUpdateGlobalHeights_4servers", t.Failed())
	log.Printf("TEST_UNIT: Starting TestUpdateGlobalHeights_4servers")
	defer log.Printf("TEST_UNIT: Finished TestUpdateGlobalHeights_4servers")

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

func TestUpdateGlobalHeights_10servers(t *testing.T) {
	defer recordTestResult("TestUpdateGlobalHeights_10servers", t.Failed())
	log.Printf("TEST_UNIT: Starting TestUpdateGlobalHeights_10servers")
	defer log.Printf("TEST_UNIT: Finished TestUpdateGlobalHeights_10servers")

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
			id: i+1,
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

func TestHashConsensusDetection(t *testing.T) {
	defer recordTestResult("TestHashConsensusDetection", t.Failed())
	log.Printf("TEST_UNIT: Starting TestHashConsensusDetection")
	defer log.Printf("TEST_UNIT: Finished TestHashConsensusDetection")

	oldConfig := config
	defer func() { config = oldConfig }()
	config = &Config{ConsensusThreshold: 0.6666666666666666}

	// Initialize global block cache with BTC block 800000
	blockCache["BTC_consensus_hash"] = &BlockCache{
		BlockHash: "0000...abc",
		timeDiff:  15,
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

func TestConcurrentServerUpdates(t *testing.T) {
	defer recordTestResult("TestConcurrentServerUpdates", t.Failed())
	log.Printf("TEST_UNIT: Starting TestConcurrentServerUpdates")
	defer log.Printf("TEST_UNIT: Finished TestConcurrentServerUpdates")

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

func TestUpdateGlobalFees_SingleServer(t *testing.T) {
	defer recordTestResult("TestUpdateGlobalFees_SingleServer", t.Failed())
	
	oldConfig := config
	defer func() { config = oldConfig }()
	config = &Config{ConsensusThreshold: 0.6666666666666666}
	log.Printf("TEST_UNIT: Starting TestUpdateGlobalFees_SingleServer (threshold=%.16f)", config.ConsensusThreshold)
	defer log.Printf("TEST_UNIT: Finished TestUpdateGlobalFees_SingleServer")

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

func TestUpdateGlobalHeights_SingleServer(t *testing.T) {
	defer recordTestResult("TestUpdateGlobalHeights_SingleServer", t.Failed())
	
	oldConfig := config
	defer func() { config = oldConfig }()
	config = &Config{ConsensusThreshold: 0.6666666666666666}
	log.Printf("TEST_UNIT: Starting TestUpdateGlobalHeights_SingleServer")
	defer log.Printf("TEST_UNIT: Finished TestUpdateGlobalHeights_SingleServer")

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

func TestHashConsensusDetection_SingleServer(t *testing.T) {
	defer recordTestResult("TestHashConsensusDetection_SingleServer", t.Failed())
	log.Printf("TEST_UNIT: Starting TestHashConsensusDetection_SingleServer")
	defer log.Printf("TEST_UNIT: Finished TestHashConsensusDetection_SingleServer")

	oldConfig := config
	defer func() { config = oldConfig }()
	config = &Config{ConsensusThreshold: 0.6666666666666666}

	blockCache["BTC_single"] = &BlockCache{
		BlockHash: "0000...abc",
		timeDiff:  15,
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

func TestBlockCacheManagement(t *testing.T) {
	defer recordTestResult("TestBlockCacheManagement", t.Failed())
	log.Printf("TEST_UNIT: Starting TestBlockCacheManagement")
	defer log.Printf("TEST_UNIT: Finished TestBlockCacheManagement")

	// Reset global cache
	blockCache = make(map[string]*BlockCache)
	log.Printf("TEST_UNIT: Reset global block cache")

	// Create multiple entries for the same blockhash to trigger purging
	// This matches how purgeCache groups by BlockHash
	blockHash := "00000000000000000001325f6c0bdd010d8013dcfe11d143c771608a3beec9d3"
	for i := 0; i < 15; i++ {
		key := "BTC_hash_" + strconv.Itoa(i)
		blockCache[key] = &BlockCache{
			BlockHash: blockHash,
			timeDiff:  float64(i),
		}
	}

	purgeCache(blockCache, 5)
	cacheSize := len(blockCache)
	log.Printf("TEST_UNIT: Cache size after purge: %d", cacheSize)
	assert.Equal(t, 5, cacheSize, "Cache should be purged from 15 to 5 entries")

	// Verify that the blockhash count is now 5
	blockHashCount := 0
	for _, bc := range blockCache {
		if bc.BlockHash == blockHash {
			blockHashCount++
		}
	}
	assert.Equal(t, 5, blockHashCount, "Blockhash should have exactly 5 entries after purge")
}
