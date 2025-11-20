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
		return fmt.Errorf("server[%d] %s: %w", s.id, ErrorMessagePingFailed, err)
	}

	jsonResp, err := parseJSON(response)
	if err != nil {
		s.setDefaultResponses()
		return fmt.Errorf("server[%d] %s: %w", s.id, ErrorMessageJSONParseFailed, err)
	}

	result := jsonResp.Get("result")
	if result == nil {
		s.setDefaultResponses()
		return fmt.Errorf("server[%d] %s", s.id, ErrorMessageMissingResult)
	}

	if result.Type() == fastjson.TypeNumber && result.GetInt() == PingSuccessValue {
		s.ping = PingSuccessValue
		// logServerSuccess(s.id, "ping")
	} else {
		s.ping = PingFailureValue
		logServerError(s.id, "ping", fmt.Errorf("returned non-success value: %d", result.GetInt()))
	}
	return nil
}

func (s *Server) server_GetBlock(coin string, blockHash string) (*fastjson.Value, error) {
	payloadMethod := "getblock"
	payloadParams := []interface{}{coin, blockHash, "true"}

	response, err := s.makeHTTPRequest(http.MethodPost, payloadMethod, payloadParams)
	if err != nil {
		logServerError(s.id, ErrorMessageGetBlockFailed, err)
		return nil, fmt.Errorf("server[%d] %s for coin %s: %w", s.id, ErrorMessageGetBlockFailed, coin, err)
	}

	jsonResp, err := parseJSON(response)
	if err != nil {
		logServerError(s.id, ErrorMessageJSONParseFailed, err)
		return nil, fmt.Errorf("server[%d] %s for coin %s: %w", s.id, ErrorMessageJSONParseFailed, coin, err)
	}

	jsonError := jsonResp.Get("error")
	if jsonError.Type() != fastjson.TypeNull {
		errorMsg := jsonError.String()
		logServerError(s.id, ErrorMessageGetBlockFailed, fmt.Errorf("coin %s: %s", coin, errorMsg))
		return nil, fmt.Errorf("server[%d] %s for coin %s: %s", s.id, ErrorMessageGetBlockFailed, coin, errorMsg)
	}

	jsonResult := jsonResp.Get("result")
	if jsonResult.Type() != fastjson.TypeObject {
		logServerError(s.id, ErrorMessageGetBlockFailed, fmt.Errorf("coin %s: invalid result type", coin))
		return nil, fmt.Errorf("server[%d] %s for coin %s: invalid result type", s.id, ErrorMessageGetBlockFailed, coin)
	}

	// logServerSuccess(s.id, "getblock", fmt.Sprintf("coin %s, hash %s", coin, blockHash))
	return jsonResult, nil
}

func (s *Server) server_GetBlockHash(coin string, height int) (string, error) {
	payloadMethod := "getblockhash"
	payloadParams := []interface{}{coin, height}

	if height == InvalidHeightValue {
		logServerError(s.id, ErrorMessageBlockHashFailed, fmt.Errorf("invalid height %d for coin %s", height, coin))
		return "", nil
	}

	response, err := s.makeHTTPRequest(http.MethodPost, payloadMethod, payloadParams)
	if err != nil {
		logServerError(s.id, ErrorMessageBlockHashFailed, err)
		return "", fmt.Errorf("server[%d] %s for coin %s height %d: %w", s.id, ErrorMessageBlockHashFailed, coin, height, err)
	}

	jsonResp, err := parseJSON(response)
	if err != nil {
		logServerError(s.id, ErrorMessageJSONParseFailed, err)
		return "", fmt.Errorf("server[%d] %s for coin %s height %d: %w", s.id, ErrorMessageJSONParseFailed, coin, height, err)
	}

	jsonError := jsonResp.Get("error")
	if jsonError.Type() != fastjson.TypeNull {
		errorMsg := jsonError.String()
		logServerError(s.id, ErrorMessageBlockHashFailed, fmt.Errorf("coin %s height %d: %s", coin, height, errorMsg))
		return "", fmt.Errorf("server[%d] %s for coin %s height %d: %s", s.id, ErrorMessageBlockHashFailed, coin, height, errorMsg)
	}

	result := jsonResp.Get("result").String()
	if result == "" {
		logServerError(s.id, ErrorMessageBlockHashFailed, fmt.Errorf("coin %s height %d: empty result", coin, height))
		return "", fmt.Errorf("server[%d] %s for coin %s height %d: empty result", s.id, ErrorMessageBlockHashFailed, coin, height)
	}

	hash := removeNonPrintableChars(result)
	// logServerSuccess(s.id, "getblockhash", fmt.Sprintf("coin %s height %d: %s", coin, height, hash))
	return hash, nil
}

func (s *Server) server_GetFees() error {
	payloadMethod := "fees"
	payloadParams := []interface{}{}
	response, err := s.makeHTTPRequest(http.MethodPost, payloadMethod, payloadParams)
	if err != nil {
		s.getfees = getDefaultJSONResponse()
		logServerError(s.id, ErrorMessageGetFeesFailed, err)
		return fmt.Errorf("server[%d] %s: %w", s.id, ErrorMessageGetFeesFailed, err)
	}
	jsonResp, err := parseJSON(response)
	if err != nil {
		s.getfees = getDefaultJSONResponse()
		logServerError(s.id, ErrorMessageJSONParseFailed, err)
		return fmt.Errorf("server[%d] %s: %w", s.id, ErrorMessageJSONParseFailed, err)
	}
	s.getfees = jsonResp
	// logServerSuccess(s.id, "getfees")
	return nil
}

func (s *Server) server_GetHeights() error {
	payloadMethod := "heights"
	payloadParams := []interface{}{}
	response, err := s.makeHTTPRequest(http.MethodPost, payloadMethod, payloadParams)
	if err != nil {
		s.getheights = getDefaultJSONResponse()
		logServerError(s.id, ErrorMessageGetHeightsFailed, err)
		return fmt.Errorf("server[%d] %s: %w", s.id, ErrorMessageGetHeightsFailed, err)
	}
	jsonResp, err := parseJSON(response)
	if err != nil {
		s.getheights = getDefaultJSONResponse()
		logServerError(s.id, ErrorMessageJSONParseFailed, err)
		return fmt.Errorf("server[%d] %s: %w", s.id, ErrorMessageJSONParseFailed, err)
	}
	s.getheights = jsonResp
	s.sortGetHeightsKeys()
	// logServerSuccess(s.id, "getheights")
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
