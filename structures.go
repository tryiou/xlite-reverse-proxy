package main

import (
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
}

type BlockCache struct {
	BlockHash string
	timeDiff  float64 // diff between block time and actual desktop time
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
