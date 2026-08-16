package main

import (
	"errors"
	"time"
)

// Constants for timeouts and intervals - internal implementation details only
const (
	// RetryAttemptsDefault is the default maximum number of retry attempts for server requests
	RetryAttemptsDefault = 3

	// PingRetryAttempts is the maximum number of ping attempts per health-check cycle
	// before a server is evicted. A server is evicted only if all attempts fail.
	PingRetryAttempts = 3

	// UpdateIntervalDefault is the default interval for server updates
	UpdateIntervalDefault = 20 * time.Second

	// VisitorCleanupInterval is the interval for cleaning up old visitor entries in rate limiter
	VisitorCleanupInterval = 3 * time.Minute

	// BlockHashToleranceRange is the tolerance range around consensus height for block hash validation
	BlockHashToleranceRange = 5

	// UpdateIntervalDynamic is the interval for dynamic server updates
	UpdateIntervalDynamic = 5 * time.Minute

	// LogRotationInterval is the interval for checking log file size
	LogRotationInterval = 5 * time.Minute

	// BackoffBaseInterval is the base interval for exponential backoff on server failures
	BackoffBaseInterval = 20 * time.Second

	// BackoffMaxInterval is the maximum backoff interval for server failures
	BackoffMaxInterval = 5 * time.Minute
)

// Constants for HTTP and networking
const (
	// ContentTypeJSON is the JSON content type header value
	ContentTypeJSON = "application/json"

	// ContentTypeJSONCharset is the JSON content type with charset
	ContentTypeJSONCharset = "application/json;charset=UTF-8"

	// ContentEncodingGzip is the gzip content encoding value
	ContentEncodingGzip = "gzip"

	// ContentEncodingDeflate is the deflate content encoding value
	ContentEncodingDeflate = "deflate"

	// HeaderAcceptEncoding is the accept encoding header name
	HeaderAcceptEncoding = "Accept-Encoding"

	// HeaderContentEncoding is the content encoding header name
	HeaderContentEncoding = "Content-Encoding"

	// HeaderContentType is the content type header name
	HeaderContentType = "Content-Type"

	// HeaderXForwardedFor is the X-Forwarded-For header name
	HeaderXForwardedFor = "X-Forwarded-For"

	// HTTPMethodPost is the HTTP POST method
	HTTPMethodPost = "POST"

	// HTTPMethodGet is the HTTP GET method
	HTTPMethodGet = "GET"
)

// Constants for HTTP status codes
const (
	// HTTPStatusOK is the HTTP 200 OK status code
	HTTPStatusOK = 200

	// HTTPStatusBadRequest is the HTTP 400 Bad Request status code
	HTTPStatusBadRequest = 400

	// HTTPStatusNotFound is the HTTP 404 Not Found status code
	HTTPStatusNotFound = 404

	// HTTPStatusInternalServerError is the HTTP 500 Internal Server Error status code
	HTTPStatusInternalServerError = 500

	// HTTPStatusServiceUnavailable is the HTTP 503 Service Unavailable status code
	HTTPStatusServiceUnavailable = 503
)

// Constants for server and coin management - internal implementation details only
const (
	// MaxHashStorageLength is the maximum number of block hashes to store per coin per server
	MaxHashStorageLength = 4

	// PingSuccessValue is the value indicating a successful ping response
	PingSuccessValue = 1

	// PingFailureValue is the value indicating a failed ping response
	PingFailureValue = 0

	// InvalidHeightValue is the value indicating an invalid or missing block height
	InvalidHeightValue = -1

	// InvalidTimeDiffValue is the value indicating an invalid time difference
	InvalidTimeDiffValue = -1000
)

