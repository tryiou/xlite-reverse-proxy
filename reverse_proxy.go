// reverse_proxy.go
// Reverse proxy for handling 'cc-daemon' requests, extracting 'coin' and 'method' parameters,
// transforming requests for EXR syntax, and relaying them to a valid server in the list.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/valyala/fastjson"
)

// Helper function for "Service unavailable" responses
func makeServiceUnavailableResponse() *fastjson.Value {
	return fastjson.MustParse(`{"result": null, "error": "Service unavailable"}`)
}

func logCachedRequest(ip, endpoint string, start time.Time) {
	elapsed := time.Since(start)
	logger.Printf("[revProxy_Serv] %s request %s relayed OK from cache, exec_timer:%s\n", ip, endpoint, elapsed)
}

func writeResponseChecked(w http.ResponseWriter, resp *fastjson.Value) error {
	if err := WriteJSONResponse(w, resp); err != nil {
		logger.Printf("*error writing response: %v", err)
		return err
	}
	return nil
}

// reverseProxy starts a reverse proxy server on the specified port.
func reverseProxyHandler(servers *Servers) http.HandlerFunc {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		requestData, err := extractRequestData(req)
		if err != nil {
			return
		}

		// Check if the request path is in the acceptedPaths list
		if !isPathAccepted(req.URL.Path) {
			logger.Print("*error ", req.URL.Path, " not in the acceptedPaths list ", requestData.Ip)
			http.NotFound(rw, req)
			return
		}

		startTimer := time.Now()

		switch {
		case req.URL.Path == "/servers" || requestData.Method == "servers":
			response := servers.GlobalCoinServerIDs
			if err := writeResponseChecked(rw, response); err != nil {
				return
			}
			logCachedRequest(requestData.Ip, "servers", startTimer)
			return

		case req.URL.Path == "/heights" || req.URL.Path == "/height" || requestData.Method == "heights" || requestData.Method == "height":
			response := servers.GlobalHeights
			if err := writeResponseChecked(rw, response); err != nil {
				return
			}
			logCachedRequest(requestData.Ip, "heights", startTimer)
			return

		case req.URL.Path == "/fees" || requestData.Method == "fees":
			response := servers.GlobalFees
			if err := writeResponseChecked(rw, response); err != nil {
				return
			}
			logCachedRequest(requestData.Ip, "fees", startTimer)
			return

		case req.URL.Path == "/ping" || requestData.Method == "ping":
			response := fastjson.MustParse("1")
			if err := writeResponseChecked(rw, response); err != nil {
				return
			}
			elapsedTimer := time.Since(startTimer)
			logger.Printf("[revProxy_Serv] %s request ping relayed OK, exec_timer:%s\n", requestData.Ip, elapsedTimer)
			return

		default:
			if !isMethodAccepted(requestData.Method) {
				logger.Print("*error ", requestData.Method, " not in the acceptedMethods list ", requestData.Ip)
				http.NotFound(rw, req)
				return
			}

			coin, err := extractCoin(requestData)
			if err != nil {
				logger.Printf("*error Failed to extract coin from params: %v", err)
				return
			}

			server, err := retryWithRandomValidServer(rw, req, servers, coin, &requestData, 3)
			if err != nil {
				logger.Printf("*error: %v", err)
				response := fastjson.MustParse(`{"result": null, "error": "No valid server for ` + coin + `"}`)
				_ = WriteJSONResponse(rw, response)
				return
			}

			logRequest(*server, &requestData, req.URL, startTimer)
		}
	})

}

func reverseProxy(port int, servers *Servers) {
	logger.Print("ReverseProxy started, Listening on ", port)

	srv := &http.Server{
		Addr:     ":" + strconv.Itoa(port),
		Handler:  limit(reverseProxyHandler(servers)),
		ErrorLog: log.New(logger.Writer(), "", log.LstdFlags),
	}

	err := srv.ListenAndServe()
	if err != nil {
		logger.Fatalf("*error reverseProxy: %v", err)
	}
}

