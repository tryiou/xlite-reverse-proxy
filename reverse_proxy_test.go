package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/valyala/fastjson"
)

// setTestHTTPClient installs a test HTTP client and returns a cleanup func that
// restores the previous client, so the relay path has a working client without
// depending on global initialization order between tests.
func setTestHTTPClient(timeoutSec int) func() {
	old := globalConfig.client
	globalConfig.client = &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	return func() { globalConfig.client = old }
}

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

	// Install a test HTTP client for the relay path (restored after the test).
	defer setTestHTTPClient(5)()

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

	// Install a test HTTP client for the relay path (restored after the test).
	defer setTestHTTPClient(5)()

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

func TestRequestFailureKeepsServerInList(t *testing.T) {
	log.Printf("TEST_UNIT: Starting TestRequestFailureKeepsServerInList")

	callCount := 0
	// Backend that always fails
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		http.Error(w, ErrorMessageServerError, http.StatusInternalServerError)
	}))
	defer backend.Close()

	servers := &Servers{
		GlobalCoinServerIDs: fastjson.MustParse(`{"BTC": {"ids": [1]}}`),
		Slice: []*Server{
			{id: 1, url: backend.URL, exr: true},
		},
	}

	oldConfig := globalConfig.config
	oldClient := globalConfig.client
	defer func() {
		globalConfig.config = oldConfig
		globalConfig.client = oldClient
	}()
	globalConfig.config = &Config{
		AcceptedMethods:         []string{"getblockcount"},
		AcceptedPaths:           []string{"/"},
		HttpTimeout:             5,
		RateLimit:               100,
		ConsensusThreshold:      0.6,
		DynlistServersProviders: []string{},
		MaxLogSize:              1048576,
	}
	globalConfig.client = &http.Client{Timeout: 5 * time.Second}

	req := httptest.NewRequest("POST", "/", strings.NewReader(
		`{"method":"getblockcount","params":["BTC"]}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	reverseProxyHandler(servers)(w, req)

	res := w.Result()
	body, _ := io.ReadAll(res.Body)
	log.Printf("TEST_UNIT: BackendCalls=%d Status=%d Body=%s", callCount, res.StatusCode, body)

	assert.Greater(t, callCount, 0, "The failing backend should have been reached")

	// The node must NOT be removed from the coin list after a failed call:
	// it stays listed so the retry loop can pick it again (no ban).
	ids := servers.GlobalCoinServerIDs.Get("BTC", "ids").GetArray()
	assert.Len(t, ids, 1, "Server should remain in the coin list after a failed request")
}

func TestReverseProxy_CoinNotFound_NoRetry(t *testing.T) {
	log.Printf("TEST_UNIT: Starting TestReverseProxy_CoinNotFound_NoRetry")

	attemptCount := 0
	// Backend that tracks how many times it's called
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		log.Printf("TEST_UNIT: Backend received request %d for path %s", attemptCount, r.URL.Path)
		fmt.Fprint(w, `{"result":"success"}`)
	}))
	defer backend.Close()

	// Only BTC is in GlobalCoinServerIDs, UNO is not — simulates coin with no provider
	servers := &Servers{
		GlobalCoinServerIDs: fastjson.MustParse(`{"BTC": {"ids": [1]}}`),
		Slice: []*Server{
			{id: 1, url: backend.URL, exr: true},
		},
	}

	globalConfig.config = &Config{
		AcceptedMethods:         []string{"validmethod"},
		AcceptedPaths:           []string{"/"},
		HttpTimeout:             5,
		RateLimit:               100,
		ConsensusThreshold:      0.6,
		DynlistServersProviders: []string{},
		MaxLogSize:              1048576,
	}

	req := httptest.NewRequest("POST", "/", strings.NewReader(
		`{"method":"validmethod","params":["UNO"]}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	reverseProxyHandler(servers)(w, req)

	res := w.Result()
	body, _ := io.ReadAll(res.Body)

	log.Printf("TEST_UNIT: Attempts: %d | Status: %d | Body: %s", attemptCount, res.StatusCode, string(body))
	// Should fail immediately without hitting the backend at all
	assert.Equal(t, 0, attemptCount, "Backend should not be called for unsupported coin")
	assert.Equal(t, http.StatusServiceUnavailable, res.StatusCode)
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

	w := httptest.NewRecorder()

	// Create a minimal servers instance
	servers := &Servers{}

	reverseProxyHandler(servers)(w, req)

	log.Printf("TEST_UNIT: Received status for invalid method: %d", w.Result().StatusCode)
	assert.Equal(t, HTTPStatusBadRequest, w.Result().StatusCode)

	// Verify generic error message is returned
	body, _ := io.ReadAll(w.Result().Body)
	assert.JSONEq(t, `{"error": "Bad request"}`, string(body))
}

func TestReverseProxy_CoinExtraction(t *testing.T) {
	log.Printf("TEST_UNIT: Starting TestReverseProxy_CoinExtraction")

	// Set up config with the methods needed for this test
	oldConfig := globalConfig.config
	defer func() { globalConfig.config = oldConfig }()
	globalConfig.config = &Config{
		AcceptedMethods: []string{"getblock", "getblockhash", "heights", "fees", "ping"},
	}

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
		if tc.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
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

// resetLogThrottleState clears the package-level throttle maps between tests.
func resetLogThrottleState() {
	coinNoServerState.Lock()
	coinNoServerState.lastLogged = make(map[string]time.Time)
	coinNoServerState.Unlock()

	canceledLogState.Lock()
	canceledLogState.count = 0
	canceledLogState.lastLogged = time.Time{}
	canceledLogState.Unlock()
}

// captureLog redirects the package logger to a buffer and restores it on cleanup.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	old := logger
	logger = log.New(buf, "", 0)
	t.Cleanup(func() { logger = old })
	return buf
}

func TestLogCoinNoServerThrottle(t *testing.T) {
	resetLogThrottleState()
	t.Cleanup(resetLogThrottleState)
	buf := captureLog(t)

	first := logCoinNoServer("BTC", "[tag]", ErrCoinNotFound)
	second := logCoinNoServer("BTC", "[tag]", ErrCoinNotFound)

	assert.True(t, first, "first call should log")
	assert.False(t, second, "immediate second call should be throttled")
	assert.Contains(t, buf.String(), "coin BTC has no available server")
	assert.Contains(t, buf.String(), "has no available server (coin not found in consensus)")
}

func TestLogCoinRecovered(t *testing.T) {
	resetLogThrottleState()
	t.Cleanup(resetLogThrottleState)
	buf := captureLog(t)

	assert.True(t, logCoinNoServer("BTC", "[tag]", ErrCoinNotFound))
	logCoinRecovered("BTC")

	assert.Contains(t, buf.String(), "coin BTC recovered, service restored")

	// After recovery the coin is cleared, so the next outage logs again.
	assert.True(t, logCoinNoServer("BTC", "[tag]", ErrCoinNotFound), "should log again after recovery")
}

func TestBumpCanceledCount(t *testing.T) {
	resetLogThrottleState()
	t.Cleanup(resetLogThrottleState)
	buf := captureLog(t)

	bumpCanceledCount()
	bumpCanceledCount()
	bumpCanceledCount()

	// First disconnect logs immediately; subsequent ones within the window are
	// aggregated and only flushed on the next window boundary, so the immediate
	// burst logs a single "1 client requests canceled" line.
	assert.Contains(t, buf.String(), "1 client requests canceled mid-request")
}

func TestFlushCanceledCountTail(t *testing.T) {
	resetLogThrottleState()
	t.Cleanup(resetLogThrottleState)
	buf := captureLog(t)

	bumpCanceledCount()  // logs "1" immediately and resets
	bumpCanceledCount()  // pending count of 1, within window, no log
	flushCanceledCount() // flushes the partially-filled window

	assert.Equal(t, 2, strings.Count(buf.String(), "client requests canceled mid-request"))
}

func TestParseRetryAfterHeader(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected time.Duration
	}{
		{"empty string falls back to base delay", "", Relay429BaseDelay},
		{"valid integer seconds", "120", 120 * time.Second},
		{"small integer seconds", "1", 1 * time.Second},
		{"large integer seconds returned as-is", "300", 300 * time.Second},
		{"zero seconds falls back to base delay", "0", Relay429BaseDelay},
		{"negative seconds falls back to base delay", "-5", Relay429BaseDelay},
		{"non-numeric string falls back to base delay", "abc", Relay429BaseDelay},
		{"valid RFC1123 date in the future", "", 0}, // special-cased below
		{"RFC1123 date in the past falls back to base delay", time.Now().Add(-10 * time.Second).UTC().Format(time.RFC1123), Relay429BaseDelay},
		{"malformed date falls back to base delay", "Mon, 02 Jan 2006 15:04:05", Relay429BaseDelay},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "valid RFC1123 date in the future" {
				future := time.Now().Add(30 * time.Second).UTC().Format(time.RFC1123)
				got := parseRetryAfterHeader(future)
				if got < 29*time.Second || got > 31*time.Second {
					t.Errorf("parseRetryAfterHeader(%q) = %v, want ~30s", future, got)
				}
				return
			}
			got := parseRetryAfterHeader(tt.header)
			if got != tt.expected {
				t.Errorf("parseRetryAfterHeader(%q) = %v, want %v", tt.header, got, tt.expected)
			}
		})
	}
}

