package graceful_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/YuukiHayashi0510/gouse/graceful"
)

const (
	testShutdownTimeout = 5 * time.Second
	testStartTimeout    = 2 * time.Second
)

// errServerStartTimeout is returned by waitForServer when the server does not
// respond within the given timeout. Using a dedicated error avoids the
// misleading implication that a context deadline was exceeded.
var errServerStartTimeout = errors.New("server failed to start within timeout")

// listenerServer wraps *http.Server to use a pre-bound listener,
// eliminating the TOCTOU race where another process grabs the port between
// freePort returning and ListenAndServe binding.
type listenerServer struct {
	srv *http.Server
	ln  net.Listener
}

func (s *listenerServer) ListenAndServe() error              { return s.srv.Serve(s.ln) }
func (s *listenerServer) Shutdown(ctx context.Context) error { return s.srv.Shutdown(ctx) }

// newTestServer binds a listener on an OS-assigned port and returns a Server
// that calls Serve(ln), plus the bound address.
// The listener is closed via t.Cleanup when the test ends.
func newTestServer(t *testing.T, handler http.Handler) (graceful.Server, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return &listenerServer{
		srv: &http.Server{Handler: handler},
		ln:  ln,
	}, ln.Addr().String()
}

// controllableServer lets tests inject custom ListenAndServe and Shutdown
// behaviour without relying on OS-level port allocation.
type controllableServer struct {
	listenFunc   func() error
	shutdownFunc func(context.Context) error
}

func (s *controllableServer) ListenAndServe() error { return s.listenFunc() }
func (s *controllableServer) Shutdown(ctx context.Context) error {
	if s.shutdownFunc != nil {
		return s.shutdownFunc(ctx)
	}
	return nil
}

// waitForServer polls until the server responds to an HTTP request,
// confirming the HTTP layer is ready (not just TCP).
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

func TestRun(t *testing.T) {
	tests := []struct {
		name string
		cfg  *graceful.Config
	}{
		{
			name: "with config",
			cfg:  &graceful.Config{ShutdownTimeout: testShutdownTimeout},
		},
		{
			name: "nil config uses default",
			cfg:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, addr := newTestServer(t, http.DefaultServeMux)
			ctx, cancel := context.WithCancel(context.Background())

			done := make(chan error, 1)
			go func() {
				done <- graceful.Run(ctx, srv, tt.cfg)
			}()

			if err := waitForServer(addr, testStartTimeout); err != nil {
				t.Fatal("server did not start in time:", err)
			}

			cancel()

			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("expected nil error, got: %v", err)
				}
			case <-time.After(testShutdownTimeout):
				t.Fatal("server did not shut down in time")
			}
		})
	}
}

// TestRunServerError verifies that Run propagates an error returned by
// ListenAndServe (e.g. address already in use). controllableServer is used
// instead of *http.Server so the test is independent of OS port availability.
func TestRunServerError(t *testing.T) {
	want := errors.New("listen tcp: bind: address already in use")
	srv := &controllableServer{
		listenFunc: func() error { return want },
	}

	got := graceful.Run(context.Background(), srv, nil)
	if !errors.Is(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestRunCleanup(t *testing.T) {
	srv, addr := newTestServer(t, http.DefaultServeMux)
	ctx, cancel := context.WithCancel(context.Background())

	var called []string
	done := make(chan error, 1)
	go func() {
		done <- graceful.Run(ctx, srv, &graceful.Config{
			ShutdownTimeout: testShutdownTimeout,
			Cleanups: []func(){
				func() { called = append(called, "first") },
				func() { called = append(called, "second") },
			},
		})
	}()

	if err := waitForServer(addr, testStartTimeout); err != nil {
		t.Fatal("server did not start in time:", err)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
	case <-time.After(testShutdownTimeout):
		t.Fatal("server did not shut down in time")
	}

	if len(called) != 2 || called[0] != "first" || called[1] != "second" {
		t.Fatalf("expected cleanups to run in order, got: %v", called)
	}
}

// TestRunCleanupPanic verifies that when one cleanup panics, subsequent
// cleanups still run before the panic is re-raised.
func TestRunCleanupPanic(t *testing.T) {
	srv, addr := newTestServer(t, http.DefaultServeMux)
	ctx, cancel := context.WithCancel(context.Background())

	secondRan := false
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		_ = graceful.Run(ctx, srv, &graceful.Config{
			ShutdownTimeout: testShutdownTimeout,
			Cleanups: []func(){
				func() { panic("cleanup panic") },
				func() { secondRan = true },
			},
		})
	}()

	if err := waitForServer(addr, testStartTimeout); err != nil {
		t.Fatal("server did not start in time:", err)
	}

	cancel()

	select {
	case val := <-done:
		if val == nil {
			t.Fatal("expected Run to panic from cleanup, but it returned normally")
		}
		if val != "cleanup panic" {
			t.Fatalf("expected panic value %q, got %v", "cleanup panic", val)
		}
	case <-time.After(testShutdownTimeout):
		t.Fatal("Run did not return in time")
	}

	if !secondRan {
		t.Fatal("second cleanup did not run after first cleanup panicked")
	}
}

func TestRunHandlesRequests(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv, addr := newTestServer(t, mux)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- graceful.Run(ctx, srv, nil)
	}()

	if err := waitForServer(addr, testStartTimeout); err != nil {
		t.Fatal("server did not start in time:", err)
	}

	resp, err := http.Get("http://" + addr + "/ping")
	if err != nil {
		t.Fatal("request failed:", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error on shutdown, got: %v", err)
		}
	case <-time.After(testShutdownTimeout):
		t.Fatal("server did not shut down in time")
	}
}

func TestRunShutdownError(t *testing.T) {
	mux := http.NewServeMux()

	// Use a very short shutdown timeout so that an in-flight request
	// will cause http.Server.Shutdown to time out and return an error.
	shortShutdownTimeout := 50 * time.Millisecond

	mux.HandleFunc("/hang", func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than the shutdown timeout to force a timeout error.
		time.Sleep(2 * shortShutdownTimeout)
		w.WriteHeader(http.StatusOK)
	})

	srv, addr := newTestServer(t, mux)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- graceful.Run(ctx, srv, &graceful.Config{
			ShutdownTimeout: shortShutdownTimeout,
		})
	}()

	if err := waitForServer(addr, testStartTimeout); err != nil {
		t.Fatal("server did not start in time:", err)
	}

	// Start a hanging request that will still be running when shutdown begins.
	go func() {
		resp, err := http.Get("http://" + addr + "/hang")
		if err == nil && resp != nil {
			resp.Body.Close()
		}
	}()

	// Give the request a moment to reach the handler.
	time.Sleep(10 * time.Millisecond)

	// Trigger shutdown.
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected non-nil error when shutdown fails, got nil")
		}
	case <-time.After(testShutdownTimeout):
		t.Fatal("server did not shut down in time")
	}
}