// retryWithRandomValidServer selects a random valid server, sends the request, and handles the response.
func retryWithRandomValidServer(rw http.ResponseWriter, req *http.Request, servers *Servers, coin string, requestData *RequestData, maxRetries int) (*Server, error) {
	for i := 0; i < maxRetries; i++ {
		randomValidServerID, err := servers.GetRandomValidServerID(coin)
		if err != nil {
			logger.Printf("*error failed to get random valid server, method: %s, error: %v", requestData.Method, err)
			sanitizedResponse := makeServiceUnavailableResponse()
			_ = WriteJSONResponse(rw, sanitizedResponse)
			return nil, err
		}

		server, exists := servers.GetServerByID(randomValidServerID)
		if !exists {
			logger.Println("*error Server not found")
			sanitizedResponse := makeServiceUnavailableResponse()
			_ = WriteJSONResponse(rw, sanitizedResponse)
			return nil, fmt.Errorf("server not found")
		}

		err = updateRequestHeaders(req, &server, *requestData)
		if err != nil {
			logger.Printf("*error updateRequestHeaders: %v", err)
			sanitizedResponse := makeServiceUnavailableResponse()
			_ = WriteJSONResponse(rw, sanitizedResponse)
			return nil, err
		}

		err = handleOriginServerResponse(rw, req, &server)
		if err != nil {
			if strings.Contains(err.Error(), "context canceled") {
				return nil, err
			}
			servers.RemoveServerFromGlobalCoinList(coin, server.id)
			logger.Printf("*error %d handleOriginServerResponse: %v pruning server[%d]", i, err, server.id)
		} else {
			return &server, nil
		}
	}

	logger.Println("All retries exhausted. Unable to process the request.")
	sanitizedResponse := makeServiceUnavailableResponse()
	_ = WriteJSONResponse(rw, sanitizedResponse)
	return nil, fmt.Errorf("all retries exhausted")
}

// updateRequestHeaders updates the request headers for the origin server.
func updateRequestHeaders(req *http.Request, server *Server, requestData RequestData) error {
	originServerURL, err := url.Parse(server.url)
	if err != nil {
		return fmt.Errorf("failed to parse origin server URL: %w", err)
	}

	if server.exr {
		req, err = transformRequestToEXRSyntax(req, server.url, requestData)
		if err != nil {
			return fmt.Errorf("failed to transform request to EXR syntax: %w", err)
		}
	}

	req.Host = originServerURL.Host
	req.URL.Host = originServerURL.Host
	req.URL.Scheme = originServerURL.Scheme
	req.RequestURI = ""
	req.Header.Set("Accept-Encoding", "gzip")

	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return fmt.Errorf("failed to read request body: %w", err)
		}
		req.Body = io.NopCloser(bytes.NewBuffer(body))
		req.ContentLength = int64(len(body))
	}

	return nil
}

// handleOriginServerResponse sends the request to the origin server and handles the response.
func handleOriginServerResponse(rw http.ResponseWriter, req *http.Request, server *Server) error {
	originServerResponse, err := sendRequestToOriginServer(req)
	if err != nil {
		return fmt.Errorf("failed to send request to origin server: %v", err)
	}

	responseBody, err := decompressResponseBody(originServerResponse)
	if err != nil {
		return fmt.Errorf("failed to decompress response body: %v", err)
	}

	orgResponse, err := parseAndNormalizeResponse(responseBody, server)
	if err != nil {
		return fmt.Errorf("failed to parse and normalize response: %v", err)
	}

	err = WriteJSONResponse(rw, orgResponse)
	if err != nil {
		return fmt.Errorf("failed to write response: %v", err)
	}

	return nil
}

func extractCoin(requestData RequestData) (string, error) {
	if len(requestData.Params) == 0 {
		return "", errors.New("missing coin parameter")
	}
	coin, ok := requestData.Params[0].(string)
	if !ok {
		return "", errors.New("invalid coin type")
	}
	return coin, nil
}

// decompressResponseBody decompresses the response body based on the content encoding.
func decompressResponseBody(response *http.Response) ([]byte, error) {
	contentEncoding := response.Header.Get("Content-Encoding")
	switch contentEncoding {
	case "gzip":
		return decompressGzip(response.Body)
	case "deflate":
		return decompressDeflate(response.Body)
	case "":
		return io.ReadAll(response.Body)
	default:
		return nil, fmt.Errorf("unsupported compression algorithm: %s", contentEncoding)
	}
}

