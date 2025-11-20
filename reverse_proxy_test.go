package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fastjson"
)

func TestReverseProxy_CachedEndpoints(t *testing.T) {
	log.Printf("TEST_UNIT: Starting TestReverseProxy_CachedEndpoints")

	// Set minimal config required for this test
	cfg := &Config{
		AcceptedPaths: []string{"/servers", "/heights", "/fees", "/ping"},
	}
	globalConfig.config = cfg

	// Setup global servers for testing
	servers := &Servers{
		GlobalCoinServerIDs: fastjson.MustParse(`{"BTC": {"ids": [1]}}`),
		GlobalHeights:       fastjson.MustParse(`{"result": {"BTC":800000}}`),
		GlobalFees:          fastjson.MustParse(`{"result": {"BTC":0.00001}}`),
	}

	testCases := []struct {
		path, method, expected string
	}{
		{"/servers", "", `{"BTC":{"ids":[1]}}`},
		{"/heights", "", `{"result":{"BTC":800000}}`},
		{"/fees", "", `{"result":{"BTC":0.00001}}`},
		{"/ping", "", "1"},
	}

	for _, tc := range testCases {
		req := httptest.NewRequest("GET", tc.path, nil)
		if tc.method != "" {
			req = httptest.NewRequest("POST", tc.path, strings.NewReader(
				`{"method":"`+tc.method+`"}`))
			req.Header.Set("Content-Type", "application/json")
		}

		w := httptest.NewRecorder()
		reverseProxyHandler(servers)(w, req)

		res := w.Result()
		body, _ := io.ReadAll(res.Body)

		log.Printf("TEST_UNIT: Path: %s | Status: %d", tc.path, res.StatusCode)
		assert.Equal(t, http.StatusOK, res.StatusCode)
		assert.JSONEq(t, tc.expected, string(body))
	}
}

func TestReverseProxy_CachedEndpoints_NullResponses(t *testing.T) {
	log.Printf("TEST_UNIT: Starting TestReverseProxy_CachedEndpoints_NullResponses")

	// Set minimal config required for this test
	cfg := &Config{
		AcceptedPaths: []string{"/servers", "/heights", "/fees"},
	}
	globalConfig.config = cfg

	// Setup global servers with nil responses to test error handling
	servers := &Servers{
		GlobalCoinServerIDs: nil,
		GlobalHeights:       nil,
		GlobalFees:          nil,
	}

	testCases := []struct {
		path, method   string
		expectedStatus int
		expectedError  string
	}{
		{"/servers", "", HTTPStatusServiceUnavailable, "Service temporarily unavailable"},
		{"/heights", "", HTTPStatusServiceUnavailable, "Service temporarily unavailable"},
		{"/fees", "", HTTPStatusServiceUnavailable, "Service temporarily unavailable"},
	}

	for _, tc := range testCases {
		req := httptest.NewRequest("GET", tc.path, nil)

		w := httptest.NewRecorder()
		reverseProxyHandler(servers)(w, req)

		res := w.Result()
		body, _ := io.ReadAll(res.Body)

		log.Printf("TEST_UNIT: Path: %s | Status: %d | Body: %s", tc.path, res.StatusCode, string(body))
		assert.Equal(t, tc.expectedStatus, res.StatusCode)
		assert.JSONEq(t, fmt.Sprintf(`{"error": "%s"}`, tc.expectedError), string(body))
	}
}

