package httpx

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Run starts the HTTP server and blocks until the context is cancelled
// or the server encounters a fatal error. On cancellation, it performs a
// graceful shutdown within the given timeout.
func Run(ctx context.Context, srv *http.Server, shutdownTimeout time.Duration) error {
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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	return srv.Shutdown(shutdownCtx)
}
