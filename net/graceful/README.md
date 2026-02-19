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
