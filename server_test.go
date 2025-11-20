package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"strings"
	"testing"
)

// mockTransport is a mock HTTP transport for testing
type mockTransport struct {
	shouldFail bool
	response   string
	statusCode int
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.shouldFail {
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
	server := &Server{
		id:  1,
		url: "http://test-server-1.example.com",
	}

	// Mock httpClient to return error
	originalClient := httpClient
	defer func() { httpClient = originalClient }()

	httpClient = &http.Client{
		Transport: &mockTransport{shouldFail: true},
	}

	err := server.server_GetPing()
	if err == nil {
		t.Error("Expected error but got none")
	}

	// Verify error message contains server ID correctly (no duplicates)
	expected := "server[1] ping request failed"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("Expected error message to contain '%s', got: %v", expected, err)
	}

	// Verify the error is wrapped properly
	if !strings.Contains(err.Error(), "mock error for testing") {
		t.Errorf("Expected error to contain mock error, got: %v", err)
	}
}

// TestServerPingJSONParseError tests JSON parse error logging
func TestServerPingJSONParseError(t *testing.T) {
	server := &Server{
		id:  2,
		url: "http://test-server-2.example.com",
	}

	// Mock httpClient to return invalid JSON
	originalClient := httpClient
	defer func() { httpClient = originalClient }()

	httpClient = &http.Client{
		Transport: &mockTransport{
			shouldFail: false,
			response:   "invalid json",
			statusCode: 200,
		},
	}

	err := server.server_GetPing()
	if err == nil {
		t.Error("Expected error but got none")
	}

	// Verify error message contains server ID correctly
	expected := "server[2] JSON parse failed"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("Expected error message to contain '%s', got: %v", expected, err)
	}
}

// TestServerPingSuccessValue tests successful ping with correct value
func TestServerPingSuccessValue(t *testing.T) {
	server := &Server{
		id:  3,
		url: "http://test-server-3.example.com",
	}

	// Mock httpClient to return successful ping
	originalClient := httpClient
	defer func() { httpClient = originalClient }()

	httpClient = &http.Client{
		Transport: &mockTransport{
			shouldFail: false,
			response:   `{"result": 1}`,
			statusCode: 200,
		},
	}

	err := server.server_GetPing()
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Verify ping value is set correctly
	if server.ping != PingSuccessValue {
		t.Errorf("Expected ping to be %d, got %d", PingSuccessValue, server.ping)
	}
}

// TestServerPingFailureValue tests ping with failure value
func TestServerPingFailureValue(t *testing.T) {
	server := &Server{
		id:  4,
		url: "http://test-server-4.example.com",
	}

	// Mock httpClient to return failure ping
	originalClient := httpClient
	defer func() { httpClient = originalClient }()

	httpClient = &http.Client{
		Transport: &mockTransport{
			shouldFail: false,
			response:   `{"result": 0}`,
			statusCode: 200,
		},
	}

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
	server := &Server{
		id:  5,
		url: "http://test-server-5.example.com",
	}

	// Mock httpClient to return response without result field
	originalClient := httpClient
	defer func() { httpClient = originalClient }()

	httpClient = &http.Client{
		Transport: &mockTransport{
			shouldFail: false,
			response:   `{"error": "no result"}`,
			statusCode: 200,
		},
	}

	err := server.server_GetPing()
	if err == nil {
		t.Error("Expected error but got none")
	}

	// Verify error message contains server ID correctly
	expected := "server[5] response missing 'result' field"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("Expected error message to contain '%s', got: %v", expected, err)
	}
}

// TestServerGetFeesErrorLogging tests fee fetching error logging
func TestServerGetFeesErrorLogging(t *testing.T) {
	server := &Server{
		id:  6,
		url: "http://test-server-6.example.com",
	}

	// Mock httpClient to return error
	originalClient := httpClient
	defer func() { httpClient = originalClient }()

	httpClient = &http.Client{
		Transport: &mockTransport{shouldFail: true},
	}

	err := server.server_GetFees()
	if err == nil {
		t.Error("Expected error but got none")
	}

	// Verify error message contains server ID correctly
	expected := "server[6] getfees failed"
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
	server := &Server{
		id:  7,
		url: "http://test-server-7.example.com",
	}

	// Mock httpClient to return error
	originalClient := httpClient
	defer func() { httpClient = originalClient }()

	httpClient = &http.Client{
		Transport: &mockTransport{shouldFail: true},
	}

	err := server.server_GetHeights()
	if err == nil {
		t.Error("Expected error but got none")
	}

	// Verify error message contains server ID correctly
	expected := "server[7] getheights failed"
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
	server := &Server{
		id:  8,
		url: "http://test-server-8.example.com",
	}

	// Mock httpClient to return error
	originalClient := httpClient
	defer func() { httpClient = originalClient }()

	httpClient = &http.Client{
		Transport: &mockTransport{shouldFail: true},
	}

	hash, err := server.server_GetBlockHash("BTC", 800000)
	if err == nil {
		t.Error("Expected error but got none")
	}

	// Verify error message contains server ID correctly
	expected := "server[8] getblockhash failed"
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
	server := &Server{
		id:  9,
		url: "http://test-server-9.example.com",
	}

	// Mock httpClient to return error
	originalClient := httpClient
	defer func() { httpClient = originalClient }()

	httpClient = &http.Client{
		Transport: &mockTransport{shouldFail: true},
	}

	result, err := server.server_GetBlock("BTC", "hash123")
	if err == nil {
		t.Error("Expected error but got none")
	}

	// Verify error message contains server ID correctly
	expected := "server[9] getblock failed"
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
	// Test logServerError
	var logOutput bytes.Buffer
	originalLogger := logger
	defer func() { logger = originalLogger }()
	logger = log.New(&logOutput, "", 0)

	logServerError(123, "test operation", mockError{})
	output := logOutput.String()
	if !strings.Contains(output, "[server123]_error test operation failed") {
		t.Errorf("Expected log output to contain server error, got: %s", output)
	}
	if !strings.Contains(output, "mock error for testing") {
		t.Errorf("Expected log output to contain error message, got: %s", output)
	}

	// Test logServerSuccess
	logOutput.Reset()
	logServerSuccess(456, "test operation")
	output = logOutput.String()
	if !strings.Contains(output, "[server456] test operation successful") {
		t.Errorf("Expected log output to contain server success, got: %s", output)
	}

	// Test logServerSuccess with details
	logOutput.Reset()
	logServerSuccess(789, "test operation", "with details")
	output = logOutput.String()
	if !strings.Contains(output, "[server789] test operation successful: with details") {
		t.Errorf("Expected log output to contain server success with details, got: %s", output)
	}
}
