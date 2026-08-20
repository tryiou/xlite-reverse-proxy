package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"
)

// setupGlobalConfigForTest sets up the global config for testing
func setupGlobalConfigForTest() {
	if globalConfig.config == nil {
		globalConfig.config = &Config{
			HttpTimeout: 5,
		}
		globalConfig.client = &http.Client{Timeout: 5 * time.Second}
	}
}

// mockTransport is a mock HTTP transport for testing
type mockTransport struct {
	shouldFail bool
	response   string
	statusCode int
	serverID   int
	failCount  int
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.shouldFail {
		return nil, &mockError{}
	}

	// Fail the first failCount calls, then behave normally.
	if m.failCount > 0 {
		m.failCount--
		return nil, &mockError{}
	}

	// Create response body
	body := strings.NewReader(m.response)

	response := &http.Response{
		StatusCode: m.statusCode,
		Body:       http.NoBody,
		Header:     make(http.Header),
	}

	if m.response != "" {
		response.Body = io.NopCloser(body)
	}

	return response, nil
}

// mockError is a mock error for testing
type mockError struct{}

func (m mockError) Error() string {
	return "mock error for testing"
}

// TestServerPingErrorLogging tests that the ping error logging doesn't have duplicate parameters
func TestServerPingErrorLogging(t *testing.T) {
	// Set up global config first
	if globalConfig.config == nil {
		globalConfig.config = &Config{
			HttpTimeout: 5,
		}
		globalConfig.client = &http.Client{Timeout: 5 * time.Second}
	}

	server := &Server{
		id:  1,
		url: "http://test-server-1.example.com",
	}

	// Create a mock HTTP client with our mock transport
	mockClient := &http.Client{
		Transport: &mockTransport{shouldFail: true, serverID: 1},
	}

	// Mock the global config to return our mock client
	originalClient := globalConfig.GetClient()
	globalConfig.client = mockClient
	defer func() { globalConfig.client = originalClient }()

	err := server.server_GetPing()
	if err == nil {
		t.Error("Expected error but got none")
	}

	// Check for server ID in error message
	if !strings.Contains(err.Error(), "server[1]") {
		t.Errorf("Expected error message to contain 'server[1]', got: %v", err)
	}
}

// TestServerPingJSONParseError tests JSON parse error logging
func TestServerPingJSONParseError(t *testing.T) {
	// Set up global config first
	if globalConfig.config == nil {
		globalConfig.config = &Config{
			HttpTimeout: 5,
		}
		globalConfig.client = &http.Client{Timeout: 5 * time.Second}
	}

	server := &Server{
		id:  2,
		url: "http://test-server-2.example.com",
	}

	// Create a mock HTTP client with our mock transport
	mockClient := &http.Client{
		Transport: &mockTransport{
			shouldFail: false,
			response:   "invalid json",
			statusCode: 200,
		},
	}

	// Mock the global config to return our mock client
	originalClient := globalConfig.GetClient()
	globalConfig.client = mockClient
	defer func() { globalConfig.client = originalClient }()

	err := server.server_GetPing()
	if err == nil {
		t.Error("Expected error but got none")
	}

	// Check for server ID in error message
	if !strings.Contains(err.Error(), "server[2]") {
		t.Errorf("Expected error message to contain 'server[2]', got: %v", err)
	}
}

// TestServerPingSuccessValue tests successful ping with correct value
func TestServerPingSuccessValue(t *testing.T) {
	// Set up global config first
	if globalConfig.config == nil {
		globalConfig.config = &Config{
			HttpTimeout: 5,
		}
		globalConfig.client = &http.Client{Timeout: 5 * time.Second}
	}

	server := &Server{
		id:  3,
		url: "http://test-server-3.example.com",
	}

	// Create a mock HTTP client with our mock transport
	mockClient := &http.Client{
		Transport: &mockTransport{
			shouldFail: false,
			response:   `{"result": 1}`,
			statusCode: 200,
		},
	}

	// Mock the global config to return our mock client
	originalClient := globalConfig.GetClient()
	globalConfig.client = mockClient
	defer func() { globalConfig.client = originalClient }()

	err := server.server_GetPing()
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Verify ping value is set correctly
	if server.ping != PingSuccessValue {
		t.Errorf("Expected ping to be %d, got %d", PingSuccessValue, server.ping)
	}
}

