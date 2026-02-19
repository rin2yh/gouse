package graceful_test

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/rin2yh/gouse/net/graceful"
)

// newBenchmarkServer returns a controllableServer whose ListenAndServe blocks
// until Shutdown is called, then returns http.ErrServerClosed.
func newBenchmarkServer() *controllableServer {
	done := make(chan struct{})
	var once sync.Once
	return &controllableServer{
		listenFunc: func() error {
			<-done
			return http.ErrServerClosed
		},
		shutdownFunc: func(ctx context.Context) error {
			once.Do(func() { close(done) })
			return nil
		},
	}
}

func noopCleanups(n int) []func() {
	fns := make([]func(), n)
	for i := range fns {
		fns[i] = func() {}
	}
	return fns
}

func BenchmarkShutdown(b *testing.B) {
	for _, n := range []int{0, 1, 5, 10} {
		cleanups := noopCleanups(n)
		cfg := &graceful.Config{Cleanups: cleanups}
		b.Run(fmt.Sprintf("cleanups=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				srv := newBenchmarkServer()
				ctx, cancel := context.WithCancel(context.Background())
				done := make(chan error, 1)
				go func() { done <- graceful.Run(ctx, srv, cfg) }()
				cancel()
				if err := <-done; err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
