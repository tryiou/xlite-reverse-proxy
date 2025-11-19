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
	config = &Config{
		AcceptedPaths: []string{"/servers", "/heights", "/fees", "/ping"},
	}

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

	config = &Config{
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
		if callCount == 1 {
			http.Error(w, ErrorMessageServerError, http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, `{"result":"success"}`)
	}))

	servers := &Servers{
		GlobalCoinServerIDs: fastjson.MustParse(`{"BTC": {"ids": [1,2]}}`),
		Slice: []*Server{
			{id: 1, url: backend.URL},
			{id: 2, url: backend.URL},
		},
	}

	config = &Config{
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

	log.Printf("TEST_UNIT: Retries: %d | FinalStatus: %d", callCount, res.StatusCode)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.JSONEq(t, `{"result":"success"}`, string(body))
}

func TestReverseProxy_NotAcceptedPath(t *testing.T) {
	log.Printf("TEST_UNIT: Starting TestReverseProxy_NotAcceptedPath")

	config = &Config{AcceptedPaths: []string{"/api"}}
	req := httptest.NewRequest("GET", "/invalid", nil)
	w := httptest.NewRecorder()

	// Create a minimal servers instance
	servers := &Servers{}

	reverseProxyHandler(servers)(w, req)

	log.Printf("TEST_UNIT: Received status for invalid path: %d", w.Result().StatusCode)
	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode)
}

func TestReverseProxy_NotAcceptedMethod(t *testing.T) {
	log.Printf("TEST_UNIT: Starting TestReverseProxy_NotAcceptedMethod")

	config = &Config{AcceptedMethods: []string{"validmethod"}}
	req := httptest.NewRequest("POST", "/", strings.NewReader(
		`{"method":"invalidmethod"}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	// Create a minimal servers instance
	servers := &Servers{}

	reverseProxyHandler(servers)(w, req)

	log.Printf("TEST_UNIT: Received status for invalid method: %d", w.Result().StatusCode)
	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode)
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
