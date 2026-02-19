package httpx_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/YuukiHayashi0510/gouse/httpx"
)

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

func TestRunWithContext_GracefulShutdown(t *testing.T) {
	addr := freePort(t)
	srv := &http.Server{Addr: addr, Handler: http.DefaultServeMux}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- httpx.RunWithContext(ctx, srv)
	}()

	// Wait for server to start accepting connections.
	if err := waitForServer(addr, 2*time.Second); err != nil {
		t.Fatal("server did not start in time:", err)
	}

	// Trigger shutdown.
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}

func TestRunWithContext_ServerError(t *testing.T) {
	addr := freePort(t)

	// Occupy the port so ListenAndServe fails.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srv := &http.Server{Addr: addr, Handler: http.DefaultServeMux}
	ctx := context.Background()

	err = httpx.RunWithContext(ctx, srv)
	if err == nil {
		t.Fatal("expected error when port is already in use, got nil")
	}
}

func TestRunWithContext_WithShutdownTimeout(t *testing.T) {
	addr := freePort(t)
	srv := &http.Server{Addr: addr, Handler: http.DefaultServeMux}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- httpx.RunWithContext(ctx, srv, httpx.WithShutdownTimeout(3*time.Second))
	}()

	if err := waitForServer(addr, 2*time.Second); err != nil {
		t.Fatal("server did not start in time:", err)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}

func TestRunWithContext_HandlesRequests(t *testing.T) {
	addr := freePort(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := &http.Server{Addr: addr, Handler: mux}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- httpx.RunWithContext(ctx, srv)
	}()

	if err := waitForServer(addr, 2*time.Second); err != nil {
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
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}

// waitForServer polls until the address is reachable or timeout expires.
func waitForServer(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return context.DeadlineExceeded
}
