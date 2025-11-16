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
			MaxIdleConns:        HTTPMaxIdleConns,
			IdleConnTimeout:     time.Duration(config.HttpTimeout) * time.Second,
			DisableCompression:  false,
			MaxIdleConnsPerHost: HTTPMaxIdleConnsPerHost,
			MaxConnsPerHost:     HTTPMaxConnsPerHost,
		},
	}
}

func getDefaultJSONResponse() *fastjson.Value {
	return fastjson.MustParse(JSONResponseDefault)
}

func getEmptyJSONResponse() *fastjson.Value {
	return fastjson.MustParse(JSONResponseEmpty)
}

func parseJSON(data []byte) (*fastjson.Value, error) {
	var p fastjson.Parser
	value, err := p.ParseBytes(data)
	if err != nil {
		errorMsg := string(data)
		if strings.Contains(errorMsg, ErrorMessageInternalServerError) {
			// Handle the error by producing valid JSON
			return fastjson.Parse(JSONResponseInternalServerError)
		}
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	return value, nil
}

// WriteJSONResponse writes a JSON response with proper headers
func WriteJSONResponse(w http.ResponseWriter, value *fastjson.Value) error {
	w.Header().Set(HeaderContentType, ContentTypeJSONCharset)
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
		if c >= CompressionThresholdMin && c <= CompressionThresholdMax && c != CompressionEscapeChar && c != CompressionQuoteChar && c != CompressionNullChar {
			result = append(result, c)
		}
	}
	return string(result)
}