// Constants for JSON response templates
const (
	// JSONResponseServiceUnavailable is the service unavailable JSON response
	JSONResponseServiceUnavailable = `{"result": null, "error": "Service unavailable"}`

	// JSONResponsePingSuccess is the successful ping response
	JSONResponsePingSuccess = "1"

	// JSONResponseDefault is the default JSON response template
	JSONResponseDefault = `{"result": null, "error": null}`

	// JSONResponseEmpty is the empty JSON response template
	JSONResponseEmpty = `{}`

	// JSONResponseInternalServerError is the internal server error response
	JSONResponseInternalServerError = `{"error": "Internal Server Error"}`

	// JSONResponseServerErrorTemplate is the template for server error responses
	JSONResponseServerErrorTemplate = `{"result": null, "error": "%s"}`

	// JSONResponseServerErrorWithDetailsTemplate is the template for server error responses with details
	JSONResponseServerErrorWithDetailsTemplate = `{"result": null, "error": "%s", "details": "%s"}`
)

// Constants for error messages
const (
	// ErrorMessageMissingCoinParam is the error message for missing coin parameter
	ErrorMessageMissingCoinParam = "missing coin parameter"

	// ErrorMessageInvalidCoinType is the error message for invalid coin parameter type
	ErrorMessageInvalidCoinType = "invalid coin type"

	// ErrorMessageFailedExtractRequestData is the error message for failed request data extraction
	ErrorMessageFailedExtractRequestData = "Failed to extract request data"

	// ErrorMessageFailedExtractCoin is the error message for failed coin extraction
	ErrorMessageFailedExtractCoin = "Failed to extract coin parameter"

	// ErrorMessageInvalidPath is the error message for invalid request path
	ErrorMessageInvalidPath = "invalid path"

	// ErrorMessageInvalidMethod is the error message for invalid request method
	ErrorMessageInvalidMethod = "invalid method"

	// ErrorMessageNoValidServerForCoin is the error message when no valid server is available for a coin
	ErrorMessageNoValidServerForCoin = "No valid server for "

	// ErrorMessageNoValidServerAvailable is the error message when no valid server is available
	ErrorMessageNoValidServerAvailable = "No valid server available"

	// ErrorMessageAllRetriesExhausted is the error message when all retry attempts are exhausted
	ErrorMessageAllRetriesExhausted = "All retries exhausted. Unable to process the request."

	// ErrorMessageFailedUpdateRequestHeaders is the error message for failed request header updates
	ErrorMessageFailedUpdateRequestHeaders = "Failed to update request headers"

	// ErrorMessageFailedTransformEXRSyntax is the error message for failed EXR syntax transformation
	ErrorMessageFailedTransformEXRSyntax = "failed to transform request to EXR syntax"

	// ErrorMessageFailedParseJSON is the error message for failed JSON parsing
	ErrorMessageFailedParseJSON = "invalid JSON format"

	// ErrorMessageUnexpectedServerResponse is the error message for unexpected server response
	ErrorMessageUnexpectedServerResponse = "*error unexpected server response status: %s"

	// ErrorMessageServerNotFound is the error message when a server is not found by ID
	ErrorMessageServerNotFound = "server not found"

	// ErrorMessageServerIDNotFound is the error message when a server ID is not found
	ErrorMessageServerIDNotFound = "server ID %d not found"

	// ErrorMessageCoinNotFound is the error message when a coin is not found
	ErrorMessageCoinNotFound = "Coin '%s' not found"

	// ErrorMessageServerIDsArrayNotFound is the error message when server IDs array is not found for a coin
	ErrorMessageServerIDsArrayNotFound = "Server IDs array not found for coin '%s'"

	// ErrorMessageNoServerForCoin is the error message when no servers are available for a coin
	ErrorMessageNoServerForCoin = "No server for %s: %d"

	// ErrorMessageInternalServerError is the internal server error message
	ErrorMessageInternalServerError = "Internal Server Error"

	// ErrorMessageServerError is the generic server error message
	ErrorMessageServerError = "server error"
)

// Sentinel errors for GetRandomValidServerID — used with errors.Is() to classify permanent errors.
var (
	// ErrCoinNotFound indicates no mapping exists for the requested coin in the consensus map.
	ErrCoinNotFound = errors.New("coin not found in consensus")
	// ErrServerIDsArrayNotFound indicates the consensus entry exists but lacks a valid server IDs array.
	ErrServerIDsArrayNotFound = errors.New("server IDs array missing from consensus entry")
	// ErrNoServerForCoin indicates the consensus entry exists but has zero servers.
	ErrNoServerForCoin = errors.New("no servers available for coin")
)

