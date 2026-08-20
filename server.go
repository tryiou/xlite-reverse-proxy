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
	"sync/atomic"
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

// Add atomic helper methods
func (s *Server) setPing(value int) {
	atomic.StoreInt64(&s.ping, int64(value))
}

func (s *Server) getPing() int {
	return int(atomic.LoadInt64(&s.ping))
}

func (s *Server) server_GetPing() error {
	payloadMethod := "ping"
	payloadParams := []interface{}{}

	// Use 5-second timeout for ping requests
	response, err := s.makeHTTPRequest(http.MethodPost, payloadMethod, payloadParams, 5)
	if err != nil {
		s.setDefaultResponses()
		return NewServerError(s.id, "ping", err)
	}

	jsonResp, err := parseJSON(response)
	if err != nil {
		s.setDefaultResponses()
		return NewServerError(s.id, "parse JSON", err)
	}

	result := jsonResp.Get("result")
	if result == nil {
		s.setPing(PingFailureValue)
		s.setDefaultResponses()
		return NewServerError(s.id, "ping", fmt.Errorf("missing result field"))
	}

	if result.Type() == fastjson.TypeNumber && result.GetInt() == PingSuccessValue {
		s.setPing(PingSuccessValue)
	} else {
		s.setPing(PingFailureValue)
		logServerError(s.id, "ping", fmt.Errorf("returned non-success value: %d", result.GetInt()))
	}
	return nil
}

func (s *Server) server_GetBlock(coin string, blockHash string) (*fastjson.Value, error) {
	payloadMethod := "getblock"
	payloadParams := []interface{}{coin, blockHash, "true"}

	response, err := s.makeHTTPRequest(http.MethodPost, payloadMethod, payloadParams)
	if err != nil {
		logServerError(s.id, "getblock failed", err)
		return nil, NewServerError(s.id, "getblock", fmt.Errorf("coin %s: %w", coin, err))
	}

	jsonResp, err := parseJSON(response)
	if err != nil {
		logServerError(s.id, "JSON parse failed", err)
		return nil, NewServerError(s.id, "parse JSON", fmt.Errorf("coin %s: %w", coin, err))
	}

	jsonError := jsonResp.Get("error")
	if jsonError.Type() != fastjson.TypeNull {
		errorMsg := jsonError.String()
		logServerError(s.id, "getblock failed", fmt.Errorf("coin %s: %s", coin, errorMsg))
		return nil, NewServerError(s.id, "getblock", fmt.Errorf("coin %s: %s", coin, errorMsg))
	}

	jsonResult := jsonResp.Get("result")
	if jsonResult.Type() != fastjson.TypeObject {
		logServerError(s.id, "getblock failed", fmt.Errorf("coin %s: invalid result type", coin))
		return nil, NewServerError(s.id, "getblock", fmt.Errorf("coin %s: invalid result type", coin))
	}

	return jsonResult, nil
}

func (s *Server) server_GetBlockHash(coin string, height int) (string, error) {
	payloadMethod := "getblockhash"
	payloadParams := []interface{}{coin, height}

	if height == InvalidHeightValue {
		logServerError(s.id, "getblockhash failed", fmt.Errorf("invalid height %d", height))
		return "", nil
	}

	response, err := s.makeHTTPRequest(http.MethodPost, payloadMethod, payloadParams)
	if err != nil {
		logServerError(s.id, "getblockhash failed", err)
		return "", NewServerError(s.id, "getblockhash", fmt.Errorf("coin %s height %d: %w", coin, height, err))
	}

	jsonResp, err := parseJSON(response)
	if err != nil {
		logServerError(s.id, "JSON parse failed", err)
		return "", NewServerError(s.id, "parse JSON", fmt.Errorf("coin %s height %d: %w", coin, height, err))
	}

	jsonError := jsonResp.Get("error")
	if jsonError.Type() != fastjson.TypeNull {
		errorMsg := jsonError.String()
		logServerError(s.id, "getblockhash failed", fmt.Errorf("coin %s height %d: %s", coin, height, errorMsg))
		return "", NewServerError(s.id, "getblockhash", fmt.Errorf("coin %s height %d: %s", coin, height, errorMsg))
	}

	result := jsonResp.Get("result").String()
	if result == "" {
		logServerError(s.id, "getblockhash failed", fmt.Errorf("coin %s height %d: empty result", coin, height))
		return "", NewServerError(s.id, "getblockhash", fmt.Errorf("coin %s height %d: empty result", coin, height))
	}

	hash := removeNonPrintableChars(result)
	return hash, nil
}

func (s *Server) server_GetFees() error {
	payloadMethod := "fees"
	payloadParams := []interface{}{}
	response, err := s.makeHTTPRequest(http.MethodPost, payloadMethod, payloadParams)
	if err != nil {
		s.getfees = getDefaultJSONResponse()
		logServerError(s.id, "getfees failed", err)
		return NewServerError(s.id, "getfees", err)
	}
	jsonResp, err := parseJSON(response)
	if err != nil {
		s.getfees = getDefaultJSONResponse()
		logServerError(s.id, "JSON parse failed", err)
		return NewServerError(s.id, "parse JSON", err)
	}
	s.getfees = jsonResp
	return nil
}

