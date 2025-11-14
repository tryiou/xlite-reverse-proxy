package main

import (
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
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

// WriteJSONResponse writes a JSON response with proper headers
func WriteJSONResponse(w http.ResponseWriter, value *fastjson.Value) error {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	_, err := w.Write(value.MarshalTo(nil))
	return err
}

// ParseToFastjson converts any Go value to fastjson.Value
func ParseToFastjson(data interface{}) (*fastjson.Value, error) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return parseJSON(jsonBytes)
}

// Fixed purgeCache function - groups by coin from cache key and removes oldest chronological entries
func purgeCache(blockCache map[string]*BlockCache, maxStoredBlocks int) {
	// Group cache entries by coin (from key format "coin_hash")
	coinEntries := make(map[string][]string)
	for key := range blockCache {
		parts := strings.SplitN(key, "_", 2) // Split into coin and hash
		if len(parts) >= 1 {
			coin := parts[0]
			coinEntries[coin] = append(coinEntries[coin], key)
		}
	}

	totalRemoved := 0
	coinsPurged := 0

	// Remove excess entries per coin
	for _, keys := range coinEntries {
		if len(keys) > maxStoredBlocks {
			coinsPurged++
			toRemove := len(keys) - maxStoredBlocks
			totalRemoved += toRemove

			// logger.Printf("[CACHE] Coin %s has %d entries, removing %d oldest (keeping %d most recent)", coin, len(keys), toRemove, maxStoredBlocks)

			// Sort keys by cachedAt time to identify oldest entries (earliest cachedAt = oldest)
			sort.Slice(keys, func(i, j int) bool {
				return blockCache[keys[i]].cachedAt.Before(blockCache[keys[j]].cachedAt)
			})

			// Remove oldest entries
			entriesToDelete := keys[:toRemove]
			for _, key := range entriesToDelete {
				// logger.Printf("[CACHE] Removing %s entry: %s (cached at: %v)",
				// 	coin, key, blockCache[key].cachedAt)
				delete(blockCache, key)
			}
		}
	}

	// Only log if we actually removed entries
	// if totalRemoved > 0 {
	// 	finalCount := len(blockCache)
	// 	logger.Printf("[CACHE] Purge completed - removed %d entries from %d coins, %d total entries remaining",
	// 		totalRemoved, coinsPurged, finalCount)
	// }
}

func decompressGzip(input io.Reader) ([]byte, error) {
	reader, err := gzip.NewReader(input)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

func decompressDeflate(input io.Reader) ([]byte, error) {
	reader := flate.NewReader(input)
	defer reader.Close()

	return io.ReadAll(reader)
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
