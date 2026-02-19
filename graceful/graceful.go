// Package graceful provides HTTP server startup and graceful shutdown.
//
// Typical usage:
//
//	srv := &http.Server{Addr: ":8080", Handler: mux}
//	if err := graceful.Run(context.Background(), srv, nil); err != nil {
//	    log.Fatal(err)
//	}
//
// With custom timeout and cleanup (e.g. closing DB after shutdown):
//
//	graceful.Run(context.Background(), srv, &graceful.Config{
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

// Run starts srv and blocks until SIGINT/SIGTERM is received (or parent is
// cancelled), then shuts down gracefully within the configured timeout and
// runs each cleanup function in order.
//
// If cfg is nil, a 5-second shutdown timeout is used with no cleanups.
func Run(parent context.Context, srv Server, cfg *Config) error {
	ctx, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if cfg == nil {
		cfg = &Config{}
	}
	timeout := defaultShutdownTimeout
	if cfg.ShutdownTimeout > 0 {
		timeout = cfg.ShutdownTimeout
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

	// context.WithoutCancel preserves values (trace IDs, loggers) from ctx
	// while preventing the already-cancelled ctx from short-circuiting shutdown.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()

	shutdownErr := srv.Shutdown(shutdownCtx)

	// Drain serverErr: a real ListenAndServe error may have raced with ctx.Done
	// and been lost when the select chose the ctx.Done branch.
	srvErr := <-serverErr

	for _, cleanup := range cfg.Cleanups {
		cleanup()
	}

	if srvErr != nil {
		return srvErr
	}
	return shutdownErr
}
