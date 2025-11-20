package main

import (
	"strings"
	"sync"
	"time"

	"github.com/valyala/fastjson"
)

type Coin struct {
	getBlockCount int
	getBlockHash  string
	fee           float64
	timeDiff      float64 // diff between block time and actual desktop time
}

type Server struct {
	id            int
	url           string
	exr           bool
	ping          int // 1 = on
	getfees       *fastjson.Value
	getheights    *fastjson.Value
	coinsMap      map[string]Coin
	hashesStorage map[string]map[int]string
	//                coins   heights hashes
}

type Servers struct {
	Slice []*Server // used for goroutines updates

	// global values, after working out consensus and health checks
	GlobalHeights       *fastjson.Value
	GlobalFees          *fastjson.Value
	GlobalCoinServerIDs *fastjson.Value

	// URL to Server.ID mapping for ID consistency
	urlToID map[string]int
}

type BlockCache struct {
	BlockHash string
	timeDiff  float64   // diff between block time and actual desktop time
	cachedAt  time.Time // when this entry was cached
}

type RequestData struct {
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
	Ip     string
	Path   string
}

type CoinData struct {
	Ids []int `json:"ids"`
}

type PrintData struct {
	ServerID      int
	Coin          string
	GetBlockCount int
	GetBlockHash  string
}

type ErrorResponse struct {
	Code  int    `json:"code"`
	Error string `json:"error"`
}

// JsonElement represents a single element in the JSON array returned by xrshowconfigs.
type JsonElement struct {
	NodePubKey     string            `json:"nodepubkey"`
	PaymentAddress string            `json:"paymentaddress"`
	Config         string            `json:"config"`
	Plugins        map[string]string `json:"plugins"`
}

// JsonResponse represents the top-level JSON response structure from xrshowconfigs.
type JsonResponse struct {
	Result string  `json:"result"`
	Error  *string `json:"error"`
	Id     int     `json:"id"`
}

// LockManager provides granular locking for different parts of the application
type LockManager struct {
	servers   sync.RWMutex // Protects servers.Slice and server data
	consensus sync.RWMutex // Protects GlobalHeights, GlobalFees, GlobalCoinServerIDs
	rateLimit sync.Mutex   // Protects visitors map in rateLimit.go
	config    sync.RWMutex // Protects config updates
	urlToID   sync.RWMutex // Protects servers.urlToID map
}

var locks = &LockManager{}

// ObjectPool manages reusable objects to reduce allocations
type ObjectPool struct {
	arenaPool sync.Pool
	mapPool   sync.Pool
	slicePool sync.Pool // Store *[]int instead
}

var globalPool = &ObjectPool{
	arenaPool: sync.Pool{
		New: func() interface{} {
			return &fastjson.Arena{}
		},
	},
	mapPool: sync.Pool{
		New: func() interface{} {
			return make(map[string]interface{})
		},
	},
	slicePool: sync.Pool{
		New: func() interface{} {
			slice := make([]int, 0, MaxHashStorageLength)
			return &slice
		},
	},
}

func (p *ObjectPool) GetArena() *fastjson.Arena {
	return p.arenaPool.Get().(*fastjson.Arena)
}

func (p *ObjectPool) PutArena(a *fastjson.Arena) {
	a.Reset()
	p.arenaPool.Put(a)
}

func (p *ObjectPool) GetMap() map[string]interface{} {
	return p.mapPool.Get().(map[string]interface{})
}

func (p *ObjectPool) PutMap(m map[string]interface{}) {
	for k := range m {
		delete(m, k)
	}
	p.mapPool.Put(m)
}

func (p *ObjectPool) GetSlice() *[]int {
	s := p.slicePool.Get().(*[]int)
	*s = (*s)[:0]
	return s
}

func (p *ObjectPool) PutSlice(s *[]int) {
	// Reset the slice before putting back in pool
	*s = (*s)[:0]
	p.slicePool.Put(s)
}

// OptimizedBlockCache provides efficient cache management with timestamp ordering
type OptimizedBlockCache struct {
	entries    map[string]*BlockCache
	timeHead   *BlockCacheEntry
	timeTail   *BlockCacheEntry
	coinCounts map[string]int
	maxPerCoin int
}

