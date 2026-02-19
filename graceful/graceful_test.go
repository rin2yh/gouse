package graceful_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/YuukiHayashi0510/gouse/graceful"
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

func TestRunGracefulShutdown(t *testing.T) {
	addr := freePort(t)
	srv := &http.Server{Addr: addr, Handler: http.DefaultServeMux}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- graceful.Run(ctx, srv, 5*time.Second)
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

func TestRunServerError(t *testing.T) {
	addr := freePort(t)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srv := &http.Server{Addr: addr, Handler: http.DefaultServeMux}

	err = graceful.Run(context.Background(), srv, 5*time.Second)
	if err == nil {
		t.Fatal("expected error when port is already in use, got nil")
	}
}

func TestRunCleanup(t *testing.T) {
	addr := freePort(t)
	srv := &http.Server{Addr: addr, Handler: http.DefaultServeMux}

	ctx, cancel := context.WithCancel(context.Background())

	var called []string
	done := make(chan error, 1)
	go func() {
		done <- graceful.Run(ctx, srv, 5*time.Second,
			func() { called = append(called, "first") },
			func() { called = append(called, "second") },
		)
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

	if len(called) != 2 || called[0] != "first" || called[1] != "second" {
		t.Fatalf("expected cleanups to run in order, got: %v", called)
	}
}

func TestRunHandlesRequests(t *testing.T) {
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
		done <- graceful.Run(ctx, srv, 5*time.Second)
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
