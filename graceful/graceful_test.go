package graceful_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/YuukiHayashi0510/gouse/graceful"
)

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

	handlerStarted := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/hang", func(w http.ResponseWriter, r *http.Request) {
		close(handlerStarted) // signal before blocking so cancel fires while in-flight
		time.Sleep(2 * shortTimeout)
		w.WriteHeader(http.StatusOK)
	})

	addr, cancel, done := startRun(t, mux, &graceful.Config{ShutdownTimeout: shortTimeout})

	// Client timeout prevents this goroutine hanging if the server never responds.
	client := &http.Client{Timeout: testShutdownTimeout}
	go func() {
		resp, err := client.Get("http://" + addr + "/hang")
		if err == nil && resp != nil {
			resp.Body.Close()
		}
	}()

	// Wait until the handler is executing before triggering shutdown so the
	// request is guaranteed to be in-flight (no timing dependency).
	select {
	case <-handlerStarted:
	case <-time.After(testStartTimeout):
		t.Fatal("handler did not start in time")
	}

	cancel()
	if err := awaitShutdown(t, done); err == nil {
		t.Fatal("expected non-nil error when shutdown times out, got nil")
	}
}
