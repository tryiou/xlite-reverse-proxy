package main

import (
	"fmt"
	"time"
)

// Error types for different domains
type ServerError struct {
	ServerID  int
	Operation string
	Err       error
}

func (e *ServerError) Error() string {
	return fmt.Sprintf("server[%d] %s: %v", e.ServerID, e.Operation, e.Err)
}

func (e *ServerError) Unwrap() error {
	return e.Err
}

type ValidationError struct {
	Field  string
	Reason string
	Value  interface{}
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error in field '%s': %s (value: %v)", e.Field, e.Reason, e.Value)
}

type HTTPError struct {
	StatusCode int
	Operation  string
	Err        error
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d - %s: %v", e.StatusCode, e.Operation, e.Err)
}

func (e *HTTPError) Unwrap() error {
	return e.Err
}

// RateLimitError is returned when a backend server responds with HTTP 429.
// It carries the Retry-After duration from the response header so the caller
// can apply an appropriate backoff before retrying.
type RateLimitError struct {
	ServerID   int
	RetryAfter time.Duration
	Err        error
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("server[%d] rate limited (429): retry after %v: %v", e.ServerID, e.RetryAfter, e.Err)
}

func (e *RateLimitError) Unwrap() error {
	return e.Err
}

// Helper functions for creating errors
func NewServerError(serverID int, operation string, err error) error {
	return &ServerError{
		ServerID:  serverID,
		Operation: operation,
		Err:       err,
	}
}

func NewValidationError(field, reason string, value interface{}) error {
	return &ValidationError{
		Field:  field,
		Reason: reason,
		Value:  value,
	}
}

func NewHTTPError(statusCode int, operation string, err error) error {
	return &HTTPError{
		StatusCode: statusCode,
		Operation:  operation,
		Err:        err,
	}
}

func NewRateLimitError(serverID int, retryAfter time.Duration, err error) error {
	return &RateLimitError{
		ServerID:   serverID,
		RetryAfter: retryAfter,
		Err:        err,
	}
}

// Error categorization helpers
func IsServerError(err error) bool {
	_, ok := err.(*ServerError)
	return ok
}

func IsValidationError(err error) bool {
	_, ok := err.(*ValidationError)
	return ok
}

func IsHTTPError(err error) bool {
	_, ok := err.(*HTTPError)
	return ok
}

func IsRateLimitError(err error) bool {
	_, ok := err.(*RateLimitError)
	return ok
}