// extractRequestData extracts the method, parameters, and client IP from the request.
func extractRequestData(req *http.Request) (RequestData, error) {
	buf, _ := io.ReadAll(req.Body)
	rdr1 := io.NopCloser(bytes.NewBuffer(buf))
	rdr2 := io.NopCloser(bytes.NewBuffer(buf))
	requestData, err := extractMethodParamsIp(rdr1, req)

	if err != nil {
		return RequestData{Method: "null", Params: nil, Ip: "null"}, err
	}
	req.Body = rdr2
	return requestData, nil
}

// extractMethodParamsIp extracts the method, parameters, and client IP from the request.
func extractMethodParamsIp(rdr io.Reader, req *http.Request) (RequestData, error) {
	var requestData RequestData
	var ip string
	var err error

	reqClientIP := req.Header.Get("X-Forwarded-For")
	if reqClientIP != "" {
		ips := strings.Split(reqClientIP, ",")
		ip = strings.TrimSpace(ips[0])
	} else {
		ip, _, err = net.SplitHostPort(req.RemoteAddr)
		if err != nil {
			logger.Printf("error extracting client ip from request: %v", err)
			return RequestData{Method: "null", Params: nil, Ip: ""}, err
		}
	}

	if req.Method == http.MethodGet {
		method := req.URL.Path[1:]
		params := []interface{}{}
		return RequestData{Method: method, Params: params, Ip: ip}, nil
	} else if req.Method == http.MethodPost {
		err := json.NewDecoder(rdr).Decode(&requestData)
		if err != nil {
			return RequestData{Method: "null", Params: nil, Ip: ip}, nil
		}
		requestData.Ip = ip
		return requestData, nil
	}

	requestData.Path = req.URL.Path
	return RequestData{}, nil
}

// transformRequestToEXRSyntax transforms the request to EXR syntax.
func transformRequestToEXRSyntax(req *http.Request, serverURL string, requestData RequestData) (*http.Request, error) {
	exrURL := serverURL + "/xrs/" + requestData.Method
	parsedURL, err := url.Parse(exrURL)
	if err != nil {
		return nil, err
	}
	req.URL = parsedURL

	exrRequestBody, err := json.Marshal(requestData.Params)
	if err != nil {
		return nil, err
	}

	req.Body = io.NopCloser(bytes.NewBuffer(exrRequestBody))
	return req, nil
}

// sendRequestToOriginServer sends the request to the origin server.
func sendRequestToOriginServer(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("*error unexpected server response status: %s", resp.Status)
	}

	return resp, nil
}

// parseAndNormalizeResponse parses and normalizes the response.
func parseAndNormalizeResponse(responseBody []byte, server *Server) (*fastjson.Value, error) {
	var parsedResponse *fastjson.Value
	var err error

	if len(responseBody) == 0 {
		parsedResponse = getDefaultJSONResponse()
	} else {
		parsedResponse, err = parseJSON(responseBody)
		if err != nil {
			return nil, err
		}

		if fastjson.Exists(parsedResponse.MarshalTo(nil), "code") && fastjson.Exists(parsedResponse.MarshalTo(nil), "error") {
			errorCode := parsedResponse.GetInt("code")
			errorMessage := parsedResponse.Get("error").String()
			return nil, fmt.Errorf("server[%d] code: %d error: %s", server.id, errorCode, errorMessage)
		}
	}

	return parsedResponse, nil
}

// logRequest logs the request details.
func logRequest(server Server, requestData *RequestData, reqURL *url.URL, startTimer time.Time) {
	var bufParams interface{}
	if len(requestData.Params) > 0 {
		bufParams = requestData.Params[0]
	} else {
		bufParams = "[]"
	}
	elapsedTimer := time.Since(startTimer)
	logger.Printf("[revProxy_Serv] %s request %s %s relayed OK to server[%d], exec_timer:%s\n", requestData.Ip, requestData.Method, bufParams, server.id, elapsedTimer)
}

// isPathAccepted checks if the request path is in the acceptedPaths list.
func isPathAccepted(path string) bool {
	for _, acceptedPath := range config.AcceptedPaths {
		if path == acceptedPath {
			return true
		}
	}
	return false
}

// isMethodAccepted checks if the request method is in the acceptedMethods list.
func isMethodAccepted(method string) bool {
	for _, acceptedMethod := range config.AcceptedMethods {
		if method == acceptedMethod {
			return true
		}
	}
	return false
}
