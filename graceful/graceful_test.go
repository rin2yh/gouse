package graceful_test

import (
	"context"
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
	return context.DeadlineExceeded
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

func TestRunServerError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srv := &http.Server{Addr: ln.Addr().String(), Handler: http.DefaultServeMux}

	err = graceful.Run(context.Background(), srv, nil)
	if err == nil {
		t.Fatal("expected error when port is already in use, got nil")
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
