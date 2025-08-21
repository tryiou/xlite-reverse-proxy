# xlite-reverse-proxy

A robust reverse proxy daemon designed to serve as a reliable endpoint for `xlite-daemon`  
<https://github.com/blocknetdx/xlite-daemon>

This proxy intelligently relays requests to a dynamic or static list of backend service node servers. It performs continuous health checks, validates data consistency, and applies consensus rules to ensure that `xlite-daemon` receives accurate and reliable responses, even if individual backend servers are unreliable or provide conflicting data.

![Image Description](.github/image.png)

---

## 🧱 Build Instructions

### Clone the Repository

```bash
git clone https://github.com/tryiou/xlite-reverse-proxy.git
cd xlite-reverse-proxy
```

### Install Dependencies

```bash
go get
```

### Build for Your Platform

- **Linux**:

  ```bash
  go build -o build/xlite-reverse-proxy_linux .
  ```

- **Windows**:

  ```bash
  go build -o build/xlite-reverse-proxy_windows.exe .
  ```

- **macOS**:

  ```bash
  go build -o build/xlite-reverse-proxy_macos .
  ```

### Run the Built Binary

- **Linux**:

  ```bash
  ./build/xlite-reverse-proxy_linux
  ```

- **Windows**:

  ```bash
  .\build\xlite-reverse-proxy_windows.exe
  ```

- **macOS**:

  ```bash
  ./build/xlite-reverse-proxy_macos
  ```

---

## 🚀 Usage

The proxy listens on port `11111` by default.

### Quick Start

1.  **Build the application** (see Build Instructions).
2.  **Run with default static configuration**:
    ```bash
    ./build/xlite-reverse-proxy_linux
    ```
    (or `xlite-reverse-proxy_windows.exe`, `xlite-reverse-proxy_macos` depending on your OS)
3.  **Query the proxy** (e.g., for global heights):
    ```bash
    curl -X POST -H "Content-Type: application/json" -d '{"method": "heights"}' http://127.0.0.1:11111/heights
    ```

### Static Server Configuration

If the application is executed without any launch arguments, it will use the `servers_map` configuration as the static list of servers. This is the default behavior.

- `servers_map` defines a list of servers to use as remote endpoints.
- Each server entry must include:
  - `url`: The URL of the server (string).
  - `exr`: Whether the server is an EXR service node or an exposed 'plugin-adapter' endpoint (bool).

### Dynamic Server Configuration

If the application is executed with the `-dynlist=true` launch argument, it will use the `dynlist_servers_providers` URLs to fetch the `servers_map` dynamically. This allows the proxy to update its server list periodically from external providers.

- `dynlist_servers_providers` defines a list of remote URLs to fetch dynamic server lists from.

When `-dynlist=true` is enabled:

1. The proxy fetches the server list from each URL in `dynlist_servers_providers` every 5 minutes.
2. It uses the returned server list to replace the static `servers_map` configuration.

---

## 🛠️ Configuration

The proxy uses a configuration file `xlite-reverse-proxy-config.yaml`. If this file does not exist on first run, a default one will be auto-generated.

The following parameters can be defined in the configuration file:

### `dynlist_servers_providers` (Dynamic Servers List)

- **Purpose**: A list of remote URLs from which to fetch dynamic server lists. This is enabled when running the application with `-dynlist=true`.
- **Format**: List of strings (HTTP/HTTPS URLs).
- **Example**:

  ```yaml
  dynlist_servers_providers:
    - https://utils.blocknet.org
    - http://exrproxy1.airdns.org:42114
  ```

### `servers_map` (Static Servers List)

- **Purpose**: Defines a static list of backend servers to use as remote endpoints. This is the default behavior if dynamic listing is not enabled.
- **Format**: List of maps with keys:
  - `url`: (string) The base URL of the server (e.g., `http://127.0.0.1:42114`).
  - `exr`: (bool) Set to `true` if the server is an EXR service node (requiring `/xrs/method` path transformation), `false` otherwise (for direct 'plugin-adapter' endpoints).
- **Example**:

  ```yaml
  servers_map:
    - url: http://exrproxy1.airdns.org:42114
      exr: true
    - url: http://127.0.0.1:5000/
      exr: false
  ```

### `accepted_paths` (Accepted API Paths)

- **Purpose**: A whitelist of API paths that the proxy will accept and relay to backend servers or handle internally. Requests to paths not in this list will be rejected.
- **Format**: List of strings.
- **Example**:

  ```yaml
  accepted_paths:
    - /
    - /height
    - /heights
    - /fees
    - /ping
    - /servers
  ```

### `accepted_methods` (Accepted RPC Methods)

- **Purpose**: A whitelist of RPC methods that the proxy will relay to backend servers. Requests for methods not in this list will be rejected.
- **Format**: List of strings.
- **Example**:

  ```yaml
  accepted_methods:
    - getutxos
    - getrawtransaction
    - getrawmempool
    - getblockcount
    - sendrawtransaction
    - gettransaction
    - getblock
    - getblockhash
    - heights
    - fees
    - getbalance
    - gethistory
    - ping
  ```

### `max_stored_blocks` (Block Cache Size)

- **Purpose**: The maximum number of recent blocks to cache per coin for validation purposes. This helps in quickly verifying block data without re-fetching.
- **Format**: Integer.
- **Example**: `3` (caches up to 3 blocks per coin).

### `max_block_time_diff` (Time Difference Threshold)

- **Purpose**: The maximum allowed difference (in seconds) between a block's timestamp and the proxy's system time for a server to be considered healthy and its data reliable.
- **Format**: Integer.
- **Example**: `7200` (2 hours).

### `http_timeout` (HTTP Request Timeout)

- **Purpose**: The timeout (in seconds) for HTTP requests made by the proxy to backend servers.
- **Format**: Integer.
- **Example**: `8` (8 seconds).

### `rate_limit` (Request Rate Limit)

- **Purpose**: The maximum number of incoming requests allowed per minute to prevent abuse and ensure stability.
- **Format**: Integer.
- **Example**: `100` (100 requests/minute).

### `max_log_size` (Log File Size Limit)

- **Purpose**: The maximum size (in bytes) for the application's log file before it is rotated (a new log file is started).
- **Format**: Integer.
- **Example**: `52428800` (50 MB).

### `consensus_threshold` (Consensus Agreement Threshold)

- **Purpose**: The minimum ratio (as a float between 0.0 and 1.0) of healthy backend servers that must agree on a specific data point (e.g., block height, transaction fee, block hash) for it to be considered a "consensus" value.
- **Format**: Float.
- **Example**: `0.66` (approximately 2/3 or 66% consensus).