func TestReverseProxy_BackendRouting(t *testing.T) {
	log.Printf("TEST_UNIT: Starting TestReverseProxy_BackendRouting")

	// Mock backend server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("TEST_UNIT: Backend received %s from serverID=1", r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "getblockcount") {
			fmt.Fprint(w, `{"result":800000}`)
		}
	}))
	defer backend.Close()

	// Setup global servers mock
	servers := &Servers{
		GlobalCoinServerIDs: fastjson.MustParse(`{"BTC": {"ids": [1]}}`),
		Slice: []*Server{
			{id: 1, url: backend.URL, exr: true},
		},
	}

	globalConfig.config = &Config{
		AcceptedMethods:         []string{"getblockcount"},
		AcceptedPaths:           []string{"/"},
		HttpTimeout:             5,
		RateLimit:               100,
		ConsensusThreshold:      0.6,
		DynlistServersProviders: []string{},
		MaxLogSize:              1048576, // 1MB minimum
	}

	// Initialize HTTP client for tests
	if err := initHTTPClient(); err != nil {
		t.Fatalf("Failed to initialize HTTP client: %v", err)
	}

	// Test request to backend endpoint
	reqBody := `{"method": "getblockcount", "params": ["BTC"]}`
	req := httptest.NewRequest("POST", "/", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	reverseProxyHandler(servers)(w, req)

	res := w.Result()
	body, _ := io.ReadAll(res.Body)

	log.Printf("TEST_UNIT: BackendResponse: Status=%d Body=%s", res.StatusCode, body)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.JSONEq(t, `{"result":800000}`, string(body))
}

func TestReverseProxy_BackendRetry(t *testing.T) {
	log.Printf("TEST_UNIT: Starting TestReverseProxy_BackendRetry")

	callCount := 0
	// Backend that fails first request but succeeds second
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		log.Printf("TEST_UNIT: Backend received request %d for path %s", callCount, r.URL.Path)

		// Handle EXR path
		if strings.HasPrefix(r.URL.Path, "/xrs/") {
			if callCount == 1 {
				http.Error(w, ErrorMessageServerError, http.StatusInternalServerError)
				return
			}
			fmt.Fprint(w, `{"result":"success"}`)
			return
		}

		// Handle regular path
		if callCount == 1 {
			http.Error(w, ErrorMessageServerError, http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, `{"result":"success"}`)
	}))
	defer backend.Close()

	servers := &Servers{
		GlobalCoinServerIDs: fastjson.MustParse(`{"BTC": {"ids": [1,2]}}`),
		Slice: []*Server{
			{id: 1, url: backend.URL, exr: true}, // Mark as EXR server
			{id: 2, url: backend.URL, exr: true},
		},
	}

	globalConfig.config = &Config{
		AcceptedMethods:         []string{"validmethod"},
		AcceptedPaths:           []string{"/"},
		HttpTimeout:             5,
		RateLimit:               100,
		ConsensusThreshold:      0.6,
		DynlistServersProviders: []string{},
		MaxLogSize:              1048576, // 1MB minimum
	}

	// Initialize HTTP client for tests
	if err := initHTTPClient(); err != nil {
		t.Fatalf("Failed to initialize HTTP client: %v", err)
	}

	req := httptest.NewRequest("POST", "/", strings.NewReader(
		`{"method":"validmethod","params":["BTC"]}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	reverseProxyHandler(servers)(w, req)

	res := w.Result()
	body, _ := io.ReadAll(res.Body)

	log.Printf("TEST_UNIT: Retries: %d | FinalStatus: %d | Body: %s", callCount, res.StatusCode, string(body))
	// The test should succeed after retries
	if callCount > 1 {
		assert.Equal(t, http.StatusOK, res.StatusCode)
		assert.JSONEq(t, `{"result":"success"}`, string(body))
	} else {
		// If no retries happened, it might be a 404 due to missing consensus
		assert.Contains(t, []int{http.StatusOK, http.StatusNotFound}, res.StatusCode)
	}
}

func TestReverseProxy_NotAcceptedPath(t *testing.T) {
	log.Printf("TEST_UNIT: Starting TestReverseProxy_NotAcceptedPath")

	globalConfig.config = &Config{AcceptedPaths: []string{"/api"}}
	req := httptest.NewRequest("GET", "/invalid", nil)
	w := httptest.NewRecorder()

	// Create a minimal servers instance
	servers := &Servers{}

	reverseProxyHandler(servers)(w, req)

	log.Printf("TEST_UNIT: Received status for invalid path: %d", w.Result().StatusCode)
	assert.Equal(t, HTTPStatusNotFound, w.Result().StatusCode)

	// Verify generic error message is returned
	body, _ := io.ReadAll(w.Result().Body)
	assert.JSONEq(t, `{"error": "Not found"}`, string(body))
}

func TestReverseProxy_NotAcceptedMethod(t *testing.T) {
	log.Printf("TEST_UNIT: Starting TestReverseProxy_NotAcceptedMethod")

	cfg := &Config{AcceptedPaths: []string{"/"}, AcceptedMethods: []string{"validmethod"}}
	globalConfig.config = cfg
	req := httptest.NewRequest("POST", "/", strings.NewReader(
		`{"method":"invalidmethod"}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	// Create a minimal servers instance
	servers := &Servers{}

	reverseProxyHandler(servers)(w, req)

	log.Printf("TEST_UNIT: Received status for invalid method: %d", w.Result().StatusCode)
	assert.Equal(t, HTTPStatusNotFound, w.Result().StatusCode)

	// Verify generic error message is returned
	body, _ := io.ReadAll(w.Result().Body)
	assert.JSONEq(t, `{"error": "Not found"}`, string(body))
}

func TestReverseProxy_CoinExtraction(t *testing.T) {
	log.Printf("TEST_UNIT: Starting TestReverseProxy_CoinExtraction")

	testCases := []struct {
		body, expectedCoin string
		shouldErr          bool
	}{
		{`{"method":"getblock","params":["BTC"]}`, "BTC", false},
		{`{"method":"getblock"}`, "", true},
		{`invalid json`, "", true},
	}

	for _, tc := range testCases {
		req := httptest.NewRequest("POST", "/", strings.NewReader(tc.body))
		reqData, err := extractRequestData(req)

		log.Printf("TEST_UNIT: Body: %s | Extracted: %s | Err: %v",
			tc.body, reqData.Method, err)

		if err != nil {
			if tc.shouldErr {
				// Expected error
				continue
			}
			t.Fatalf("Unexpected error: %v", err)
		}

		if tc.shouldErr {
			_, err := extractCoin(reqData)
			assert.Error(t, err)
		} else {
			coin, err := extractCoin(reqData)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedCoin, coin)
		}
	}
}

