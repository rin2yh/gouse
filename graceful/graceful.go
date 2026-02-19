// Package graceful provides HTTP server startup and graceful shutdown.
//
// Run responds to both SIGINT/SIGTERM and programmatic context cancellation.
//
// Signal-based shutdown (typical production use):
//
//	ctx := context.Background()
//	srv := &http.Server{Addr: ":8080", Handler: mux}
//	if err := graceful.Run(ctx, srv, nil); err != nil {
//	    log.Fatal(err)
//	}
//
// Programmatic cancellation (e.g. managed lifecycle or tests):
//
//	ctx, cancel := context.WithCancel(context.Background())
//	go func() {
//	    if err := graceful.Run(ctx, srv, nil); err != nil {
//	        log.Print(err)
//	    }
//	}()
//	// ... when ready to stop:
//	cancel()
//
// With custom shutdown timeout and post-shutdown cleanups:
//
//	if err := graceful.Run(ctx, srv, &graceful.Config{
//	    ShutdownTimeout: 10 * time.Second,
//	    Cleanups:        []func(){db.Close},
//	}); err != nil {
//	    log.Fatal(err)
//	}
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
// ListenAndServe should return http.ErrServerClosed when Shutdown is called;
// any other non-nil return value is treated as a startup failure by Run.
type Server interface {
	ListenAndServe() error
	Shutdown(ctx context.Context) error
}

// Config holds optional configuration for Run. The zero value is valid.
type Config struct {
	// ShutdownTimeout is the maximum duration Shutdown waits for in-flight
	// requests to complete. If exceeded, Shutdown returns an error; active
	// connections are not forcibly closed.
	// Defaults to 5 seconds if zero.
	ShutdownTimeout time.Duration

	// Cleanups are functions called in order after the server shuts down
	// (e.g. closing database connections, flushing caches).
	// If a cleanup panics, all remaining cleanups still run before the
	// panic is re-raised.
	Cleanups []func()
}

// Run starts srv and blocks until SIGINT/SIGTERM is received (or parent is
// cancelled), then shuts down gracefully within the configured timeout and
// runs each cleanup function in order.
//
// If cfg is nil, a 5-second shutdown timeout is used with no cleanups.
func Run(parent context.Context, srv Server, cfg *Config) error {
	if cfg == nil {
		cfg = &Config{}
	}

	ctx, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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

	timeout := defaultShutdownTimeout
	if cfg.ShutdownTimeout > 0 {
		timeout = cfg.ShutdownTimeout
	}
	// context.WithoutCancel preserves values (trace IDs, loggers) from ctx
	// while preventing the already-cancelled ctx from short-circuiting shutdown.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()

	shutdownErr := srv.Shutdown(shutdownCtx)

	// Drain serverErr: a real ListenAndServe error may have raced with ctx.Done
	// and been lost when the select chose the ctx.Done branch.
	srvErr := <-serverErr

	cleanup(cfg.Cleanups)

	if srvErr != nil {
		return srvErr
	}
	return shutdownErr
}

// cleanup calls each fn in order. If one panics, the rest still run;
// the first panic value is re-raised after all have completed.
func cleanup(fns []func()) {
	var panicVal any
	for _, fn := range fns {
		func() {
			defer func() {
				if r := recover(); r != nil && panicVal == nil {
					panicVal = r
				}
			}()
			fn()
		}()
	}
	if panicVal != nil {
		panic(panicVal)
	}
}
