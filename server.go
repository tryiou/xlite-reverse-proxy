package main

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/valyala/fastjson"
)

func (s *Server) pruneHashStorage() {
	for _, storage := range s.hashesStorage {
		maxLen := 4
		if len(storage) <= maxLen {
			continue // No need to prune if there are maxLen or fewer entries
		}
		// Create a slice to store the heights
		heights := make([]int, 0, len(storage))
		for height := range storage {
			heights = append(heights, height)
		}
		// Sort the heights in descending order
		sort.Sort(sort.Reverse(sort.IntSlice(heights)))
		// Remove entries with heights beyond the maxLen highest
		for i := maxLen; i < len(heights); i++ {
			//	logger.Printf("[server%d], Pruning hashesHistory %v\n", s.id, heights[i])
			delete(storage, heights[i])
		}
	}
}

func (s *Server) server_GetPing() error {
	payloadMethod := "ping"
	payloadParams := []interface{}{}
	response, err := makeHTTPRequest(*s, http.MethodPost, payloadMethod, payloadParams, config.HttpTimeout)
	if err != nil {
		s.ping = 0
		s.coinsMap = make(map[string]Coin)
		s.getfees = getDefaultJSONResponse()
		s.getheights = getDefaultJSONResponse()
		return fmt.Errorf("server_GetPing, failed to make HTTP request: %w", err)
	}
	jsonValue, err := parseJSON(response)
	if err != nil {
		s.ping = 0
		s.coinsMap = make(map[string]Coin)
		s.getfees = getDefaultJSONResponse()
		s.getheights = getDefaultJSONResponse()
		return fmt.Errorf("server_GetPing, failed to parse JSON response: %w", err)
	}

	resultValue := jsonValue.Get("result")
	// Check if the resultValue is nil, indicating an error
	if resultValue == nil {
		// Handle the error
		s.ping = 0
		s.coinsMap = make(map[string]Coin)
		s.getfees = getDefaultJSONResponse()
		s.getheights = getDefaultJSONResponse()
		return fmt.Errorf("server_GetPing, error: failed to retrieve the 'result' element")
	}

	// Check if the resultValue is an integer and equal to 1
	if resultValue.Type() == fastjson.TypeNumber && resultValue.GetInt() == 1 {
		s.ping = 1
	} else {
		s.ping = 0
	}
	return nil
}

func (s *Server) server_GetBlock(coin string, blockHash string) (*fastjson.Value, error) {
	payloadMethod := "getblock"
	payloadParams := []interface{}{coin, blockHash, "true"}
	//fmt.Printf("payloadParams: %q\n", payloadParams)
	response, err := makeHTTPRequest(*s, http.MethodPost, payloadMethod, payloadParams, config.HttpTimeout)
	if err != nil {
		return nil, fmt.Errorf("server_GetBlock: failed to make HTTP request: %w", err)
	}
	jsonValue, err := parseJSON(response)
	if err != nil {
		return nil, fmt.Errorf("server_GetBlock: failed to parse JSON response: %w", err)
	}

	//fmt.Printf("jsonValue: %q\n", jsonValue)
	jsonErr := jsonValue.Get("error")
	if jsonErr.Type() != fastjson.TypeNull {
		return nil, fmt.Errorf("server_GetBlock: JSON response contains an error: %s", jsonErr.String())
	}

	jsonResult := jsonValue.Get("result")
	if jsonResult.Type() != fastjson.TypeObject {
		return nil, fmt.Errorf("server_GetBlock: JSON response does not contain a valid block data")
	}

	return jsonResult, nil
}

func (s *Server) server_GetBlockHash(coin string, height int) (string, error) {
	//coinObj := s.coinsMap[coin]

	payloadMethod := "getblockhash"
	payloadParams := []interface{}{coin, height}
	if height == -1 {
		logger.Printf("server_GetBlockHash called with height = -1, server:%d,%s,%d", s.id, coin, height)
		return "", nil
	}
	response, err := makeHTTPRequest(*s, http.MethodPost, payloadMethod, payloadParams, config.HttpTimeout)
	if err != nil {
		return "", fmt.Errorf("server_GetBlockHash: failed to make HTTP request: %w", err)
	}
	jsonValue, err := parseJSON(response)
	if err != nil {
		return "", fmt.Errorf("server_GetBlockHash: failed to parse JSON response: %w", err)
	}

	jsonErr := jsonValue.Get("error")
	if jsonErr.Type() != fastjson.TypeNull {
		// Handle the error by setting getBlockHash to ""
		return "", fmt.Errorf("server_GetBlockHash: JSON response contains an error: %s", jsonErr.String())
	}
	return removeNonPrintableChars(jsonValue.Get("result").String()), nil
}

func (s *Server) server_GetFees() error {
	payloadMethod := "fees"
	payloadParams := []interface{}{}

	response, err := makeHTTPRequest(*s, http.MethodPost, payloadMethod, payloadParams, config.HttpTimeout)
	if err != nil {
		s.getfees = getDefaultJSONResponse()
		return fmt.Errorf("server_GetFees, failed to make HTTP request: %w", err)
	}
	jsonValue, err := parseJSON(response)
	if err != nil {
		s.getfees = getDefaultJSONResponse()
		return fmt.Errorf("server_GetFees, failed to parse JSON response: %w", err)
	}
	s.getfees = jsonValue
	return nil
}

func (s *Server) server_GetHeights() error {
	payloadMethod := "heights"
	payloadParams := []interface{}{}
	response, err := makeHTTPRequest(*s, http.MethodPost, payloadMethod, payloadParams, config.HttpTimeout)
	if err != nil {
		s.getheights = getDefaultJSONResponse()
		return fmt.Errorf("server_GetHeights, failed to make HTTP request: %w", err)
	}
	json, err := parseJSON(response)
	if err != nil {
		s.getheights = getDefaultJSONResponse()
		return fmt.Errorf("server_GetHeights, failed to parse JSON response: %w", err)
	}
	s.getheights = json
	s.sortGetHeightsKeys()
	return nil
}

func (s *Server) sortGetHeightsKeys() {
	if s.getheights != nil {
		resultObj := s.getheights.Get("result")
		if resultObj != nil && resultObj.Type() == fastjson.TypeObject {
			keys := make([]string, 0)
			resultObj.GetObject().Visit(func(key []byte, value *fastjson.Value) {
				keys = append(keys, string(key))
			})
			// Sort the keys alphabetically
			sort.Strings(keys)
			// Create a new JSON object with sorted keys
			sortedResultObj := fastjson.MustParse("{}")
			for _, key := range keys {
				value := resultObj.Get(key)
				sortedResultObj.Set(key, value)
			}
			// Replace the "result" object with the sorted one
			s.getheights.Set("result", sortedResultObj)
		}
	}
}
