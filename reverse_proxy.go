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

// Helper functions for standardized error responses
func makeServiceUnavailableResponse() *fastjson.Value {
	return fastjson.MustParse(JSONResponseServiceUnavailable)
}

func makeErrorResponse(message string, details ...string) *fastjson.Value {
	if len(details) > 0 {
		return fastjson.MustParse(fmt.Sprintf(JSONResponseServerErrorWithDetailsTemplate, message, details[0]))
	}
	return fastjson.MustParse(fmt.Sprintf(JSONResponseServerErrorTemplate, message))
}

func logError(operation, message string, err error) {
	logger.Printf(LogPrefixError+" %s: %v", operation, err)
}

func writeErrorResponse(w http.ResponseWriter, status int, message string, details ...string) {
	w.WriteHeader(status)
	response := makeErrorResponse(message, details...)
	_ = WriteJSONResponse(w, response)
}

// Add request context to error responses
type RequestContext struct {
	IP        string
	Method    string
	Path      string
	Timestamp time.Time
	UserAgent string
}

func createRequestContext(r *http.Request) RequestContext {
	ip := extractIPFromRequest(r)
	return RequestContext{
		IP:        ip,
		Method:    r.Method,
		Path:      r.URL.Path,
		Timestamp: time.Now(),
		UserAgent: r.Header.Get("User-Agent"),
	}
}

func extractIPFromRequest(r *http.Request) string {
	reqClientIP := r.Header.Get("X-Forwarded-For")
	if reqClientIP != "" {
		ips := strings.Split(reqClientIP, ",")
		return strings.TrimSpace(ips[0])
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "unknown"
	}
	return ip
}

func logErrorWithContext(ctx RequestContext, operation string, err error) {
	logger.Printf(LogPrefixError+" [IP:%s Method:%s Path:%s UA:%s] %s: %v",
		ctx.IP, ctx.Method, ctx.Path, ctx.UserAgent, operation, err)
}

// writeGenericErrorResponse writes a generic error response to the client while logging detailed error internally
func writeGenericErrorResponse(w http.ResponseWriter, status int, genericMsg string, operation string, err error) {
	logger.Printf(LogPrefixError+" %s: %v", operation, err)
	w.WriteHeader(status)
	response := fastjson.MustParse(fmt.Sprintf(`{"error": "%s"}`, genericMsg))
	_ = WriteJSONResponse(w, response)
}

// writeServerErrorResponse writes a server error response with generic message to client
func writeServerErrorResponse(w http.ResponseWriter, operation string, err error) {
	writeGenericErrorResponse(w, HTTPStatusServiceUnavailable, ErrorMessageServiceUnavailable, operation, err)
}

// writeBadRequestResponse writes a bad request error response
func writeBadRequestResponse(w http.ResponseWriter, operation string, err error) {
	logger.Printf(LogPrefixError+" %s: %v", operation, err)
	w.WriteHeader(HTTPStatusBadRequest)
	response := fastjson.MustParse(fmt.Sprintf(`{"error": "%s"}`, ErrorMessageBadRequest))
	_ = WriteJSONResponse(w, response)
}

// writeNotFoundResponse writes a not found error response
func writeNotFoundResponse(w http.ResponseWriter) {
	logger.Print(LogPrefixError + " " + ErrorMessageNotFound)
	w.WriteHeader(HTTPStatusNotFound)
	response := fastjson.MustParse(fmt.Sprintf(`{"error": "%s"}`, ErrorMessageNotFound))
	_ = WriteJSONResponse(w, response)
}

