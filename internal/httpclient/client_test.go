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