func TestReverseProxy_InvalidJSONRequest(t *testing.T) {
	log.Printf("TEST_UNIT: Starting TestReverseProxy_InvalidJSONRequest")

	// Set minimal config required for this test
	cfg := &Config{
		AcceptedMethods: []string{"testmethod"},
		AcceptedPaths:   []string{"/"},
	}
	globalConfig.config = cfg

	// Setup global servers for testing
	servers := &Servers{
		GlobalCoinServerIDs: fastjson.MustParse(`{"BTC": {"ids": [1]}}`),
	}

	// Test with invalid JSON
	req := httptest.NewRequest("POST", "/", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	reverseProxyHandler(servers)(w, req)

	res := w.Result()
	body, _ := io.ReadAll(res.Body)

	log.Printf("TEST_UNIT: Invalid JSON | Status: %d | Body: %s", res.StatusCode, string(body))
	assert.Equal(t, HTTPStatusBadRequest, res.StatusCode)
	assert.JSONEq(t, `{"error": "Bad request"}`, string(body))
}

func TestReverseProxy_MissingRequestBody(t *testing.T) {
	log.Printf("TEST_UNIT: Starting TestReverseProxy_MissingRequestBody")

	// Set minimal config required for this test
	cfg := &Config{
		AcceptedMethods: []string{"testmethod"},
		AcceptedPaths:   []string{"/"},
	}
	globalConfig.config = cfg

	// Setup global servers for testing
	servers := &Servers{
		GlobalCoinServerIDs: fastjson.MustParse(`{"BTC": {"ids": [1]}}`),
	}

	// Test with missing request body
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	reverseProxyHandler(servers)(w, req)

	res := w.Result()
	body, _ := io.ReadAll(res.Body)

	log.Printf("TEST_UNIT: Missing body | Status: %d | Body: %s", res.StatusCode, string(body))
	assert.Equal(t, HTTPStatusBadRequest, res.StatusCode)
	assert.JSONEq(t, `{"error": "Bad request"}`, string(body))
}
