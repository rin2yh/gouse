package graceful_test

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/YuukiHayashi0510/gouse/graceful"
)

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
