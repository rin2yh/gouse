package httpx

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const defaultShutdownTimeout = 5 * time.Second

type config struct {
	shutdownTimeout time.Duration
	signals         []os.Signal
}

// Option is a function that configures the server run behavior.
type Option func(*config)

func defaultConfig() *config {
	return &config{
		shutdownTimeout: defaultShutdownTimeout,
		signals:         []os.Signal{syscall.SIGINT, syscall.SIGTERM},
	}
}

// WithShutdownTimeout sets the timeout for graceful shutdown.
// Default is 5 seconds.
func WithShutdownTimeout(d time.Duration) Option {
	return func(c *config) {
		c.shutdownTimeout = d
	}
}

// WithSignals sets the OS signals that trigger graceful shutdown.
// Default signals are SIGINT and SIGTERM.
func WithSignals(signals ...os.Signal) Option {
	return func(c *config) {
		c.signals = signals
	}
}

// Run starts the HTTP server and blocks until a shutdown signal is received or
// the server encounters a fatal error. On receiving a signal, it performs a
// graceful shutdown within the configured timeout.
//
// Returns nil if the server was shut down gracefully, or an error if the server
// failed to start or shutdown did not complete within the timeout.
func Run(srv *http.Server, opts ...Option) error {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	serverErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, cfg.signals...)
	defer signal.Stop(quit)

	select {
	case err := <-serverErr:
		return err
	case <-quit:
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.shutdownTimeout)
	defer cancel()

	return srv.Shutdown(ctx)
}

// RunWithContext starts the HTTP server and blocks until the context is cancelled
// or the server encounters a fatal error. On cancellation, it performs a graceful
// shutdown within the configured timeout.
//
// This is useful for testing or when shutdown is controlled programmatically.
func RunWithContext(ctx context.Context, srv *http.Server, opts ...Option) error {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.shutdownTimeout)
	defer cancel()

	return srv.Shutdown(shutdownCtx)
}
