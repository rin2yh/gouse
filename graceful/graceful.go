// Package graceful provides HTTP server startup and graceful shutdown.
//
// Signal-based shutdown (typical production usage):
//
//	ctx, stop := graceful.NewContext(context.Background())
//	defer stop()
//
//	srv := &http.Server{Addr: ":8080", Handler: mux}
//	if err := graceful.Run(ctx, srv, nil); err != nil {
//	    log.Fatal(err)
//	}
//
// With custom timeout and cleanup (e.g. closing DB after shutdown):
//
//	ctx, stop := graceful.NewContext(context.Background())
//	defer stop()
//
//	graceful.Run(ctx, srv, &graceful.Config{
//	    ShutdownTimeout: 10 * time.Second,
//	    Cleanups:        []func(){db.Close},
//	})
package graceful

import (
	"context"
	"errors"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

const defaultShutdownTimeout = 5 * time.Second

// Server is the interface required by Run.
// *http.Server satisfies this interface.
//
// ListenAndServe must return http.ErrServerClosed when Shutdown is called;
// this is the standard behaviour of *http.Server.
type Server interface {
	ListenAndServe() error
	Shutdown(ctx context.Context) error
}

// Config holds optional configuration for Run.
// The zero value is valid; ShutdownTimeout defaults to 5 seconds.
type Config struct {
	// ShutdownTimeout is the maximum duration to wait for in-flight requests
	// to complete before forcibly closing connections.
	// Defaults to 5 seconds if zero.
	ShutdownTimeout time.Duration

	// Cleanups are functions called in order after the server shuts down
	// (e.g. closing database connections, flushing caches).
	Cleanups []func()
}

// NewContext returns a context that is cancelled when SIGINT or SIGTERM is received.
// The returned stop function must be called to release resources.
//
// Typical usage:
//
//	ctx, stop := graceful.NewContext(context.Background())
//	defer stop()
//	graceful.Run(ctx, srv, nil)
func NewContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
}

// Run starts srv and blocks until ctx is cancelled or the server fails.
// On cancellation, it shuts down gracefully within the configured timeout,
// then runs each cleanup function in order.
//
// If cfg is nil, a 5-second shutdown timeout is used with no cleanups.
func Run(ctx context.Context, srv Server, cfg *Config) error {
	timeout := defaultShutdownTimeout
	var cleanups []func()
	if cfg != nil {
		if cfg.ShutdownTimeout > 0 {
			timeout = cfg.ShutdownTimeout
		}
		cleanups = cfg.Cleanups
	}

	serverErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	for _, cleanup := range cleanups {
		cleanup()
	}

	return nil
}
