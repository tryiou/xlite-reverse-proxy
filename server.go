package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/valyala/fastjson"
)

func (s *Server) pruneHashStorage() {
	for _, storage := range s.hashesStorage {
		if len(storage) <= MaxHashStorageLength {
			continue
		}
		heights := make([]int, 0, len(storage))
		for height := range storage {
			heights = append(heights, height)
		}
		sort.Sort(sort.Reverse(sort.IntSlice(heights)))
		for i := MaxHashStorageLength; i < len(heights); i++ {
			delete(storage, heights[i])
		}
	}
}

func (s *Server) setDefaultResponses() {
	s.ping = 0
	s.coinsMap = make(map[string]Coin)
	s.getfees = getDefaultJSONResponse()
	s.getheights = getDefaultJSONResponse()
}

func (s *Server) server_GetPing() error {
	payloadMethod := "ping"
	payloadParams := []interface{}{}

	response, err := s.makeHTTPRequest(http.MethodPost, payloadMethod, payloadParams)
	if err != nil {
		s.setDefaultResponses()
		// Log the server communication error with more context
		// logger.Printf(LogPrefixServerError+" server[%d] ping failed: %v", s.id, s.id, err)
		return fmt.Errorf("server[%d] ping request failed: %w", s.id, err)
	}

	jsonResp, err := parseJSON(response)
	if err != nil {
		s.setDefaultResponses()
		logger.Printf(LogPrefixServerError+" ping JSON parse failed: %v", s.id, err)
		return fmt.Errorf("server[%d] ping JSON parse failed: %w", s.id, err)
	}

	result := jsonResp.Get("result")
	if result == nil {
		s.setDefaultResponses()
		logger.Printf(LogPrefixServerError+" ping missing result field", s.id)
		return fmt.Errorf("server[%d] ping response missing 'result' field", s.id)
	}

	if result.Type() == fastjson.TypeNumber && result.GetInt() == PingSuccessValue {
		s.ping = PingSuccessValue
		// logger.Printf(LogPrefixServer+" server[%d] ping successful", s.id, s.id)
	} else {
		s.ping = PingFailureValue
		logger.Printf(LogPrefixServerError+" ping returned non-success value: %d", s.id, result.GetInt())
	}
	return nil
}

func (s *Server) server_GetBlock(coin string, blockHash string) (*fastjson.Value, error) {
	payloadMethod := "getblock"
	payloadParams := []interface{}{coin, blockHash, "true"}

	response, err := s.makeHTTPRequest(http.MethodPost, payloadMethod, payloadParams)
	if err != nil {
		logger.Printf(LogPrefixServerError+" getblock failed for coin %s, hash %s: %v", s.id, coin, blockHash, err)
		return nil, fmt.Errorf("server[%d] getblock request failed for coin %s: %w", s.id, coin, err)
	}

	jsonResp, err := parseJSON(response)
	if err != nil {
		logger.Printf(LogPrefixServerError+" getblock JSON parse failed for coin %s: %v", s.id, coin, err)
		return nil, fmt.Errorf("server[%d] getblock JSON parse failed for coin %s: %w", s.id, coin, err)
	}

	jsonError := jsonResp.Get("error")
	if jsonError.Type() != fastjson.TypeNull {
		errorMsg := jsonError.String()
		logger.Printf(LogPrefixServerError+" getblock error for coin %s: %s", s.id, coin, errorMsg)
		return nil, fmt.Errorf("server[%d] getblock error for coin %s: %s", s.id, coin, errorMsg)
	}

	jsonResult := jsonResp.Get("result")
	if jsonResult.Type() != fastjson.TypeObject {
		logger.Printf(LogPrefixServerError+" getblock invalid result type for coin %s", s.id, coin)
		return nil, fmt.Errorf("server[%d] getblock invalid result type for coin %s", s.id, coin)
	}

	// Success logging removed - keep only error logging
	return jsonResult, nil
}

func (s *Server) server_GetBlockHash(coin string, height int) (string, error) {
	payloadMethod := "getblockhash"
	payloadParams := []interface{}{coin, height}

	if height == InvalidHeightValue {
		logger.Printf(LogPrefixError+" server[%d] getblockhash called with invalid height %d for coin %s", s.id, height, coin)
		return "", nil
	}

	response, err := s.makeHTTPRequest(http.MethodPost, payloadMethod, payloadParams)
	if err != nil {
		logger.Printf(LogPrefixServerError+" getblockhash failed for coin %s, height %d: %v", s.id, coin, height, err)
		return "", fmt.Errorf("server[%d] getblockhash request failed for coin %s height %d: %w", s.id, coin, height, err)
	}

	jsonResp, err := parseJSON(response)
	if err != nil {
		logger.Printf(LogPrefixServerError+" getblockhash JSON parse failed for coin %s height %d: %v", s.id, coin, height, err)
		return "", fmt.Errorf("server[%d] getblockhash JSON parse failed for coin %s height %d: %w", s.id, coin, height, err)
	}

	jsonError := jsonResp.Get("error")
	if jsonError.Type() != fastjson.TypeNull {
		errorMsg := jsonError.String()
		logger.Printf(LogPrefixServerError+" getblockhash error for coin %s height %d: %s", s.id, coin, height, errorMsg)
		return "", fmt.Errorf("server[%d] getblockhash error for coin %s height %d: %s", s.id, coin, height, errorMsg)
	}

	result := jsonResp.Get("result").String()
	if result == "" {
		logger.Printf(LogPrefixServerError+" getblockhash empty result for coin %s height %d", s.id, coin, height)
		return "", fmt.Errorf("server[%d] getblockhash empty result for coin %s height %d", s.id, coin, height)
	}

	hash := removeNonPrintableChars(result)
	// logger.Printf(LogPrefixServer+" server[%d] getblockhash successful for coin %s height %d: %s", s.id, s.id, coin, height, hash)
	return hash, nil
}

