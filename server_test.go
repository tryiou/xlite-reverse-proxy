package main

import (
	"io"
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

func (m *mockError) Error() string {
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
	expected := "server[2] ping JSON parse failed"
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
	expected := "server[5] ping response missing 'result' field"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("Expected error message to contain '%s', got: %v", expected, err)
	}
}
