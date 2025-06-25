package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

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

func makeHTTPRequest(s Server, httpMethod, payloadMethod string, payloadParams []interface{}, timeout int) ([]byte, error) {
	var (
		url     string
		payload string
	)

	if !s.exr {
		// DIRECT CALL TO PLUGIN_ADAPTER
		url = s.url
		payloadData := struct {
			Method string        `json:"method"`
			Params []interface{} `json:"params"`
		}{
			Method: payloadMethod,
			Params: payloadParams,
		}
		payloadBytes, err := json.Marshal(payloadData)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payloadData to JSON: %w", err)
		}
		payload = string(payloadBytes)
	} else {
		// EXR NODE!
		url = s.url + "/xrs/" + payloadMethod
		payloadBytes, err := json.Marshal(payloadParams)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payloadParams to JSON: %w", err)
		}
		payload = string(payloadBytes)
	}

	//timeout
	timeoutDuration := time.Duration(timeout) * time.Second
	http.DefaultClient.Timeout = timeoutDuration

	client := &http.Client{
		Timeout: timeoutDuration,
	}
	reqTimer := time.Now()
	//elapsedTimer := time.Since(startTimer)
	req, err := http.NewRequest(httpMethod, url, strings.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w, %v", err, time.Since(reqTimer))
	}

	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send HTTP request: %w, %v", err, time.Since(reqTimer))
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read HTTP response body: %w", err)
	}

	return body, nil
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