// TestServerPingWithRetriesSuccess verifies that pingWithRetries succeeds when a
// later attempt succeeds after an initial failure.
func TestServerPingWithRetriesSuccess(t *testing.T) {
	if globalConfig.config == nil {
		globalConfig.config = &Config{
			HttpTimeout: 5,
		}
		globalConfig.client = &http.Client{Timeout: 5 * time.Second}
	}

	server := &Server{
		id:  1,
		url: "http://test-server-retry.example.com",
	}

	mockClient := &http.Client{
		Transport: &mockTransport{
			response:   `{"result": 1}`,
			statusCode: 200,
			failCount:  1, // fail the first ping, succeed on retry
		},
	}

	originalClient := globalConfig.GetClient()
	globalConfig.client = mockClient
	defer func() { globalConfig.client = originalClient }()

	err := server.pingWithRetries()
	if err != nil {
		t.Errorf("Expected no error after retry, got: %v", err)
	}
	if server.ping != PingSuccessValue {
		t.Errorf("Expected ping to be %d after retry, got %d", PingSuccessValue, server.ping)
	}
}

// TestServerPingWithRetriesAllFail verifies that pingWithRetries returns an error
// when all ping attempts fail (the server should then be evicted).
func TestServerPingWithRetriesAllFail(t *testing.T) {
	if globalConfig.config == nil {
		globalConfig.config = &Config{
			HttpTimeout: 5,
		}
		globalConfig.client = &http.Client{Timeout: 5 * time.Second}
	}

	server := &Server{
		id:  1,
		url: "http://test-server-retry-fail.example.com",
	}

	mockClient := &http.Client{
		Transport: &mockTransport{shouldFail: true},
	}

	originalClient := globalConfig.GetClient()
	globalConfig.client = mockClient
	defer func() { globalConfig.client = originalClient }()

	err := server.pingWithRetries()
	if err == nil {
		t.Errorf("Expected an error after all ping attempts failed, got nil")
	}
	if server.ping != PingFailureValue {
		t.Errorf("Expected ping to be %d after all failures, got %d", PingFailureValue, server.ping)
	}
}

// TestServerPingFailureValue tests ping with failure value
func TestServerPingFailureValue(t *testing.T) {
	// Set up global config first
	if globalConfig.config == nil {
		globalConfig.config = &Config{
			HttpTimeout: 5,
		}
		globalConfig.client = &http.Client{Timeout: 5 * time.Second}
	}

	server := &Server{
		id:  4,
		url: "http://test-server-4.example.com",
	}

	// Create a mock HTTP client with our mock transport
	mockClient := &http.Client{
		Transport: &mockTransport{
			shouldFail: false,
			response:   `{"result": 0}`,
			statusCode: 200,
		},
	}

	// Mock the global config to return our mock client
	originalClient := globalConfig.GetClient()
	globalConfig.client = mockClient
	defer func() { globalConfig.client = originalClient }()

	err := server.server_GetPing()
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Verify ping value is set correctly
	if server.ping != PingFailureValue {
		t.Errorf("Expected ping to be %d, got %d", PingFailureValue, server.ping)
	}
}

// TestServerPingMissingResultField tests missing result field error
func TestServerPingMissingResultField(t *testing.T) {
	// Set up global config first
	if globalConfig.config == nil {
		globalConfig.config = &Config{
			HttpTimeout: 5,
		}
		globalConfig.client = &http.Client{Timeout: 5 * time.Second}
	}

	server := &Server{
		id:  5,
		url: "http://test-server-5.example.com",
	}

	// Create a mock HTTP client with our mock transport
	mockClient := &http.Client{
		Transport: &mockTransport{
			shouldFail: false,
			response:   `{"error": "no result"}`,
			statusCode: 200,
		},
	}

	// Mock the global config to return our mock client
	originalClient := globalConfig.GetClient()
	globalConfig.client = mockClient
	defer func() { globalConfig.client = originalClient }()

	err := server.server_GetPing()
	if err == nil {
		t.Error("Expected error but got none")
	}

	// Check for server ID in error message
	if !strings.Contains(err.Error(), "server[5]") {
		t.Errorf("Expected error message to contain 'server[5]', got: %v", err)
	}
}

// TestServerGetFeesErrorLogging tests fee fetching error logging
func TestServerGetFeesErrorLogging(t *testing.T) {
	// Set up global config first
	if globalConfig.config == nil {
		globalConfig.config = &Config{
			HttpTimeout: 5,
		}
		globalConfig.client = &http.Client{Timeout: 5 * time.Second}
	}

	server := &Server{
		id:  6,
		url: "http://test-server-6.example.com",
	}

	// Create a mock HTTP client with our mock transport
	mockClient := &http.Client{
		Transport: &mockTransport{shouldFail: true},
	}

	// Mock the global config to return our mock client
	originalClient := globalConfig.GetClient()
	globalConfig.client = mockClient
	defer func() { globalConfig.client = originalClient }()

	err := server.server_GetFees()
	if err == nil {
		t.Error("Expected error but got none")
	}

	// Verify error message contains server ID correctly
	expected := "server[6] getfees:"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("Expected error message to contain '%s', got: %v", expected, err)
	}

	// Verify default response was set
	if server.getfees == nil {
		t.Error("Expected default JSON response to be set")
	}
}

