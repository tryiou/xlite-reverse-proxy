package main

import (
	"fmt"
	"strings"

	"github.com/valyala/fastjson"
)

func getDefaultJSONResponse() *fastjson.Value {
	return fastjson.MustParse(`{"result": null, "error": null}`)
}

func getEmptyJSONResponse() *fastjson.Value {
	return fastjson.MustParse(`{}`)
}

func parseJSON(data []byte) (*fastjson.Value, error) {
	var p fastjson.Parser
	value, err := p.ParseBytes(data)
	if err != nil {
		errorMsg := string(data)
		if strings.Contains(errorMsg, "Internal Server Error") {
			// Handle the error by producing valid JSON
			return fastjson.Parse(`{"error": "Internal Server Error"}`)
		}
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	return value, nil
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
