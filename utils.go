package main

import (
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/valyala/fastjson"
)

// initHTTPClient initializes the HTTP client with configuration values
func initHTTPClient() {
	httpClient = &http.Client{
		Timeout: time.Duration(config.HttpTimeout) * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			IdleConnTimeout:     time.Duration(config.HttpTimeout) * time.Second,
			DisableCompression:  false,
			MaxIdleConnsPerHost: 10,
			MaxConnsPerHost:     100,
		},
	}
}

func getDefaultJSONResponse() *fastjson.Value {
	return fastjson.MustParse(`{"result": null, "error": null}`)
}

func getEmptyJSONResponse() *fastjson.Value {
	return fastjson.MustParse(`{}`)
}

func parseJSON(data []byte) (*fastjson.Value, error) {
	var p fastjson.Parser
	value, err := p.ParseBytes(data)
	if err != nil {
		errorMsg := string(data)
		if strings.Contains(errorMsg, "Internal Server Error") {
			// Handle the error by producing valid JSON
			return fastjson.Parse(`{"error": "Internal Server Error"}`)
		}
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	return value, nil
}

// WriteJSONResponse writes a JSON response with proper headers
func WriteJSONResponse(w http.ResponseWriter, value *fastjson.Value) error {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	_, err := w.Write(value.MarshalTo(nil))
	return err
}

// ParseToFastjson converts any Go value to fastjson.Value
func ParseToFastjson(data interface{}) (*fastjson.Value, error) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return parseJSON(jsonBytes)
}

func decompressGzip(input io.Reader) ([]byte, error) {
	reader, err := gzip.NewReader(input)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

func decompressDeflate(input io.Reader) ([]byte, error) {
	reader := flate.NewReader(input)
	defer reader.Close()

	return io.ReadAll(reader)
}

func removeNonPrintableChars(s string) string {
	var result []rune
	for _, c := range s {
		if c >= 32 && c <= 126 && c != '\\' && c != '"' && c != '\x00' {
			result = append(result, c)
		}
	}
	return string(result)
}