func logCachedRequest(ip, endpoint string, start time.Time) {
	elapsed := time.Since(start)
	logger.Printf(LogPrefixRevProxy+" %s request %s relayed OK from cache, "+LogExecTimerFormat, ip, endpoint, elapsed)
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
		// Check if the request path is in the acceptedPaths list first
		if !isPathAccepted(req.URL.Path) {
			writeNotFoundResponse(rw)
			return
		}

		requestData, err := extractRequestData(req)
		if err != nil {
			writeBadRequestResponse(rw, "extractRequestData", err)
			return
		}

		startTimer := time.Now()

		switch {
		case req.URL.Path == "/servers" || requestData.Method == "servers":
			if servers.GlobalCoinServerIDs == nil {
				writeServerErrorResponse(rw, "servers endpoint", fmt.Errorf("global coin server IDs not available"))
				return
			}
			response := servers.GlobalCoinServerIDs
			if err := writeResponseChecked(rw, response); err != nil {
				return
			}
			logCachedRequest(requestData.Ip, "servers", startTimer)
			return

		case req.URL.Path == "/heights" || req.URL.Path == "/height" || requestData.Method == "heights" || requestData.Method == "height":
			if servers.GlobalHeights == nil {
				writeServerErrorResponse(rw, "heights endpoint", fmt.Errorf("global heights not available"))
				return
			}
			response := servers.GlobalHeights
			if err := writeResponseChecked(rw, response); err != nil {
				return
			}
			logCachedRequest(requestData.Ip, "heights", startTimer)
			return

		case req.URL.Path == "/fees" || requestData.Method == "fees":
			if servers.GlobalFees == nil {
				writeServerErrorResponse(rw, "fees endpoint", fmt.Errorf("global fees not available"))
				return
			}
			response := servers.GlobalFees
			if err := writeResponseChecked(rw, response); err != nil {
				return
			}
			logCachedRequest(requestData.Ip, "fees", startTimer)
			return

		case req.URL.Path == "/ping" || requestData.Method == "ping":
			response := fastjson.MustParse(JSONResponsePingSuccess)
			if err := writeResponseChecked(rw, response); err != nil {
				return
			}
			elapsedTimer := time.Since(startTimer)
			logger.Printf(LogPrefixRevProxy+" %s request ping relayed OK, "+LogExecTimerFormat+"\n", requestData.Ip, elapsedTimer)
			return

		default:
			if !isMethodAccepted(requestData.Method) {
				writeNotFoundResponse(rw)
				return
			}

			coin, err := extractCoin(requestData)
			if err != nil {
				writeBadRequestResponse(rw, "extractCoin", err)
				return
			}

			server, err := retryWithRandomValidServer(rw, req, servers, coin, &requestData, RetryAttemptsDefault)
			if err != nil {
				writeServerErrorResponse(rw, "retryWithRandomValidServer", err)
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
	if maxRetries <= 0 {
		return nil, fmt.Errorf("invalid retry configuration: maxRetries must be positive, got %d", maxRetries)
	}
	var lastError error
	var actualAttempts int

	for i := 0; i < maxRetries; i++ {
		actualAttempts = i + 1
		if i > 0 { // Only log for attempts after the first one
			logger.Printf(LogPrefixRevProxy+" Attempt %d/%d for coin %s", i+1, maxRetries, coin)
		}

		randomValidServerID, err := servers.GetRandomValidServerID(coin)
		if err != nil {
			logError("getRandomValidServer", fmt.Sprintf("failed to get random valid server for coin %s (attempt %d/%d)", coin, i+1, maxRetries), err)
			lastError = err
			// Permanent error: coin not found or no servers for coin — don't retry
			if errors.Is(err, ErrCoinNotFound) || errors.Is(err, ErrServerIDsArrayNotFound) || errors.Is(err, ErrNoServerForCoin) {
				break
			}
			continue
		}

		// Use read lock for getting server data
		locks.servers.RLock()
		server, exists := servers.GetServerByID(randomValidServerID)
		locks.servers.RUnlock()

		if !exists {
			logError("GetServerByID", fmt.Sprintf("server %d not found for coin %s (attempt %d/%d)", randomValidServerID, coin, i+1, maxRetries), fmt.Errorf(ErrorMessageServerIDNotFound, randomValidServerID))
			lastError = fmt.Errorf(ErrorMessageServerNotFound)
			continue
		}

		err = updateRequestHeaders(req, &server, *requestData)
		if err != nil {
			logError("updateRequestHeaders", fmt.Sprintf("failed to update request headers for server %d (attempt %d/%d)", server.id, i+1, maxRetries), err)
			lastError = err
			continue
		}

		resp, err := handleOriginServerResponse(req, &server)
		if err != nil {
			if strings.Contains(err.Error(), "context canceled") {
				logError("handleOriginServerResponse", "request context canceled", err)
				return nil, err
			}

			logError("handleOriginServerResponse", fmt.Sprintf("server %d response failed for coin %s (attempt %d/%d)", server.id, coin, i+1, maxRetries), err)
			servers.RemoveServerFromGlobalCoinList(coin, server.id)
			lastError = err

			if strings.Contains(err.Error(), " status: 4") {
				logger.Printf(LogPrefixRevProxy+" Client error detected, stopping retries: %v", err)
				return nil, err
			}
			continue
		}

		if err := WriteJSONResponse(rw, resp); err != nil {
			return nil, fmt.Errorf("failed to write response: %v", err)
		}
		return &server, nil
	}

	// All retries exhausted
	// Don't write response here - let the caller handle it to avoid double responses
	// Provide more context in the error message
	if lastError != nil {
		return nil, fmt.Errorf("failed after %d/%d attempt(s) for coin %s: %w", actualAttempts, maxRetries, coin, lastError)
	}
	return nil, fmt.Errorf("failed after %d/%d attempt(s) for coin %s", actualAttempts, maxRetries, coin)
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
	req.Header.Set(HeaderAcceptEncoding, ContentEncodingGzip)

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

// handleOriginServerResponse sends the request to the origin server, parses the response,
// and returns it. The caller is responsible for writing the response to the client.
func handleOriginServerResponse(req *http.Request, server *Server) (*fastjson.Value, error) {
	originServerResponse, err := sendRequestToOriginServer(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to origin server: %v", err)
	}

	responseBody, err := decompressResponseBody(originServerResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress response body: %v", err)
	}

	orgResponse, err := parseAndNormalizeResponse(responseBody, server)
	if err != nil {
		return nil, fmt.Errorf("failed to parse and normalize response: %v", err)
	}

	return orgResponse, nil
}

func extractCoin(requestData RequestData) (string, error) {
	if len(requestData.Params) == 0 {
		return "", errors.New(ErrorMessageMissingCoinParam)
	}
	coin, ok := requestData.Params[0].(string)
	if !ok {
		return "", errors.New(ErrorMessageInvalidCoinType)
	}
	return coin, nil
}

// decompressResponseBody decompresses the response body based on the content encoding.
func decompressResponseBody(response *http.Response) ([]byte, error) {
	contentEncoding := response.Header.Get(HeaderContentEncoding)
	switch contentEncoding {
	case ContentEncodingGzip:
		return decompressGzip(response.Body)
	case ContentEncodingDeflate:
		return decompressDeflate(response.Body)
	case "":
		return io.ReadAll(response.Body)
	default:
		return nil, fmt.Errorf("unsupported compression algorithm: %s", contentEncoding)
	}
}

// extractRequestData extracts the method, parameters, and client IP from the request.
func extractRequestData(req *http.Request) (RequestData, error) {
	// Validate HTTP request first
	validator := &Validator{}
	validator.ValidateHTTPRequest(req)

	if validator.HasErrors() {
		return RequestData{}, validator.Validate()
	}

	buf, err := io.ReadAll(req.Body)
	if err != nil {
		return RequestData{}, fmt.Errorf("failed to read request body: %w", err)
	}

	var requestData RequestData

	switch req.Method {
	case http.MethodGet:
		// For GET requests, extract method from path
		requestData.Method = strings.TrimPrefix(req.URL.Path, "/")
		requestData.Params = []interface{}{}
		requestData.Path = req.URL.Path

		// Extract and validate IP for GET requests
		ip, err := extractAndValidateIP(req)
		if err != nil {
			return RequestData{}, err
		}
		requestData.Ip = ip

	case http.MethodPost:
		if err := json.Unmarshal(buf, &requestData); err != nil {
			return RequestData{}, fmt.Errorf("failed to parse request JSON: %w", err)
		}
		requestData.Path = req.URL.Path

		// Extract and validate IP for POST requests
		ip, err := extractAndValidateIP(req)
		if err != nil {
			return RequestData{}, err
		}
		requestData.Ip = ip

		// Validate request data for POST requests
		validator = &Validator{}
		validator.ValidateRequestData(requestData)

		if validator.HasErrors() {
			return RequestData{}, validator.Validate()
		}
	}

	return requestData, nil
}

// isCachedEndpoint checks if the path is a cached endpoint that doesn't need full validation
func isCachedEndpoint(path string) bool {
	cachedEndpoints := []string{"/servers", "/heights", "/fees", "/ping"}
	for _, endpoint := range cachedEndpoints {
		if path == endpoint {
			return true
		}
	}
	return false
}

// extractAndValidateIP extracts and validates the client IP from the request.
func extractAndValidateIP(req *http.Request) (string, error) {
	var ip string
	reqClientIP := req.Header.Get(HeaderXForwardedFor)

	if reqClientIP != "" {
		ips := strings.Split(reqClientIP, ",")
		ip = strings.TrimSpace(ips[0])
	} else {
		var err error
		ip, _, err = net.SplitHostPort(req.RemoteAddr)
		if err != nil {
			return "", fmt.Errorf("failed to extract client IP: %w", err)
		}
	}

	// Validate the extracted IP
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("invalid client IP address: %s", ip)
	}

	return ip, nil
}

// extractMethodParamsIp extracts the method, parameters, and client IP from the request.
// This function is kept for backward compatibility but now delegated to extractRequestData.
func extractMethodParamsIp(rdr io.Reader, req *http.Request) (RequestData, error) {
	var requestData RequestData
	var ip string
	var err error

	reqClientIP := req.Header.Get(HeaderXForwardedFor)
	if reqClientIP != "" {
		ips := strings.Split(reqClientIP, ",")
		ip = strings.TrimSpace(ips[0])
	} else {
		ip, _, err = net.SplitHostPort(req.RemoteAddr)
		if err != nil {
			return RequestData{}, fmt.Errorf("failed to extract client IP: %w", err)
		}
	}

	switch req.Method {
	case http.MethodGet:
		method := req.URL.Path[1:]
		params := []interface{}{}
		return RequestData{Method: method, Params: params, Ip: ip}, nil
	case http.MethodPost:
		err := json.NewDecoder(rdr).Decode(&requestData)
		if err != nil {
			return RequestData{}, fmt.Errorf("failed to parse request JSON: %w", err)
		}
		requestData.Ip = ip
		return requestData, nil
	}

	requestData.Path = req.URL.Path
	return RequestData{}, fmt.Errorf("unsupported HTTP method: %s", req.Method)
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
	client := globalConfig.GetClient()
	if client == nil {
		return nil, fmt.Errorf("HTTP client not initialized")
	}

	resp, err := client.Do(req)
	if err != nil {
		// Provide more specific error messages based on the error type
		if netErr, ok := err.(net.Error); ok {
			if netErr.Timeout() {
				return nil, fmt.Errorf("request timeout: %w", err)
			}
			if netErr.Temporary() {
				return nil, fmt.Errorf("temporary network error: %w", err)
			}
		}

		// Handle connection errors
		if strings.Contains(err.Error(), "connection refused") {
			return nil, fmt.Errorf("server connection refused: %w", err)
		}
		if strings.Contains(err.Error(), "no such host") {
			return nil, fmt.Errorf("DNS resolution failed for host: %w", err)
		}
		if strings.Contains(err.Error(), "too many open files") {
			return nil, fmt.Errorf("system resource limit reached: %w", err)
		}

		return nil, fmt.Errorf("failed to send request to server: %w", err)
	}

	if resp.StatusCode != HTTPStatusOK {
		return nil, fmt.Errorf("unexpected server response status: %s", resp.Status)
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
	logger.Printf(LogPrefixRevProxy+" %s request %s %s relayed OK to server[%d], "+LogExecTimerFormat+"\n", requestData.Ip, requestData.Method, bufParams, server.id, elapsedTimer)
}

// isPathAccepted checks if the request path is in the acceptedPaths list.
func isPathAccepted(path string) bool {
	cfg := globalConfig.GetConfig()
	for _, acceptedPath := range cfg.AcceptedPaths {
		if path == acceptedPath {
			return true
		}
	}
	return false
}

// isMethodAccepted checks if the request method is in the acceptedMethods list.
func isMethodAccepted(method string) bool {
	cfg := globalConfig.GetConfig()
	for _, acceptedMethod := range cfg.AcceptedMethods {
		if method == acceptedMethod {
			return true
		}
	}
	return false
}