func (s *Server) server_GetFees() error {
	payloadMethod := "fees"
	payloadParams := []interface{}{}
	response, err := s.makeHTTPRequest(http.MethodPost, payloadMethod, payloadParams)
	if err != nil {
		s.getfees = getDefaultJSONResponse()
		return fmt.Errorf("failed to make HTTP request: %w", err)
	}
	jsonResp, err := parseJSON(response)
	if err != nil {
		s.getfees = getDefaultJSONResponse()
		return fmt.Errorf("failed to parse JSON response: %w", err)
	}
	s.getfees = jsonResp
	return nil
}

func (s *Server) server_GetHeights() error {
	payloadMethod := "heights"
	payloadParams := []interface{}{}
	response, err := s.makeHTTPRequest(http.MethodPost, payloadMethod, payloadParams)
	if err != nil {
		s.getheights = getDefaultJSONResponse()
		return fmt.Errorf("failed to make HTTP request: %w", err)
	}
	jsonResp, err := parseJSON(response)
	if err != nil {
		s.getheights = getDefaultJSONResponse()
		return fmt.Errorf("failed to parse JSON response: %w", err)
	}
	s.getheights = jsonResp
	s.sortGetHeightsKeys()
	return nil
}

func (s *Server) sortGetHeightsKeys() {
	if s.getheights != nil {
		resultObj := s.getheights.Get("result")
		if resultObj != nil && resultObj.Type() == fastjson.TypeObject {
			keys := make([]string, 0)
			resultObj.GetObject().Visit(func(key []byte, value *fastjson.Value) {
				keys = append(keys, string(key))
			})
			sort.Strings(keys)
			sortedResultObj := getEmptyJSONResponse()
			for _, key := range keys {
				value := resultObj.Get(key)
				sortedResultObj.Set(key, value)
			}
			s.getheights.Set("result", sortedResultObj)
		}
	}
}

func (s *Server) makeHTTPRequest(httpMethod, payloadMethod string, payloadParams []interface{}) ([]byte, error) {
	var (
		url     string
		payload string
	)

	// Validate server configuration
	if s.url == "" {
		return nil, fmt.Errorf("server %d has empty URL", s.id)
	}

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
			return nil, fmt.Errorf("server[%d] failed to marshal payloadData to JSON for method %s: %w", s.id, payloadMethod, err)
		}
		payload = string(payloadBytes)
	} else {
		// EXR NODE!
		url = s.url + "/xrs/" + payloadMethod
		payloadBytes, err := json.Marshal(payloadParams)
		if err != nil {
			return nil, fmt.Errorf("server[%d] failed to marshal payloadParams to JSON for EXR method %s: %w", s.id, payloadMethod, err)
		}
		payload = string(payloadBytes)
	}

	reqTimer := time.Now()

	// Validate URL format
	if !strings.Contains(url, "://") {
		return nil, fmt.Errorf("server[%d] invalid URL format: %s", s.id, url)
	}

	req, err := http.NewRequest(httpMethod, url, strings.NewReader(payload))
	if err != nil {
		elapsed := time.Since(reqTimer)
		return nil, fmt.Errorf("server[%d] failed to create HTTP request for %s: %w (elapsed: %v)", s.id, url, err, elapsed)
	}

	req.Header.Set(HeaderContentType, ContentTypeJSON)

	// Add request timeout context if not already set
	if req.Context() == nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.HttpTimeout)*time.Second)
		defer cancel()
		req = req.WithContext(ctx)
	}

	res, err := httpClient.Do(req)
	if err != nil {
		elapsed := time.Since(reqTimer)

		// Provide specific error messages based on error type
		if netErr, ok := err.(net.Error); ok {
			if netErr.Timeout() {
				return nil, fmt.Errorf("server[%d] request timeout after %v for %s: %w", s.id, elapsed, url, err)
			}
			if netErr.Temporary() {
				return nil, fmt.Errorf("server[%d] temporary network error for %s: %w", s.id, url, err)
			}
		}

		// Handle connection-specific errors
		if strings.Contains(err.Error(), "connection refused") {
			return nil, fmt.Errorf("server[%d] connection refused for %s: %w", s.id, url, err)
		}
		if strings.Contains(err.Error(), "no such host") {
			return nil, fmt.Errorf("server[%d] DNS resolution failed for %s: %w", s.id, url, err)
		}
		if strings.Contains(err.Error(), "too many open files") {
			return nil, fmt.Errorf("server[%d] system resource limit reached for %s: %w", s.id, url, err)
		}

		return nil, fmt.Errorf("server[%d] failed to send HTTP request to %s: %w (elapsed: %v)", s.id, url, err, elapsed)
	}
	defer res.Body.Close()

	// Check HTTP status code
	if res.StatusCode != HTTPStatusOK {
		elapsed := time.Since(reqTimer)
		return nil, fmt.Errorf("server[%d] HTTP %d %s for %s (elapsed: %v)", s.id, res.StatusCode, res.Status, url, elapsed)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("server[%d] failed to read HTTP response body from %s: %w", s.id, url, err)
	}

	// elapsed := time.Since(reqTimer)
	// logger.Printf(LogPrefixServer+" server[%d] HTTP request to %s successful (%v, %d bytes)", s.id, s.id, url, elapsed, len(body))
	return body, nil
}