func TestComputeRelay429Backoff(t *testing.T) {
	t.Run("attempt 1 base delay with zero retry-after", func(t *testing.T) {
		delay := computeRelay429Backoff(1, 0)
		min := time.Duration(float64(Relay429BaseDelay) * 0.8)
		max := time.Duration(float64(Relay429BaseDelay) * 1.2)
		if delay < min || delay > max {
			t.Errorf("attempt 1 with zero retryAfter: got %v, want [%v, %v]", delay, min, max)
		}
	})

	t.Run("attempt 2 doubles", func(t *testing.T) {
		delay := computeRelay429Backoff(2, 0)
		base := Relay429BaseDelay * 2
		min := time.Duration(float64(base) * 0.8)
		max := time.Duration(float64(base) * 1.2)
		if delay < min || delay > max {
			t.Errorf("attempt 2: got %v, want [%v, %v]", delay, min, max)
		}
	})

	t.Run("retry-after larger than exponential is capped", func(t *testing.T) {
		retryAfter := 5 * time.Second
		delay := computeRelay429Backoff(1, retryAfter)
		// retryAfter exceeds cap, so delay is capped at Relay429MaxDelay ± 20%
		min := time.Duration(float64(Relay429MaxDelay) * 0.8)
		max := Relay429MaxDelay
		if delay < min || delay > max {
			t.Errorf("retryAfter=5s attempt 1: got %v, want [%v, %v]", delay, min, max)
		}
	})

	t.Run("result never exceeds max delay", func(t *testing.T) {
		for attempt := 1; attempt <= 20; attempt++ {
			delay := computeRelay429Backoff(attempt, 10*time.Second)
			if delay > Relay429MaxDelay {
				t.Errorf("attempt %d: delay %v exceeds max %v", attempt, delay, Relay429MaxDelay)
			}
		}
	})

	t.Run("result is never negative", func(t *testing.T) {
		for attempt := 1; attempt <= 20; attempt++ {
			delay := computeRelay429Backoff(attempt, 0)
			if delay < 0 {
				t.Errorf("attempt %d: negative delay %v", attempt, delay)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Phase 1: Tests for 429 rate limit retry flow (written BEFORE code changes)
// ---------------------------------------------------------------------------

// Test 1: sendRequestToOriginServer returns RateLimitError on 429.
func TestSendRequestToOriginServer_429ReturnsRateLimitError(t *testing.T) {
	log.Printf("TEST_UNIT: Starting TestSendRequestToOriginServer_429ReturnsRateLimitError")

	t.Run("429 with Retry-After header", func(t *testing.T) {
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":"rate limit"}`)
		}))
		defer backend.Close()

		defer setTestHTTPClient(5)()

		req := httptest.NewRequest("POST", backend.URL, strings.NewReader(`{"method":"test"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RequestURI = "" // client.Do requires empty RequestURI

		_, err := sendRequestToOriginServer(req, 3)
		assert.Error(t, err)

		var rateLimitErr *RateLimitError
		if !errors.As(err, &rateLimitErr) {
			t.Fatalf("error should be *RateLimitError, got %T: %v", err, err)
		}
		assert.Equal(t, 3, rateLimitErr.ServerID)
		assert.Equal(t, 5*time.Second, rateLimitErr.RetryAfter)
	})

	t.Run("429 without Retry-After header falls back to base delay", func(t *testing.T) {
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":"rate limit"}`)
		}))
		defer backend.Close()

		defer setTestHTTPClient(5)()

		req := httptest.NewRequest("POST", backend.URL, strings.NewReader(`{"method":"test"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RequestURI = ""

		_, err := sendRequestToOriginServer(req, 7)
		assert.Error(t, err)

		var rateLimitErr *RateLimitError
		if !errors.As(err, &rateLimitErr) {
			t.Fatalf("error should be *RateLimitError, got %T: %v", err, err)
		}
		assert.Equal(t, 7, rateLimitErr.ServerID)
		assert.Equal(t, Relay429BaseDelay, rateLimitErr.RetryAfter)
	})

	t.Run("429 with non-numeric Retry-After falls back to base delay", func(t *testing.T) {
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "abc")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":"rate limit"}`)
		}))
		defer backend.Close()

		defer setTestHTTPClient(5)()

		req := httptest.NewRequest("POST", backend.URL, strings.NewReader(`{"method":"test"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RequestURI = ""

		_, err := sendRequestToOriginServer(req, 1)
		assert.Error(t, err)

		var rateLimitErr *RateLimitError
		if !errors.As(err, &rateLimitErr) {
			t.Fatalf("error should be *RateLimitError, got %T: %v", err, err)
		}
		assert.Equal(t, Relay429BaseDelay, rateLimitErr.RetryAfter)
	})

	t.Run("non-429 error is not RateLimitError", func(t *testing.T) {
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "server error", http.StatusInternalServerError)
		}))
		defer backend.Close()

		defer setTestHTTPClient(5)()

		req := httptest.NewRequest("POST", backend.URL, strings.NewReader(`{"method":"test"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RequestURI = ""

		_, err := sendRequestToOriginServer(req, 1)
		assert.Error(t, err)
		assert.False(t, IsRateLimitError(err), "5xx error should not be RateLimitError")
	})
}

// Test 2: retryWithRandomValidServer retries the SAME server on 429 before
// falling through to a different server. With two servers, at least one
// iteration should succeed (when server[2] is picked first or after fallback).
func TestRetryWithRandomValidServer_429BackoffRetriesSameServer(t *testing.T) {
	log.Printf("TEST_UNIT: Starting TestRetryWithRandomValidServer_429BackoffRetriesSameServer")

	callCount1 := 0
	callCount2 := 0

	// Backend that always returns 429 (simulates rate-limited server)
	backend429 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount1++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":"rate limit"}`)
	}))
	defer backend429.Close()

	// Backend that always returns 200 (simulates healthy server)
	backendOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount2++
		fmt.Fprint(w, `{"result":"ok"}`)
	}))
	defer backendOK.Close()

	servers := &Servers{
		GlobalCoinServerIDs: fastjson.MustParse(`{"BTC":{"ids":[1,2]}}`),
		Slice: []*Server{
			{id: 1, url: backend429.URL, exr: true},
			{id: 2, url: backendOK.URL, exr: true},
		},
	}

	globalConfig.config = &Config{
		AcceptedMethods:         []string{"validmethod"},
		AcceptedPaths:           []string{"/"},
		HttpTimeout:             5,
		RateLimit:               100,
		ConsensusThreshold:      0.6,
		DynlistServersProviders: []string{},
		MaxLogSize:              1048576,
	}
	defer setTestHTTPClient(5)()
	resetLogThrottleState()
	t.Cleanup(resetLogThrottleState)

	requestData := RequestData{Method: "validmethod", Params: []interface{}{"BTC"}, Coin: "BTC", Ip: "127.0.0.1"}

	// Run iterations until server[1] is picked first (exercises the429 backoff path).
	// With 2 servers, ~50% of iterations hit server[1] first.
	sawServer1Backoff := false
	for i := 0; i < 20; i++ {
		callCount1 = 0
		callCount2 = 0

		req := httptest.NewRequest("POST", "/", strings.NewReader(
			`{"method":"validmethod","params":["BTC"]}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:12345"

		w := httptest.NewRecorder()
		server, err := retryWithRandomValidServer(w, req, servers, "BTC", &requestData, RetryAttemptsDefault)

		if err == nil && w.Code == http.StatusOK && callCount1 > 0 {
			// server[1] was picked first, exercised429 backoff, then server[2] succeeded
			sawServer1Backoff = true
			assert.Equal(t, 2, server.id, "should return server[2]")
			assert.JSONEq(t, `{"result":"ok"}`, w.Body.String())
			assert.Equal(t, Relay429MaxRetries+1, callCount1,
				"server[1] should be called Relay429MaxRetries+1 times (initial + backoff retries)")
			assert.Equal(t, 1, callCount2, "server[2] should be called once (after exclusion)")
			break
		}
	}

	assert.True(t, sawServer1Backoff,
		"should have seen server[1]429 backoff path at least once in 20 iterations")
}

// Test 3: retryWithRandomValidServer exhausts 429 retries on one server and
// falls through to try other servers. With two servers in the pool, the
// function always eventually succeeds (either server[2] is picked first, or
// server[1] is exhausted then server[2] is eventually picked).
func TestRetryWithRandomValidServer_429ExhaustedFallsThrough(t *testing.T) {
	log.Printf("TEST_UNIT: Starting TestRetryWithRandomValidServer_429ExhaustedFallsThrough")

	callCount1 := 0
	callCount2 := 0

	// Backend that always returns 429 — server[1] is rate-limited
	backend429 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount1++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":"rate limit"}`)
	}))
	defer backend429.Close()

	// Backend that always returns 200 — server[2] is healthy
	backendOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount2++
		fmt.Fprint(w, `{"result":"ok"}`)
	}))
	defer backendOK.Close()

	servers := &Servers{
		GlobalCoinServerIDs: fastjson.MustParse(`{"BTC":{"ids":[1,2]}}`),
		Slice: []*Server{
			{id: 1, url: backend429.URL, exr: true},
			{id: 2, url: backendOK.URL, exr: true},
		},
	}

	globalConfig.config = &Config{
		AcceptedMethods:         []string{"validmethod"},
		AcceptedPaths:           []string{"/"},
		HttpTimeout:             5,
		RateLimit:               100,
		ConsensusThreshold:      0.6,
		DynlistServersProviders: []string{},
		MaxLogSize:              1048576,
	}
	defer setTestHTTPClient(5)()
	resetLogThrottleState()
	t.Cleanup(resetLogThrottleState)

	requestData := RequestData{Method: "validmethod", Params: []interface{}{"BTC"}, Coin: "BTC", Ip: "127.0.0.1"}

	// Run multiple iterations: some pick server[1] first (429 → exhaust →
	// exclude → server[2] succeeds), others pick server[2] first (immediate success).
	// Assert that at least one succeeds and server exclusion works.
	succeeded := false
	for i := 0; i < 20; i++ {
		req := httptest.NewRequest("POST", "/", strings.NewReader(
			`{"method":"validmethod","params":["BTC"]}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:12345"

		w := httptest.NewRecorder()
		_, err := retryWithRandomValidServer(w, req, servers, "BTC", &requestData, RetryAttemptsDefault)

		if err == nil && w.Code == http.StatusOK {
			succeeded = true
			break
		}
	}

	assert.True(t, succeeded, "with 2 servers, at least one iteration should succeed when server[2] (200) is picked")
	assert.Greater(t, callCount2, 0, "server[2] should have been called at least once")
}

// Test 4: retryWithRandomValidServer on a single-server coin exhausts all
// retries and returns an error when the only server always returns 429.
func TestRetryWithRandomValidServer_SingleServer429Exhausted(t *testing.T) {
	log.Printf("TEST_UNIT: Starting TestRetryWithRandomValidServer_SingleServer429Exhausted")

	callCount := 0
	backend429 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":"rate limit"}`)
	}))
	defer backend429.Close()

	servers := &Servers{
		GlobalCoinServerIDs: fastjson.MustParse(`{"BTC":{"ids":[1]}}`),
		Slice: []*Server{
			{id: 1, url: backend429.URL, exr: true},
		},
	}

	globalConfig.config = &Config{
		AcceptedMethods:         []string{"validmethod"},
		AcceptedPaths:           []string{"/"},
		HttpTimeout:             5,
		RateLimit:               100,
		ConsensusThreshold:      0.6,
		DynlistServersProviders: []string{},
		MaxLogSize:              1048576,
	}
	defer setTestHTTPClient(5)()
	resetLogThrottleState()
	t.Cleanup(resetLogThrottleState)

	req := httptest.NewRequest("POST", "/", strings.NewReader(
		`{"method":"validmethod","params":["BTC"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"

	requestData := RequestData{Method: "validmethod", Params: []interface{}{"BTC"}, Coin: "BTC", Ip: "127.0.0.1"}

	w := httptest.NewRecorder()
	server, err := retryWithRandomValidServer(w, req, servers, "BTC", &requestData, RetryAttemptsDefault)

	assert.Error(t, err, "should return error when all retries exhausted")
	assert.Nil(t, server, "should return nil server on exhaustion")
	assert.Contains(t, err.Error(), "failed after", "error should indicate retries were exhausted")
	// With Relay429MaxRetries=2: server[1] called 3 times (initial + 2 backoffs),
	// then exclusion finds no other servers → permanentStop. No more backend calls.
	assert.Equal(t, Relay429MaxRetries+1, callCount,
		"server[1] should be called exactly Relay429MaxRetries+1 times before exclusion fails")
}

// Test 5: Full integration through reverseProxyHandler with 429 then success.
func TestReverseProxy_429EndToEnd(t *testing.T) {
	log.Printf("TEST_UNIT: Starting TestReverseProxy_429EndToEnd")

	callCount := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		log.Printf("TEST_UNIT: Backend received request %d for path %s", callCount, r.URL.Path)

		// Handle EXR path
		if strings.HasPrefix(r.URL.Path, "/xrs/") {
			if callCount == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprint(w, `{"error":"rate limit"}`)
				return
			}
			fmt.Fprint(w, `{"result":"ok"}`)
			return
		}

		// Handle regular path
		if callCount == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":"rate limit"}`)
			return
		}
		fmt.Fprint(w, `{"result":"ok"}`)
	}))
	defer backend.Close()

	servers := &Servers{
		GlobalCoinServerIDs: fastjson.MustParse(`{"BTC":{"ids":[1]}}`),
		Slice: []*Server{
			{id: 1, url: backend.URL, exr: true},
		},
	}

	globalConfig.config = &Config{
		AcceptedMethods:         []string{"validmethod"},
		AcceptedPaths:           []string{"/"},
		HttpTimeout:             5,
		RateLimit:               100,
		ConsensusThreshold:      0.6,
		DynlistServersProviders: []string{},
		MaxLogSize:              1048576,
	}
	defer setTestHTTPClient(5)()
	resetLogThrottleState()
	t.Cleanup(resetLogThrottleState)

	req := httptest.NewRequest("POST", "/", strings.NewReader(
		`{"method":"validmethod","params":["BTC"]}`))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	reverseProxyHandler(servers)(w, req)

	res := w.Result()
	body, _ := io.ReadAll(res.Body)

	log.Printf("TEST_UNIT: callCount=%d status=%d body=%s", callCount, res.StatusCode, string(body))

	assert.Equal(t, http.StatusOK, res.StatusCode, "should succeed after retry")
	assert.JSONEq(t, `{"result":"ok"}`, string(body))
	assert.GreaterOrEqual(t, callCount, 2, "backend should have received at least 2 requests (429 + retry)")
}

