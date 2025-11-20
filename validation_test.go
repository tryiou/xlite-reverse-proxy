package main

import (
	"testing"
)

func TestValidator_ValidateIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		wantErr bool
	}{
		{"valid IPv4", "192.168.1.1", false},
		{"valid IPv6", "2001:db8::1", false},
		{"empty IP", "", true},
		{"invalid IP", "not-an-ip", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &Validator{}
			validator.ValidateIP(tt.ip)

			if (validator.HasErrors()) != tt.wantErr {
				t.Errorf("Validator.ValidateIP() error = %v, wantErr %v", validator.HasErrors(), tt.wantErr)
			}
		})
	}
}

func TestValidator_ValidateCoin(t *testing.T) {
	tests := []struct {
		name    string
		coin    string
		wantErr bool
	}{
		{"valid coin BTC", "BTC", false},
		{"valid coin LTC", "LTC", false},
		{"valid coin with underscore", "COIN_CHAIN", false},
		{"empty coin", "", true},
		{"coin with invalid chars", "BTC$", true},
		{"coin too long", "VERYLONGCOINNAME", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &Validator{}
			validator.ValidateCoin(tt.coin)

			if (validator.HasErrors()) != tt.wantErr {
				t.Errorf("Validator.ValidateCoin() error = %v, wantErr %v", validator.HasErrors(), tt.wantErr)
			}
		})
	}
}

func TestValidator_ValidateMethod(t *testing.T) {
	// Set up a test config
	globalConfig.config = &Config{
		AcceptedMethods: []string{"heights", "fees", "ping", "getblock", "getblockhash"},
	}
	defer func() { globalConfig.config = nil }()

	tests := []struct {
		name    string
		method  string
		wantErr bool
	}{
		{"valid method", "heights", false},
		{"valid method", "fees", false},
		{"valid method", "ping", false},
		{"valid method", "getblock", false},
		{"valid method", "getblockhash", false},
		{"empty method", "", true},
		{"invalid method", "invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &Validator{}
			validator.ValidateMethod(tt.method)

			if (validator.HasErrors()) != tt.wantErr {
				t.Errorf("Validator.ValidateMethod() error = %v, wantErr %v", validator.HasErrors(), tt.wantErr)
			}
		})
	}
}

func TestValidator_ValidateParams(t *testing.T) {
	tests := []struct {
		name    string
		params  []interface{}
		wantErr bool
	}{
		{"valid params", []interface{}{"BTC", 800000}, false},
		{"too many params", make([]interface{}, 11), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &Validator{}
			validator.ValidateParams(tt.params)

			if (validator.HasErrors()) != tt.wantErr {
				t.Errorf("Validator.ValidateParams() error = %v, wantErr %v", validator.HasErrors(), tt.wantErr)
			}
		})
	}
}

func TestValidator_ValidateRequestData(t *testing.T) {
	// Set up a test config
	globalConfig.config = &Config{
		AcceptedMethods: []string{"heights", "fees", "ping", "getblock", "getblockhash"},
	}
	defer func() { globalConfig.config = nil }()

	tests := []struct {
		name    string
		data    RequestData
		wantErr bool
	}{
		{
			name: "valid request data",
			data: RequestData{
				Method: "heights",
				Params: []interface{}{},
				Ip:     "192.168.1.1",
			},
			wantErr: false,
		},
		{
			name: "invalid IP",
			data: RequestData{
				Method: "heights",
				Params: []interface{}{},
				Ip:     "invalid-ip",
			},
			wantErr: true,
		},
		{
			name: "invalid method",
			data: RequestData{
				Method: "invalid",
				Params: []interface{}{},
				Ip:     "192.168.1.1",
			},
			wantErr: true,
		},
		{
			name: "invalid coin in getblockhash",
			data: RequestData{
				Method: "getblockhash",
				Params: []interface{}{"BTC$"},
				Ip:     "192.168.1.1",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &Validator{}
			validator.ValidateRequestData(tt.data)

			if (validator.HasErrors()) != tt.wantErr {
				t.Errorf("Validator.ValidateRequestData() error = %v, wantErr %v", validator.HasErrors(), tt.wantErr)
			}
		})
	}
}
