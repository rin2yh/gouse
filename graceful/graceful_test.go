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

var errServerStartTimeout = errors.New("server failed to start within timeout")

// listenerServer wraps *http.Server with a pre-bound listener to avoid the
// TOCTOU race between acquiring a free port and calling ListenAndServe.
type listenerServer struct {
	srv *http.Server
	ln  net.Listener
}

func (s *listenerServer) ListenAndServe() error              { return s.srv.Serve(s.ln) }
func (s *listenerServer) Shutdown(ctx context.Context) error { return s.srv.Shutdown(ctx) }

// controllableServer injects arbitrary ListenAndServe / Shutdown behaviour.
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

func newTestServer(t *testing.T, handler http.Handler) (graceful.Server, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return &listenerServer{srv: &http.Server{Handler: handler}, ln: ln}, ln.Addr().String()
}

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

func TestRun(t *testing.T) {
	tests := []struct {
		name string
		cfg  *graceful.Config
	}{
		{"with config", &graceful.Config{ShutdownTimeout: testShutdownTimeout}},
		{"nil config uses default", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, cancel, done := startRun(t, http.DefaultServeMux, tt.cfg)
			cancel()
			if err := awaitShutdown(t, done); err != nil {
				t.Fatalf("expected nil error, got: %v", err)
			}
		})
	}
}

func TestRunServerError(t *testing.T) {
	want := errors.New("listen tcp: bind: address already in use")
	srv := &controllableServer{listenFunc: func() error { return want }}

	got := graceful.Run(context.Background(), srv, nil)
	if !errors.Is(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestRunCleanup(t *testing.T) {
	var called []string
	_, cancel, done := startRun(t, http.DefaultServeMux, &graceful.Config{
		ShutdownTimeout: testShutdownTimeout,
		Cleanups: []func(){
			func() { called = append(called, "first") },
			func() { called = append(called, "second") },
		},
	})
	cancel()
	if err := awaitShutdown(t, done); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if len(called) != 2 || called[0] != "first" || called[1] != "second" {
		t.Fatalf("expected cleanups to run in order, got: %v", called)
	}
}

func TestRunCleanupPanic(t *testing.T) {
	srv, addr := newTestServer(t, http.DefaultServeMux)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

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

	addr, cancel, done := startRun(t, mux, nil)

	resp, err := http.Get("http://" + addr + "/ping")
	if err != nil {
		t.Fatal("request failed:", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	cancel()
	if err := awaitShutdown(t, done); err != nil {
		t.Fatalf("expected nil error on shutdown, got: %v", err)
	}
}

func TestRunShutdownError(t *testing.T) {
	const shortTimeout = 50 * time.Millisecond
	mux := http.NewServeMux()
	mux.HandleFunc("/hang", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * shortTimeout)
		w.WriteHeader(http.StatusOK)
	})

	addr, cancel, done := startRun(t, mux, &graceful.Config{ShutdownTimeout: shortTimeout})

	// Launch a request that will still be in-flight when shutdown begins.
	go func() {
		resp, err := http.Get("http://" + addr + "/hang")
		if err == nil && resp != nil {
			resp.Body.Close()
		}
	}()
	time.Sleep(10 * time.Millisecond) // let the request reach the handler

	cancel()
	if err := awaitShutdown(t, done); err == nil {
		t.Fatal("expected non-nil error when shutdown times out, got nil")
	}
}
