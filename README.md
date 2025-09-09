# httpdumper

A tiny Go HTTP server that accepts any HTTP request, prints a detailed dump to stdout, and (optionally) echoes the request headers and body back to the caller.

- Default listen port: 8080 (configurable via `-port`)
- Optional echo mode: `-echo` mirrors request headers (excluding hop-by-hop) and echoes the request body
- Request body capture is capped at 10 MiB for both dump and echo
- Simple HTML UI shows the most recent requests kept in memory (configurable). Serve it on a separate port via `-ui-port` or on the same port when `-ui-port=0` (default). The UI also provides a per-request “Copy curl” button to reproduce the request.

## Features

- Logs every request to stdout including:
  - Method, URL, protocol
  - RemoteAddr and parsed RemoteIP
  - Host and derived scheme (honors TLS and common proxy headers)
  - Path and raw query
  - All headers (sorted), cookies, and trailers
  - Body bytes (up to 10 MiB) with truncation notice
  - TLS details if present
- Echo mode (`-echo`) mirrors:
  - Response headers copied from request headers except hop‑by‑hop headers (Connection, Transfer‑Encoding, Keep‑Alive, Upgrade, TE, Trailer, Content‑Length, Host, Proxy‑Connection)
  - Content‑Type preserved if provided by the client; otherwise defaults to `application/octet-stream`
  - Response body equals the captured request body bytes (subject to 10 MiB cap). If truncated, an `X-Echo-Note: body truncated by server cap` header is added.

## Quick start

### Prerequisites
- Go 1.21+ (or a recent Go toolchain)

### Build and run (local)
```bash
# From the repository root
go build -o httpdumper
./httpdumper -port 8080            # start without echo
# or
./httpdumper -port 8080 -echo      # start with echo mode enabled
```

### Using Makefile
```bash
make build      # builds binary
make run        # runs with defaults (adjust Makefile if needed)
```

### Docker
A simple Dockerfile is provided.

```bash
docker build -t httpdumper:latest .
# without echo
docker run --rm -p 8080:8080 httpdumper:latest -port 8080
# with echo
docker run --rm -p 8080:8080 httpdumper:latest -port 8080 -echo
```

## Usage examples

### Basic GET
```bash
curl -v http://localhost:8080/hello?x=1
```
- Server logs a detailed request dump to stdout.
- Response body is `OK\n` unless `-echo` is enabled.
- UI: by default (no `-ui-port`), visit http://localhost:8080/ui and http://localhost:8080/requests.json.
- With `-ui-port`, the UI is served on that port instead. Example: `-port 8080 -ui-port 9090` -> http://localhost:9090/ui

### Echo JSON body
```bash
curl -v -H 'Content-Type: application/json' \
     -H 'X-Custom: demo' \
     --data '{"msg":"hi"}' \
     http://localhost:8080/echo
```
- With `-echo`, response headers include `Content-Type: application/json` and `X-Custom: demo`.
- Response body is exactly `{"msg":"hi"}`.

### Large body behavior
Bodies larger than 10 MiB are truncated to 10 MiB for both logging and echoing.
When truncated in echo mode, the response contains header:

```
X-Echo-Note: body truncated by server cap
```

## Flags
- `-port int` (default `8080`): Port to listen on for request handling.
- `-ui-port int`: Port to serve the UI (`/ui`, `/requests.json`). Set this flag to enable the UI.
- `-echo` (default `false`): When set, mirrors request headers (excluding hop‑by‑hop) and echoes the request body.
- `-history int` (default `1000`): Number of recent requests to keep in memory for the HTML UI and /requests.json.
- `-shutdown-timeout duration` (default `10s`): Timeout for graceful shutdown when terminating (e.g., 5s, 1m).

## Notes and caveats
- Hop‑by‑hop headers are intentionally not mirrored to comply with HTTP semantics.
- `Content-Length` is not copied from the request; it is automatically set by Go’s HTTP server for the response.
- The derived scheme respects TLS and common proxy headers like `X-Forwarded-Proto` and `Forwarded` (basic parsing).
- Request body read limit is 10 MiB. Increase by editing `maxBody` in `main.go` if needed.
- For binary bodies, ensure your client sets an appropriate `Content-Type` or the response will default to `application/octet-stream` in echo mode.

## Development

```bash
go build ./...
go test ./...    # no tests yet, placeholder
```
