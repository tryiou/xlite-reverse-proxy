package main

import (
	"fmt"
	"net"
	"net/http"
	"regexp"
)


// Validator manages validation state and errors
type Validator struct {
	errors []ValidationError
}

func (v *Validator) AddError(field, message string, value interface{}) {
	v.errors = append(v.errors, ValidationError{
		Field:  field,
		Reason: message,
		Value:  value,
	})
}

func (v *Validator) HasErrors() bool {
	return len(v.errors) > 0
}

func (v *Validator) Errors() []ValidationError {
	return v.errors
}

func (v *Validator) Validate() error {
	if v.HasErrors() {
		return &v.errors[0] // Return pointer to first error
	}
	return nil
}

// Validation functions
func (v *Validator) ValidateIP(ip string) {
	if ip == "" {
		v.AddError("IP", "cannot be empty", ip)
		return
	}

	if net.ParseIP(ip) == nil {
		v.AddError("IP", "invalid IP address format", ip)
	}
}

func (v *Validator) ValidateCoin(coin string) {
	if coin == "" {
		v.AddError("coin", "cannot be empty", coin)
		return
	}

	// Allow alphanumeric coins with common separators like underscore
	validCoin := regexp.MustCompile(`^[A-Za-z0-9_]+$`)
	if !validCoin.MatchString(coin) {
		v.AddError("coin", "contains invalid characters", coin)
	}

	if len(coin) > 10 { // Reasonable length limit
		v.AddError("coin", "too long (max 10 characters)", coin)
	}
}

func (v *Validator) ValidateMethod(method string) {
	if method == "" {
		v.AddError("method", "cannot be empty", method)
		return
	}

	cfg := globalConfig.GetConfig()
	for _, validMethod := range cfg.AcceptedMethods {
		if method == validMethod {
			return
		}
	}

	v.AddError("method", "not in accepted methods list", method)
}

func (v *Validator) ValidateParams(params []interface{}) {
	if len(params) > 10 { // Prevent abuse
		v.AddError("params", "too many parameters (max 10)", len(params))
	}
}

func (v *Validator) ValidatePath(path string) {
	if path == "" {
		v.AddError("path", "cannot be empty", path)
		return
	}

	cfg := globalConfig.GetConfig()
	for _, validPath := range cfg.AcceptedPaths {
		if path == validPath {
			return
		}
	}

	v.AddError("path", "not in accepted paths list", path)
}

func (v *Validator) ValidateRequestData(data RequestData) {
	v.ValidateIP(data.Ip)
	v.ValidateMethod(data.Method)
	v.ValidateParams(data.Params)

	// Validate specific methods
	if data.Method == "getblockhash" || data.Method == "getblock" {
		if len(data.Params) < 1 {
			v.AddError("params", "missing coin parameter", data.Params)
		} else if coin, ok := data.Params[0].(string); ok {
			v.ValidateCoin(coin)
		}
	}
}

// Configuration validation
func (v *Validator) ValidateConfig(cfg *Config) {
	// Validate timeouts
	if cfg.HttpTimeout <= 0 || cfg.HttpTimeout > 300 {
		v.AddError("HttpTimeout", "must be between 1 and 300 seconds", cfg.HttpTimeout)
	}

	// Validate rate limit
	if cfg.RateLimit <= 0 || cfg.RateLimit > 1000 {
		v.AddError("RateLimit", "must be between 1 and 1000", cfg.RateLimit)
	}

	// Validate consensus threshold
	if cfg.ConsensusThreshold <= 0 || cfg.ConsensusThreshold > 1 {
		v.AddError("ConsensusThreshold", "must be between 0 and 1", cfg.ConsensusThreshold)
	}

	// Validate max stored blocks
	if cfg.MaxStoredBlocks <= 0 || cfg.MaxStoredBlocks > 100 {
		v.AddError("MaxStoredBlocks", "must be between 1 and 100", cfg.MaxStoredBlocks)
	}

	// Validate max block time diff
	if cfg.MaxBlockTimeDiff <= 0 || cfg.MaxBlockTimeDiff > 86400 {
		v.AddError("MaxBlockTimeDiff", "must be between 1 and 86400 seconds", cfg.MaxBlockTimeDiff)
	}

	// Validate log size - must be positive and reasonable limits
	if cfg.MaxLogSize <= 0 {
		v.AddError("MaxLogSize", "must be positive", cfg.MaxLogSize)
	}
	if cfg.MaxLogSize > 100*1024*1024 { // 100MB maximum
		v.AddError("MaxLogSize", "must be less than 100MB", cfg.MaxLogSize)
	}

	// Validate accepted paths and methods are not empty
	if len(cfg.AcceptedPaths) == 0 {
		v.AddError("AcceptedPaths", "cannot be empty", cfg.AcceptedPaths)
	}

	if len(cfg.AcceptedMethods) == 0 {
		v.AddError("AcceptedMethods", "cannot be empty", cfg.AcceptedMethods)
	}

	// Validate server configurations
	if len(cfg.ServersMap) == 0 && len(cfg.DynlistServersProviders) == 0 {
		v.AddError("Servers", "must have at least one server or provider configured", nil)
	}

	// Validate individual servers
	for i, server := range cfg.ServersMap {
		if err := validateURLFormat(server.URL); err != nil {
			v.AddError(fmt.Sprintf("Server[%d].URL", i), err.Error(), server.URL)
		}
	}

	// Validate dynamic server providers
	for i, provider := range cfg.DynlistServersProviders {
		if err := validateURLFormat(provider); err != nil {
			v.AddError(fmt.Sprintf("DynlistProvider[%d]", i), err.Error(), provider)
		}
	}
}

// Request-specific validation
func (v *Validator) ValidateHTTPRequest(r *http.Request) {
	// Validate HTTP method
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		v.AddError("HTTPMethod", "only GET and POST methods are supported", r.Method)
	}

	// Validate content length for POST requests
	if r.Method == http.MethodPost && r.ContentLength > 1024*1024 { // 1MB limit
		v.AddError("ContentLength", "request too large (max 1MB)", r.ContentLength)
	}

	// Validate required headers
	if r.Method == http.MethodPost && r.Header.Get("Content-Type") == "" {
		v.AddError("Content-Type", "required for POST requests", "")
	}
}