type BlockCacheEntry struct {
	key      string
	data     *BlockCache
	nextTime *BlockCacheEntry
}

func NewOptimizedBlockCache(maxPerCoin int) *OptimizedBlockCache {
	return &OptimizedBlockCache{
		entries:    make(map[string]*BlockCache),
		timeHead:   nil,
		timeTail:   nil,
		coinCounts: make(map[string]int),
		maxPerCoin: maxPerCoin,
	}
}

func (obc *OptimizedBlockCache) Add(key string, data *BlockCache) {
	// If already exists, just update
	if existing, exists := obc.entries[key]; exists {
		existing.timeDiff = data.timeDiff
		existing.cachedAt = data.cachedAt
		return
	}

	obc.entries[key] = data

	// Update per-coin count
	coin := obc.getCoinFromKey(key)
	obc.coinCounts[coin]++

	// Add to time-sorted list
	entry := &BlockCacheEntry{key: key, data: data}
	if obc.timeTail == nil {
		obc.timeHead = entry
		obc.timeTail = entry
	} else {
		obc.timeTail.nextTime = entry
		obc.timeTail = entry
	}

	// Check if we need to purge
	if obc.coinCounts[coin] > obc.maxPerCoin {
		obc.purgeCoin(coin)
	}
}

func (obc *OptimizedBlockCache) purgeCoin(coin string) {
	// Count entries for this coin that actually exist in the map
	coinEntries := make([]*BlockCacheEntry, 0)
	current := obc.timeHead
	for current != nil {
		if obc.getCoinFromKey(current.key) == coin {
			// Only include entries that still exist in the map
			if _, exists := obc.entries[current.key]; exists {
				coinEntries = append(coinEntries, current)
			}
		}
		current = current.nextTime
	}

	// Remove oldest entries if over limit
	if len(coinEntries) > obc.maxPerCoin {
		toRemove := len(coinEntries) - obc.maxPerCoin
		for i := 0; i < toRemove; i++ {
			// Only delete from map and decrement count if entry exists
			if _, exists := obc.entries[coinEntries[i].key]; exists {
				delete(obc.entries, coinEntries[i].key)
				obc.coinCounts[coin]--
			}
		}
	}
}

func (obc *OptimizedBlockCache) getCoinFromKey(key string) string {
	parts := strings.SplitN(key, "_", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

func (obc *OptimizedBlockCache) Get(key string) (*BlockCache, bool) {
	data, exists := obc.entries[key]
	return data, exists
}

func (obc *OptimizedBlockCache) Purge() {
	// Only rebuild if the time-sorted list is corrupted
	if obc.timeHead == nil && len(obc.entries) > 0 {
		obc.rebuildTimeList()
	}
}

func (obc *OptimizedBlockCache) rebuildTimeList() {
	// Reset the time-sorted list
	obc.timeHead = nil
	obc.timeTail = nil

	if len(obc.entries) == 0 {
		return
	}

	// Collect all entries and sort them by cachedAt time (oldest first)
	type timedEntry struct {
		key  string
		data *BlockCache
	}

	var allEntries []timedEntry
	for key, data := range obc.entries {
		allEntries = append(allEntries, timedEntry{key: key, data: data})
	}

	// Sort by cachedAt time (oldest first) using simple bubble sort to avoid imports
	for i := 0; i < len(allEntries)-1; i++ {
		for j := i + 1; j < len(allEntries); j++ {
			if allEntries[i].data.cachedAt.After(allEntries[j].data.cachedAt) {
				allEntries[i], allEntries[j] = allEntries[j], allEntries[i]
			}
		}
	}

	// Rebuild the linked list
	var prev *BlockCacheEntry
	for _, entry := range allEntries {
		listEntry := &BlockCacheEntry{
			key:  entry.key,
			data: entry.data,
		}

		if prev == nil {
			obc.timeHead = listEntry
		} else {
			prev.nextTime = listEntry
		}
		prev = listEntry
	}

	obc.timeTail = prev
}

// Global optimized cache instance
var optimizedBlockCache *OptimizedBlockCache