func (s *Server) server_GetFees_Concurrent() (*fastjson.Value, error) {
	payloadMethod := "fees"
	payloadParams := []interface{}{}
	response, err := s.makeHTTPRequest(http.MethodPost, payloadMethod, payloadParams)
	if err != nil {
		return getDefaultJSONResponse(), fmt.Errorf("server[%d] %s: %w", s.id, ErrorMessageGetFeesFailed, err)
	}
	jsonResp, err := parseJSON(response)
	if err != nil {
		return getDefaultJSONResponse(), fmt.Errorf("server[%d] %s: %w", s.id, ErrorMessageJSONParseFailed, err)
	}
	return jsonResp, nil
}

func (s *Server) server_GetHeights() error {
	payloadMethod := "heights"
	payloadParams := []interface{}{}
	response, err := s.makeHTTPRequest(http.MethodPost, payloadMethod, payloadParams)
	if err != nil {
		s.getheights = getDefaultJSONResponse()
		logServerError(s.id, "getheights failed", err)
		return NewServerError(s.id, "getheights", err)
	}
	jsonResp, err := parseJSON(response)
	if err != nil {
		s.getheights = getDefaultJSONResponse()
		logServerError(s.id, "JSON parse failed", err)
		return NewServerError(s.id, "parse JSON", err)
	}
	s.getheights = jsonResp
	s.sortGetHeightsKeys()
	return nil
}

func (s *Server) server_GetHeights_Concurrent() (*fastjson.Value, error) {
	payloadMethod := "heights"
	payloadParams := []interface{}{}
	response, err := s.makeHTTPRequest(http.MethodPost, payloadMethod, payloadParams)
	if err != nil {
		return getDefaultJSONResponse(), fmt.Errorf("server[%d] %s: %w", s.id, ErrorMessageGetHeightsFailed, err)
	}
	jsonResp, err := parseJSON(response)
	if err != nil {
		return getDefaultJSONResponse(), fmt.Errorf("server[%d] %s: %w", s.id, ErrorMessageJSONParseFailed, err)
	}
	return jsonResp, nil
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

// Add context support to HTTP requests
func (s *Server) makeHTTPRequestWithContext(ctx context.Context, httpMethod, payloadMethod string, payloadParams []interface{}, timeoutSeconds ...int) ([]byte, error) {
	var (
		url     string
		payload string
	)

	cfg := globalConfig.GetConfig()

	// Use provided timeout or default from config
	timeout := cfg.HttpTimeout
	if len(timeoutSeconds) > 0 {
		timeout = timeoutSeconds[0]
	}

	// Validate server configuration
	if s.url == "" {
		return nil, NewServerError(s.id, "validate server config", fmt.Errorf("empty URL"))
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
			return nil, NewServerError(s.id, "marshal payload", fmt.Errorf("method %s: %w", payloadMethod, err))
		}
		payload = string(payloadBytes)
	} else {
		// EXR NODE!
		url = s.url + "/xrs/" + payloadMethod
		payloadBytes, err := json.Marshal(payloadParams)
		if err != nil {
			return nil, NewServerError(s.id, "marshal payload", fmt.Errorf("EXR method %s: %w", payloadMethod, err))
		}
		payload = string(payloadBytes)
	}

	reqTimer := time.Now()

	// Validate URL format
	if !strings.Contains(url, "://") {
		return nil, NewServerError(s.id, "validate URL", fmt.Errorf("invalid URL format: %s", url))
	}

	req, err := http.NewRequestWithContext(ctx, httpMethod, url, strings.NewReader(payload))
	if err != nil {
		elapsed := time.Since(reqTimer)
		return nil, NewServerError(s.id, "create HTTP request", fmt.Errorf("url: %s, elapsed: %v: %w", url, elapsed, err))
	}

	req.Header.Set(HeaderContentType, ContentTypeJSON)

	// Use the determined timeout
	if ctx == context.Background() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()
		req = req.WithContext(ctx)
	}

	client := globalConfig.GetClient()
	res, err := client.Do(req)
	if err != nil {
		elapsed := time.Since(reqTimer)

		// Provide specific error messages based on error type
		if netErr, ok := err.(net.Error); ok {
			if netErr.Timeout() {
				return nil, NewServerError(s.id, "HTTP request", fmt.Errorf("timeout after %v: %w", elapsed, err))
			}
			if netErr.Temporary() {
				return nil, NewServerError(s.id, "HTTP request", fmt.Errorf("temporary network error: %w", err))
			}
		}

		// Handle connection-specific errors
		if strings.Contains(err.Error(), "connection refused") {
			return nil, NewServerError(s.id, "HTTP request", fmt.Errorf("connection refused: %w", err))
		}
		if strings.Contains(err.Error(), "no such host") {
			return nil, NewServerError(s.id, "HTTP request", fmt.Errorf("DNS resolution failed: %w", err))
		}

		return nil, NewServerError(s.id, "HTTP request", err)
	}
	defer res.Body.Close()

	// Check HTTP status code
	if res.StatusCode != HTTPStatusOK {
		elapsed := time.Since(reqTimer)
		return nil, NewHTTPError(res.StatusCode, "HTTP response", fmt.Errorf("status: %s, elapsed: %v", res.Status, elapsed))
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, NewServerError(s.id, "read response body", err)
	}

	return body, nil
}

func (s *Server) makeHTTPRequest(httpMethod, payloadMethod string, payloadParams []interface{}, timeoutSeconds ...int) ([]byte, error) {
	ctx := context.Background()
	return s.makeHTTPRequestWithContext(ctx, httpMethod, payloadMethod, payloadParams, timeoutSeconds...)
}
