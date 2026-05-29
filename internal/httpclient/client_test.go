package httpclient

import (
	"net/http"
	"testing"
	"time"
)

func TestNewRequest_SetsDeadline(t *testing.T) {
	req, cancel, err := NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	defer cancel()

	deadline, ok := req.Context().Deadline()
	if !ok {
		t.Fatal("expected request context to include a deadline")
	}

	remaining := time.Until(deadline)
	if remaining <= 0 {
		t.Fatalf("expected future deadline, got %v", remaining)
	}
	if remaining > DefaultTimeout+time.Second {
		t.Fatalf("expected deadline near %v, got %v", DefaultTimeout, remaining)
	}
}

func TestNewRequest_InvalidMethod(t *testing.T) {
	req, cancel, err := NewRequest("bad method", "https://example.com", nil)
	if cancel != nil {
		defer cancel()
	}
	if err == nil {
		t.Fatalf("expected invalid method error, got request %#v", req)
	}
}

func TestDo_UsesSharedTimeout(t *testing.T) {
	if defaultClient.Timeout != DefaultTimeout {
		t.Fatalf("expected shared timeout %v, got %v", DefaultTimeout, defaultClient.Timeout)
	}
}

func TestDo_UsesConfiguredTransportSettings(t *testing.T) {
	transport, ok := defaultClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", defaultClient.Transport)
	}

	// Use explicit values so this test catches accidental policy drift in transport defaults.
	if transport.TLSHandshakeTimeout != 10*time.Second {
		t.Fatalf("expected TLS handshake timeout %v, got %v", 10*time.Second, transport.TLSHandshakeTimeout)
	}
	if transport.ResponseHeaderTimeout != 15*time.Second {
		t.Fatalf("expected response header timeout %v, got %v", 15*time.Second, transport.ResponseHeaderTimeout)
	}
	if transport.IdleConnTimeout != 90*time.Second {
		t.Fatalf("expected idle connection timeout %v, got %v", 90*time.Second, transport.IdleConnTimeout)
	}
	if transport.MaxIdleConnsPerHost != 10 {
		t.Fatalf("expected max idle conns per host %d, got %d", 10, transport.MaxIdleConnsPerHost)
	}
}
