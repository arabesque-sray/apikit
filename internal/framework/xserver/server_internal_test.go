package xserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/go-ozzo/ozzo-routing"
)

func TestNewServer(t *testing.T) {

	tests := []struct {
		name       string
		args       []Middleware
		wantBefore bool
		wantAfter  bool
	}{
		{
			name: "NoMiddleware",
			args: nil,
		},
		{
			name: "OnlyBeforeMiddleware",
			args: []Middleware{
				{
					Handler: func(c *routing.Context) error { return nil },
					After:   false,
				},
			},
			wantBefore: true,
		},
		{
			name: "OnlyAfterMiddleware",
			args: []Middleware{
				{
					Handler: func(c *routing.Context) error { return nil },
					After:   true,
				},
			},
			wantAfter: true,
		},
		{
			name: "BeforeAndAfterMiddleware",
			args: []Middleware{
				{
					Handler: func(c *routing.Context) error { return nil },
					After:   false,
				},
				{
					Handler: func(c *routing.Context) error { return nil },
					After:   true,
				},
			},
			wantAfter:  true,
			wantBefore: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			got := NewServer(&ServerOpts{Middleware: tc.args})
			if (len(got.after) != 0) != tc.wantAfter {
				t.Fatalf("xserver.NewServer().after = %v, want %v", got.after, tc.wantAfter)
			}
			if (len(got.before) != 0) != tc.wantBefore {
				t.Fatalf("xserver.NewServer().before = %v, want %v", got.before, tc.wantAfter)
			}
		})
	}
}

func TestServer_ShutdownBeforeStart(t *testing.T) {

	server := NewServer(nil)
	if err := server.Shutdown(context.Background()); err != nil {
		t.Fatalf("xserver.Server.Shutdown() before Start = %v, want nil", err)
	}
	if err := server.Stop(); err != nil {
		t.Fatalf("xserver.Server.Stop() before Start = %v, want nil", err)
	}
}

// serveBlocking starts server with a handler that blocks until release is
// closed, sends one request and waits until the handler is running.
func serveBlocking(t *testing.T, release <-chan struct{}) (server *Server, served <-chan error, response <-chan error) {
	t.Helper()

	started := make(chan struct{})
	server = NewServer(nil)
	server.server = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
	})}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() = %v", err)
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.server.Serve(ln) }()

	respErr := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String())
		if err == nil {
			resp.Body.Close()
		}
		respErr <- err
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not reach the handler")
	}
	return server, serveErr, respErr
}

func TestServer_ShutdownWaitsForInFlightRequest(t *testing.T) {

	release := make(chan struct{})
	server, served, response := serveBlocking(t, release)

	shutdown := make(chan error, 1)
	go func() { shutdown <- server.Shutdown(context.Background()) }()

	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("http.Server.Serve() = %v, want %v", err, http.ErrServerClosed)
	}
	select {
	case err := <-shutdown:
		t.Fatalf("xserver.Server.Shutdown() returned %v while a request was in flight", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	if err := <-response; err != nil {
		t.Fatalf("in-flight request failed: %v", err)
	}
	if err := <-shutdown; err != nil {
		t.Fatalf("xserver.Server.Shutdown() = %v, want nil", err)
	}
}

func TestServer_ShutdownStopsAtContextDeadline(t *testing.T) {

	release := make(chan struct{})
	defer close(release)
	server, _, _ := serveBlocking(t, release)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := server.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("xserver.Server.Shutdown() = %v, want %v", err, context.DeadlineExceeded)
	}
}