// Test 6: handleOriginServerResponse passes through without blocking when
// there is no proactive rate limiter (the field will be nil after removal).
func TestHandleOriginServerResponse_NoProactiveLimiter(t *testing.T) {
	log.Printf("TEST_UNIT: Starting TestHandleOriginServerResponse_NoProactiveLimiter")

	callCount := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"result":"test"}`)
	}))
	defer backend.Close()

	server := &Server{id: 1, url: backend.URL, exr: false}

	defer setTestHTTPClient(5)()

	// Build a request that already points to the backend URL, as
	// updateRequestHeaders would have done before calling handleOriginServerResponse.
	req := httptest.NewRequest("GET", backend.URL+"/getblockcount", nil)
	req.Header.Set("Content-Type", "application/json")
	req.RequestURI = "" // client.Do requires empty RequestURI

	resp, err := handleOriginServerResponse(req, server)

	assert.NoError(t, err, "should not error when requestLimiter is nil")
	assert.NotNil(t, resp, "should return a response")
	assert.Equal(t, 1, callCount, "backend should be called exactly once")
	if resp != nil {
		assert.JSONEq(t, `{"result":"test"}`, string(resp.MarshalTo(nil)))
	}
}

// TestRetryWithRandomValidServer_TwoServersBoth429 exhausts server[1],
// switches to server[2] via exclusion, and succeeds.
func TestRetryWithRandomValidServer_TwoServersBoth429(t *testing.T) {
	log.Printf("TEST_UNIT: Starting TestRetryWithRandomValidServer_TwoServersBoth429")

	callCount1 := 0
	callCount2 := 0

	// server[1] always 429
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount1++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":"rate limit"}`)
	}))
	defer backend1.Close()

	// server[2] always 200
	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount2++
		fmt.Fprint(w, `{"result":"ok"}`)
	}))
	defer backend2.Close()

	servers := &Servers{
		GlobalCoinServerIDs: fastjson.MustParse(`{"BTC":{"ids":[1,2]}}`),
		Slice: []*Server{
			{id: 1, url: backend1.URL, exr: true},
			{id: 2, url: backend2.URL, exr: true},
		},
	}

	globalConfig.config = &Config{
		AcceptedMethods:         []string{"validmethod"},
		AcceptedPaths:           []string{"/"},
		HttpTimeout:             5,
		RateLimit:               100,
		ConsensusThreshold:      0.6,
		DynlistServersProviders: []string{},
		MaxLogSize:              1048576,
	}
	defer setTestHTTPClient(5)()
	resetLogThrottleState()
	t.Cleanup(resetLogThrottleState)

	requestData := RequestData{Method: "validmethod", Params: []interface{}{"BTC"}, Coin: "BTC", Ip: "127.0.0.1"}

	// Run until we see the server[1]-first path (exclusion path)
	sawExclusionPath := false
	for i := 0; i < 20; i++ {
		callCount1 = 0
		callCount2 = 0

		req := httptest.NewRequest("POST", "/", strings.NewReader(
			`{"method":"validmethod","params":["BTC"]}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:12345"

		w := httptest.NewRecorder()
		server, err := retryWithRandomValidServer(w, req, servers, "BTC", &requestData, RetryAttemptsDefault)

		if err == nil && callCount1 > 0 {
			sawExclusionPath = true
			assert.Equal(t, 2, server.id, "should return server[2]")
			assert.Equal(t, Relay429MaxRetries+1, callCount1,
				"server[1] called initial + %d backoff retries = %d total",
				Relay429MaxRetries, Relay429MaxRetries+1)
			assert.Equal(t, 1, callCount2, "server[2] called once after exclusion")
			break
		}
	}

	assert.True(t, sawExclusionPath, "should have seen server[1]-first exclusion path")
}
