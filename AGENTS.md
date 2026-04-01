# AGENTS.md

## Project Overview

`xlite-reverse-proxy` is a Go 1.24 reverse proxy daemon that relays requests from `xlite-daemon` to backend service node servers. It performs health checks, consensus validation, and dynamic/static server list management.

Single `package main` flat layout — all `.go` files live in the repository root. No sub-packages.

## Build / Test / Lint Commands

```bash
# Install dependencies
go get

# Build for Linux
go build -o build/xlite-reverse-proxy_linux .

# Build for all CI targets
GOOS=linux   GOARCH=amd64 go build -o build/xlite-reverse-proxy_linux-amd64 .
GOOS=linux   GOARCH=arm64 go build -o build/xlite-reverse-proxy_linux-arm64 .
GOOS=windows GOARCH=amd64 go build -o build/xlite-reverse-proxy_windows-amd64.exe .
GOOS=windows GOARCH=arm64 go build -o build/xlite-reverse-proxy_windows-arm64.exe .
GOOS=darwin  GOARCH=amd64 go build -o build/xlite-reverse-proxy_macos-amd64 .
GOOS=darwin  GOARCH=arm64 go build -o build/xlite-reverse-proxy_macos-arm64 .

# Run all tests
go test -v ./...

# Run a single test by name
go test -v -run TestServerError

# Run tests in a specific file (pattern matches test function prefix)
go test -v -run TestServerPing ./...

# Race detector (important: code uses sync primitives extensively)
go test -race -v ./...

# Format check (gofmt)
gofmt -d .

# Vet
go vet ./...

# Note: No linter config (golangci-lint, etc.) is present in this repo.
# The CI workflow (.github/workflows/go.yml) runs build only; tests are commented out.
# No .cursorrules, .cursor/rules/, or .github/copilot-instructions.md found.
```

## Code Style Guidelines

### Package & Imports

- Single package: `package main` everywhere.
- Group imports: stdlib first, then third-party (`github.com/...`, `gopkg.in/...`), separated by blank line.
- Use tabs (Go standard `gofmt`).

```go
import (
    "fmt"
    "log"
    "net/http"

    "github.com/valyala/fastjson"
    "gopkg.in/yaml.v2"
)
```

### Naming Conventions

- **Exported types/funcs**: PascalCase (`Server`, `Config`, `NewApplication`, `ValidateConfig`).
- **Unexported fields/funcs**: camelCase (`coinsMap`, `hashesStorage`, `server_GetPing`).
- **Constants**: PascalCase grouped in `const` blocks with doc comments (`PingSuccessValue`, `HTTPStatusOK`, `ErrorMessageMissingCoinParam`).
- **Error variables/types**: PascalCase with `Error` suffix (`ServerError`, `ValidationError`, `HTTPError`).
- **Test helpers**: Prefix with `setup` or descriptive name (`setupGlobalConfigForTest`, `setupTestConfigForConfigManager`).

### Types & Structs

- Define structs in `structures.go`.
- JSON/YAML tags use lowercase snake_case: `` `yaml:"servers_map"` ``, `` `json:"method"` ``.
- Custom error types implement `error` interface + `Unwrap() error` when wrapping.
- Use `sync.RWMutex` for concurrent access (see `LockManager` in `structures.go`).
- Use `sync.Pool` for object reuse (`ObjectPool` in `structures.go`).

### Error Handling

- Return `error` as the last return value.
- Wrap errors with `fmt.Errorf("context: %w", err)` for stack traces.
- Use custom error types (`ServerError`, `ValidationError`, `HTTPError`) with constructor functions (`NewServerError`, `NewValidationError`, `NewHTTPError`).
- Use helper functions for type checking: `IsServerError()`, `IsValidationError()`, `IsHTTPError()`.
- Log errors via `log.Printf` or custom `logServerError`/`logServerSuccess` helpers.
- For fatal startup errors, use `log.Fatalf`.

### Constants

- All constants defined in `constants.go` — do not define inline magic values.
- Group by domain: timeouts, HTTP, server management, JSON templates, error messages, log prefixes.
- Every constant must have a doc comment.

### Concurrency

- Use `sync.WaitGroup` for goroutine coordination.
- Use `sync.RWMutex` for read-heavy shared state.
- Guard goroutines with `defer func() { if r := recover(); r != nil { ... } }()` panics.
- Use channels for error propagation from goroutines.

### Testing

- Tests use the standard `testing` package — no external test framework.
- `github.com/stretchr/testify` is available but only used in some tests.
- Mock HTTP with custom `mockTransport` implementing `http.RoundTripper`.
- Tests in the same `package main` (not `_test` package) for access to unexported types.
- Test function naming: `Test<Unit><Scenario>` (e.g., `TestServerPingErrorLogging`, `TestConcurrentConfigAccess`).
- Use `t.Errorf` for non-fatal, `t.Error`/`t.Fatal` for fatal assertions.
- Set up global state explicitly in test helpers; restore with `defer`.

### Configuration

- YAML config file: `xlite-reverse-proxy-config.yaml`.
- Auto-generated with defaults if missing.
- Config struct in `config.go`; access via `globalConfig.GetConfig()` (thread-safe).
- Validate all config values in `ValidateConfig`; use the `Validator` pattern to collect multiple errors.

### Validation Pattern

- `Validator` struct in `validation.go` collects errors during validation.
- Call `v.AddError(field, message, value)` for each issue; check `v.HasErrors()` after.
- Use for request validation (`ValidateRequestData`), config validation (`ValidateConfig`), and HTTP request validation.
- Sentinel errors (`ErrCoinNotFound`, `ErrServerIDsArrayNotFound`, `ErrNoServerForCoin`) in `constants.go` for `errors.Is()` checks.

### Logging

- Custom logger initialized in `logs.go`, called via `init()`.
- Use structured log prefixes from constants (`LogPrefixServer`, `LogPrefixError`, etc.).
- Log format: `[serverXX] operation description` or `[serverXX]_error operation failed: reason`.

### General Patterns

- Flat file structure — keep related logic in named files (`server.go`, `config.go`, `errors.go`, etc.).
- Constructor pattern: `NewXxx()` functions (`NewApplication`, `NewOptimizedBlockCache`).
- Global state via package-level vars (`globalConfig`, `locks`, `globalPool`, `logger`).
- Receiver methods on structs for behavior (`server.server_GetPing()`, `servers.UpdateAllServersData()`).
- No comments on obvious code; doc comments on exported types and non-trivial functions only.
