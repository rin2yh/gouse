package graceful_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/YuukiHayashi0510/gouse/graceful"
)

const (
	testShutdownTimeout = 5 * time.Second
	testStartTimeout    = 2 * time.Second
)

var errServerStartTimeout = errors.New("server failed to start within timeout")

func waitForServer(addr string, timeout time.Duration) error {
	client := &http.Client{Timeout: 100 * time.Millisecond}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://" + addr + "/")
		if err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errServerStartTimeout
}

// startRun launches Run in a goroutine and waits for HTTP readiness.
// cancel is registered with t.Cleanup to prevent goroutine leaks on failure.
func startRun(t *testing.T, handler http.Handler, cfg *graceful.Config) (addr string, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	srv, addr := newTestServer(t, handler)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch := make(chan error, 1)
	go func() { ch <- graceful.Run(ctx, srv, cfg) }()
	if err := waitForServer(addr, testStartTimeout); err != nil {
		t.Fatal("server did not start in time:", err)
	}
	return addr, cancel, ch
}

func awaitShutdown(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(testShutdownTimeout):
		t.Fatal("server did not shut down in time")
		return nil
	}
}
