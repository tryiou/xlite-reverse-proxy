// script parameters

package main

import "sync"

// exr = false  ==> direct connection to 'plugin_adapter'
// exr = true ==> connect via EXR calls, accross Xrproxy
var serversJsonList = `
[	
	{
		"url": "https://utils.blocknet.org",
		"exr": true
	},
	{
		"url": "http://149.102.153.235",
		"exr": true
	},
	{
		"url": "http://exrproxy3.airdns.org:42114",
		"exr": true
	}

]
`

/*

	{
		"url": "https://utils.blocknet.org",
		"exr": true
	},

	{
		"url": "http://149.102.153.235",
		"exr": true
	},
	{
		"url": "http://149.102.153.235",
		"exr": true
	},
*/

var acceptedPaths = []string{
	"/",
	"/height",
	"/heights",
	"/fees",
	"/ping",
	"/servers",
}
var acceptedMethods = []string{
	"getutxos",
	"getrawtransaction",
	"getrawmempool",
	"getblockcount",
	"sendrawtransaction",
	"gettransaction",
	"getblock",
	"getblockhash",
	"heights",
	"fees",
	"getbalance",
	"gethistory",
	"ping",
}

var mu sync.Mutex
var blockCache = make(map[string]*BlockCache)

// Max number of stored blocks to keep in the cache.
const maxStoredBlocks = 3
const maxBlockTimeDiff = 3600 * 2

var httpTimeout = 8

// Create a rate limiter with a limit of x requests per minute
var rateLimit = 80

const maxLogSize = 50 * 1024 * 1024 // 50 MB