// Additional error message, constant, and configuration constants
const (
	// JSONNullString is the string representation of null in JSON
	JSONNullString = "null"

	// JSONNullValue is the null value for request data fallback
	JSONNullValue = "null"

	// Server error message constants
	ErrorMessagePingFailed       = "ping request failed"
	ErrorMessageJSONParseFailed  = "JSON parse failed"
	ErrorMessageMissingResult    = "response missing 'result' field"
	ErrorMessageBlockHashFailed  = "getblockhash failed"
	ErrorMessageGetBlockFailed   = "getblock failed"
	ErrorMessageGetFeesFailed    = "getfees failed"
	ErrorMessageGetHeightsFailed = "getheights failed"

	// Generic error message constants for client responses
	ErrorMessageServiceUnavailable = "Service temporarily unavailable"
	ErrorMessageInvalidRequest     = "Invalid request"
	ErrorMessageRateLimitExceeded  = "Rate limit exceeded"
	ErrorMessageNotFound           = "Not found"
	ErrorMessageBadRequest         = "Bad request"

	// FilePermissionRWXRWXR is the file permission 0644 (owner: read/write/execute, group: read/write, other: read/write)
	FilePermissionRWXRWXR = 0644

	// DefaultPort is the default port for the reverse proxy server
	DefaultPort = 11111
)

// Constants for log message prefixes and formats
const (
	// LogPrefixRevProxy is the log prefix for reverse proxy operations
	LogPrefixRevProxy = "[revProxy_Serv]"

	// LogPrefixServer is the log prefix for server operations
	LogPrefixServer = "[server%2d]"

	// LogPrefixServerError is the log prefix for server errors
	LogPrefixServerError = "[server%2d]_error"

	// LogPrefixServerHeights is the log prefix for server heights operations
	LogPrefixServerHeights = "[server%2d]_Heights"

	// LogPrefixServerUpdate is the log prefix for server update operations
	LogPrefixServerUpdate = "|SERVERS_UPDATE|"

	// LogPrefixServers is the log prefix for servers operations
	LogPrefixServers = "|SERVERS|"

	// LogPrefixError is the general error log prefix
	LogPrefixError = "*error"

	// LogPrefixTestUnit is the test unit log prefix
	LogPrefixTestUnit = "TEST_UNIT:"

	// LogExecTimerFormat is the format for execution timing logs
	LogExecTimerFormat = "exec_timer:%s"
)

// Constants for EXR (Exchange Router) functionality
const (
	// EXRPathPrefix is the EXR path prefix
	EXRPathPrefix = "/xrs/"
)

// Constants for JSON field names
const (
	// JSONFieldResult is the result field name in JSON responses
	JSONFieldResult = "result"

	// JSONFieldError is the error field name in JSON responses
	JSONFieldError = "error"

	// JSONFieldCode is the code field name in error responses
	JSONFieldCode = "code"

	// JSONFieldDetails is the details field name in error responses
	JSONFieldDetails = "details"

	// JSONFieldTime is the time field name in block data
	JSONFieldTime = "time"

	// JSONFieldIds is the ids field name in server ID arrays
	JSONFieldIds = "ids"
)

// Constants for compression
const (
	// CompressionThresholdMin is the minimum value for compression algorithms
	CompressionThresholdMin = 32

	// CompressionThresholdMax is the maximum value for compression algorithms
	CompressionThresholdMax = 126

	// CompressionEscapeChar is the escape character value
	CompressionEscapeChar = '\\'

	// CompressionQuoteChar is the quote character value
	CompressionQuoteChar = '"'

	// CompressionNullChar is the null character value
	CompressionNullChar = '\x00'
)

// Constants for HTTP client configuration - internal implementation details only
const (
	// HTTPMaxIdleConns is the maximum number of idle connections for HTTP client
	HTTPMaxIdleConns = 100

	// HTTPMaxIdleConnsPerHost is the maximum number of idle connections per host
	HTTPMaxIdleConnsPerHost = 10

	// HTTPMaxConnsPerHost is the maximum number of connections per host
	HTTPMaxConnsPerHost = 100
)
