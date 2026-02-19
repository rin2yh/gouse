# net/graceful

HTTP server graceful shutdown.

Responds to `SIGINT` / `SIGTERM` and programmatic context cancellation, shutting the server down safely within a configurable timeout.

## Install

```sh
go get github.com/rin2yh/gouse/net/graceful
```

## Usage

```go
import "github.com/rin2yh/gouse/net/graceful"

// Simple usage
srv := &http.Server{Addr: ":8080", Handler: mux}
if err := graceful.Run(ctx, srv, nil); err != nil {
    log.Fatal(err)
}

// With custom shutdown timeout and cleanup functions
if err := graceful.Run(ctx, srv, &graceful.Config{
    ShutdownTimeout: 10 * time.Second,
    Cleanups:        []func(){db.Close},
}); err != nil {
    log.Fatal(err)
}
```

## Config

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `ShutdownTimeout` | `time.Duration` | `5s` | Maximum time to wait for in-flight requests to complete |
| `Cleanups` | `[]func()` | none | Functions called in order after the server shuts down |

## Benchmarks

Measured with `go test -bench=. -benchmem -count=3` on the following environment (library minimum supported version is Go 1.21, per `go.mod`):

- Go 1.24.7 linux/amd64 (benchmark runtime; library supports Go 1.21+)
- CPU: Intel Xeon Platinum 8581C @ 2.10GHz

| Benchmark | ns/op | B/op | allocs/op |
|-----------|------:|-----:|----------:|
| `BenchmarkShutdown/cleanups=0` | 227,000 | 1,256 | 26 |
| `BenchmarkShutdown/cleanups=1` | 239,000 | 1,256 | 26 |
| `BenchmarkShutdown/cleanups=5` | 229,000 | 1,256 | 26 |
| `BenchmarkShutdown/cleanups=10` | 223,000 | 1,256 | 26 |

Each iteration covers a full shutdown cycle: context cancellation → `Shutdown()` → cleanup execution.
Memory footprint is constant regardless of the number of cleanup functions, since cleanup functions themselves are not allocated by this package.

To run on your own machine:

```sh
go test -bench=. -benchmem ./net/graceful/
```
