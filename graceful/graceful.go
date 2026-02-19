package graceful

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Config holds configuration for Run.
type Config struct {
	ShutdownTimeout time.Duration
	Cleanups        []func()
}

// Run starts the HTTP server and blocks until the context is cancelled
// or the server encounters a fatal error. On cancellation, it performs a
// graceful shutdown within Config.ShutdownTimeout, then runs Config.Cleanups
// in order (e.g. closing DB connections).
func Run(ctx context.Context, srv *http.Server, cfg Config) error {
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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	for _, cleanup := range cfg.Cleanups {
		cleanup()
	}

	return nil
}
