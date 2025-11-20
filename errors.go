package main

import "fmt"

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