// TestServerGetHeightsErrorLogging tests heights fetching error logging
func TestServerGetHeightsErrorLogging(t *testing.T) {
	setupGlobalConfigForTest()

	server := &Server{
		id:  7,
		url: "http://test-server-7.example.com",
	}

	// Create a mock HTTP client with our mock transport
	mockClient := &http.Client{
		Transport: &mockTransport{shouldFail: true},
	}

	// Mock the global config to return our mock client
	originalClient := globalConfig.GetClient()
	globalConfig.client = mockClient
	defer func() { globalConfig.client = originalClient }()

	err := server.server_GetHeights()
	if err == nil {
		t.Error("Expected error but got none")
	}

	// Verify error message contains server ID correctly
	expected := "server[7] getheights:"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("Expected error message to contain '%s', got: %v", expected, err)
	}

	// Verify default response was set
	if server.getheights == nil {
		t.Error("Expected default JSON response to be set")
	}
}

// TestServerGetBlockHashErrorLogging tests getblockhash error logging
func TestServerGetBlockHashErrorLogging(t *testing.T) {
	setupGlobalConfigForTest()

	server := &Server{
		id:  8,
		url: "http://test-server-8.example.com",
	}

	// Create a mock HTTP client with our mock transport
	mockClient := &http.Client{
		Transport: &mockTransport{shouldFail: true},
	}

	// Mock the global config to return our mock client
	originalClient := globalConfig.GetClient()
	globalConfig.client = mockClient
	defer func() { globalConfig.client = originalClient }()

	hash, err := server.server_GetBlockHash("BTC", 800000)
	if err == nil {
		t.Error("Expected error but got none")
	}

	// Verify error message contains server ID correctly
	expected := "server[8] getblockhash:"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("Expected error message to contain '%s', got: %v", expected, err)
	}

	// Verify empty hash returned
	if hash != "" {
		t.Errorf("Expected empty hash, got: %s", hash)
	}
}

// TestServerGetBlockErrorLogging tests getblock error logging
func TestServerGetBlockErrorLogging(t *testing.T) {
	setupGlobalConfigForTest()

	server := &Server{
		id:  9,
		url: "http://test-server-9.example.com",
	}

	// Create a mock HTTP client with our mock transport
	mockClient := &http.Client{
		Transport: &mockTransport{shouldFail: true},
	}

	// Mock the global config to return our mock client
	originalClient := globalConfig.GetClient()
	globalConfig.client = mockClient
	defer func() { globalConfig.client = originalClient }()

	result, err := server.server_GetBlock("BTC", "hash123")
	if err == nil {
		t.Error("Expected error but got none")
	}

	// Verify error message contains server ID correctly
	expected := "server[9] getblock:"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("Expected error message to contain '%s', got: %v", expected, err)
	}

	// Verify nil result returned
	if result != nil {
		t.Error("Expected nil result")
	}
}

// TestServerInvalidHeightForGetBlockHash tests invalid height handling
func TestServerInvalidHeightForGetBlockHash(t *testing.T) {
	setupGlobalConfigForTest()

	server := &Server{
		id:  10,
		url: "http://test-server-10.example.com",
	}

	// Test with invalid height
	hash, err := server.server_GetBlockHash("BTC", InvalidHeightValue)
	if err != nil {
		t.Errorf("Expected no error for invalid height, got: %v", err)
	}

	// Verify empty hash returned
	if hash != "" {
		t.Errorf("Expected empty hash for invalid height, got: %s", hash)
	}
}

// TestServerLoggingUtilities tests the logging utility functions
func TestServerLoggingUtilities(t *testing.T) {
	setupGlobalConfigForTest()

	// Test logServerError
	var logOutput bytes.Buffer
	originalLogger := logger
	defer func() { logger = originalLogger }()
	logger = log.New(&logOutput, "", 0)

	logServerError(123, "test operation", mockError{})
	output := logOutput.String()
	if !strings.Contains(output, "[server123]_error") || !strings.Contains(output, "test operation failed") {
		t.Errorf("Expected log output to contain server error, got: %s", output)
	}
	if !strings.Contains(output, "mock error for testing") {
		t.Errorf("Expected log output to contain error message, got: %s", output)
	}
}
