package httpclient

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	t.Parallel()

	client := New(5 * time.Second)
	if got, want := client.Timeout, 5*time.Second; got != want {
		t.Fatalf("client.Timeout = %v, want %v", got, want)
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client.Transport type = %T, want *http.Transport", client.Transport)
	}
	if !transport.ForceAttemptHTTP2 {
		t.Fatal("transport.ForceAttemptHTTP2 = false, want true")
	}
	if transport.Proxy != nil {
		t.Fatal("transport.Proxy != nil, want ambient proxies disabled")
	}
	if !transport.DisableCompression {
		t.Fatal("transport.DisableCompression = false, want true")
	}
	if got, want := transport.MaxIdleConns, 4; got != want {
		t.Fatalf("transport.MaxIdleConns = %d, want %d", got, want)
	}
	if got, want := transport.MaxIdleConnsPerHost, 2; got != want {
		t.Fatalf("transport.MaxIdleConnsPerHost = %d, want %d", got, want)
	}
	if got, want := transport.MaxConnsPerHost, 4; got != want {
		t.Fatalf("transport.MaxConnsPerHost = %d, want %d", got, want)
	}
	if got, want := transport.MaxResponseHeaderBytes, int64(maxResponseHeaderBytes); got != want {
		t.Fatalf("transport.MaxResponseHeaderBytes = %d, want %d", got, want)
	}
	if got := transport.TLSClientConfig.MinVersion; got == 0 {
		t.Fatal("transport.TLSClientConfig.MinVersion = 0, want TLS minimum")
	}
	if err := client.CheckRedirect(&http.Request{}, nil); err == nil {
		t.Fatal("CheckRedirect() error = nil, want redirect rejection")
	}
}

func TestNewUsesRequestedNetwork(t *testing.T) {
	t.Parallel()

	listener := newTCPListener(t, "tcp4", "127.0.0.1:0")
	defer listener.Close()
	acceptOnceAndClose(t, listener)

	client := New(5 * time.Second)
	transport := transportForClient(t, client)

	conn, err := transport.DialContext(context.Background(), "tcp4", listener.Addr().String())
	if err != nil {
		t.Fatalf("DialContext() error = %v", err)
	}
	_ = conn.Close()
}

func TestNewWithNetworkForcesNetwork(t *testing.T) {
	t.Parallel()

	listener := newTCPListener(t, "tcp4", "127.0.0.1:0")
	defer listener.Close()
	acceptOnceAndClose(t, listener)

	client := NewWithNetwork(5*time.Second, "tcp4")
	transport := transportForClient(t, client)

	conn, err := transport.DialContext(context.Background(), "tcp6", listener.Addr().String())
	if err != nil {
		t.Fatalf("DialContext() error = %v", err)
	}
	_ = conn.Close()
}

func transportForClient(t *testing.T, client *http.Client) *http.Transport {
	t.Helper()

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client.Transport type = %T, want *http.Transport", client.Transport)
	}
	return transport
}

func newTCPListener(t *testing.T, network string, address string) net.Listener {
	t.Helper()

	listener, err := net.Listen(network, address)
	if err != nil {
		t.Fatalf("net.Listen(%q, %q) error = %v", network, address, err)
	}
	return listener
}

func acceptOnceAndClose(t *testing.T, listener net.Listener) {
	t.Helper()

	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		_ = conn.Close()
		done <- nil
	}()

	t.Cleanup(func() {
		select {
		case err := <-done:
			if err != nil && !isClosedNetworkError(err) {
				t.Fatalf("listener.Accept() error = %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("listener.Accept() did not complete")
		}
	})
}

func isClosedNetworkError(err error) bool {
	return err != nil && (errors.Is(err, net.ErrClosed) || strings.Contains(err.Error(), "use of closed network connection"))
}
