package main

import (
	"errors"
	"testing"
	"time"
)

func TestServerError(t *testing.T) {
	originalErr := errors.New("original error")
	serverErr := NewServerError(123, "test operation", originalErr)

	// Test error message
	expected := "server[123] test operation: original error"
	if serverErr.Error() != expected {
		t.Errorf("Expected error message %q, got %q", expected, serverErr.Error())
	}

	// Test unwrapping
	if !errors.Is(serverErr, originalErr) {
		t.Errorf("Expected error to wrap original error")
	}

	// Test type assertion
	if serr, ok := serverErr.(*ServerError); ok {
		if serr.ServerID != 123 {
			t.Errorf("Expected ServerID 123, got %d", serr.ServerID)
		}
		if serr.Operation != "test operation" {
			t.Errorf("Expected Operation 'test operation', got %q", serr.Operation)
		}
		if serr.Err != originalErr {
			t.Errorf("Expected Err to be original error")
		}
	} else {
		t.Errorf("Expected ServerError type")
	}
}

func TestValidationError(t *testing.T) {
	validationErr := NewValidationError("field", "test reason", "test value")

	// Test error message
	expected := "validation error in field 'field': test reason (value: test value)"
	if validationErr.Error() != expected {
		t.Errorf("Expected error message %q, got %q", expected, validationErr.Error())
	}

	// Test type assertion
	if verr, ok := validationErr.(*ValidationError); ok {
		if verr.Field != "field" {
			t.Errorf("Expected Field 'field', got %q", verr.Field)
		}
		if verr.Reason != "test reason" {
			t.Errorf("Expected Reason 'test reason', got %q", verr.Reason)
		}
		if verr.Value != "test value" {
			t.Errorf("Expected Value 'test value'")
		}
	} else {
		t.Errorf("Expected ValidationError type")
	}
}

func TestHTTPError(t *testing.T) {
	originalErr := errors.New("http error")
	httpErr := NewHTTPError(500, "test operation", originalErr)

	// Test error message
	expected := "HTTP 500 - test operation: http error"
	if httpErr.Error() != expected {
		t.Errorf("Expected error message %q, got %q", expected, httpErr.Error())
	}

	// Test unwrapping
	if !errors.Is(httpErr, originalErr) {
		t.Errorf("Expected error to wrap original error")
	}

	// Test type assertion
	if herr, ok := httpErr.(*HTTPError); ok {
		if herr.StatusCode != 500 {
			t.Errorf("Expected StatusCode 500, got %d", herr.StatusCode)
		}
		if herr.Operation != "test operation" {
			t.Errorf("Expected Operation 'test operation', got %q", herr.Operation)
		}
		if herr.Err != originalErr {
			t.Errorf("Expected Err to be original error")
		}
	} else {
		t.Errorf("Expected HTTPError type")
	}
}

func TestErrorCategorization(t *testing.T) {
	serverErr := NewServerError(123, "test", nil)
	validationErr := NewValidationError("field", "reason", "value")
	httpErr := NewHTTPError(500, "test", nil)

	// Test categorization functions
	if !IsServerError(serverErr) {
		t.Error("Expected IsServerError to return true for ServerError")
	}
	if IsServerError(validationErr) {
		t.Error("Expected IsServerError to return false for ValidationError")
	}
	if IsServerError(httpErr) {
		t.Error("Expected IsServerError to return false for HTTPError")
	}

	if IsValidationError(serverErr) {
		t.Error("Expected IsValidationError to return false for ServerError")
	}
	if !IsValidationError(validationErr) {
		t.Error("Expected IsValidationError to return true for ValidationError")
	}
	if IsValidationError(httpErr) {
		t.Error("Expected IsValidationError to return false for HTTPError")
	}

	if IsHTTPError(serverErr) {
		t.Error("Expected IsHTTPError to return false for ServerError")
	}
	if IsHTTPError(validationErr) {
		t.Error("Expected IsHTTPError to return false for ValidationError")
	}
	if !IsHTTPError(httpErr) {
		t.Error("Expected IsHTTPError to return true for HTTPError")
	}
}

func TestRateLimitError(t *testing.T) {
	originalErr := errors.New("429 Too Many Requests")
	retryAfter := 5 * time.Second
	rlErr := NewRateLimitError(7, retryAfter, originalErr)

	// Test error message
	expected := "server[7] rate limited (429): retry after 5s: 429 Too Many Requests"
	if rlErr.Error() != expected {
		t.Errorf("Expected error message %q, got %q", expected, rlErr.Error())
	}

	// Test unwrapping
	if !errors.Is(rlErr, originalErr) {
		t.Errorf("Expected error to wrap original error")
	}

	// Test type assertion via errors.As
	var target *RateLimitError
	if !errors.As(rlErr, &target) {
		t.Fatalf("Expected errors.As to succeed for RateLimitError")
	}
	if target.ServerID != 7 {
		t.Errorf("Expected ServerID 7, got %d", target.ServerID)
	}
	if target.RetryAfter != retryAfter {
		t.Errorf("Expected RetryAfter %v, got %v", retryAfter, target.RetryAfter)
	}
	if target.Err != originalErr {
		t.Errorf("Expected Err to be original error")
	}

	// Test IsRateLimitError helper
	if !IsRateLimitError(rlErr) {
		t.Error("Expected IsRateLimitError to return true for RateLimitError")
	}
	if IsRateLimitError(errors.New("not a rate limit error")) {
		t.Error("Expected IsRateLimitError to return false for plain error")
	}
	if IsRateLimitError(NewServerError(1, "test", nil)) {
		t.Error("Expected IsRateLimitError to return false for ServerError")
	}
}
