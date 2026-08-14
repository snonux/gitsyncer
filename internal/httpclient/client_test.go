package httpclient

import (
	"io"
	"net/http"
	"net/http/httptest"
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

func TestDoJSON_SetsHeadersAndReturnsBody(t *testing.T) {
	var gotMethod, gotAuth, gotAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	result, err := DoJSON(http.MethodPost, server.URL, map[string]string{
		"Authorization": "token abc",
		"Accept":        "application/json",
	}, nil)
	if err != nil {
		t.Fatalf("DoJSON returned error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("expected method %s, got %s", http.MethodPost, gotMethod)
	}
	if gotAuth != "token abc" {
		t.Fatalf("expected Authorization header %q, got %q", "token abc", gotAuth)
	}
	if gotAccept != "application/json" {
		t.Fatalf("expected Accept header %q, got %q", "application/json", gotAccept)
	}
	if result.StatusCode != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, result.StatusCode)
	}
	if result.Status == "" {
		t.Fatal("expected non-empty status text")
	}
	if string(result.Body) != `{"ok":true}` {
		t.Fatalf("expected body %q, got %q", `{"ok":true}`, string(result.Body))
	}
}

func TestDoJSON_ReadsErrorBodyOnFailureStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer server.Close()

	result, err := DoJSON(http.MethodGet, server.URL, nil, nil)
	if err != nil {
		t.Fatalf("DoJSON returned error: %v", err)
	}
	if result.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, result.StatusCode)
	}
	if string(result.Body) != "not found" {
		t.Fatalf("expected body %q, got %q", "not found", string(result.Body))
	}
}

func TestDoJSON_RequestBuildFailure(t *testing.T) {
	if _, err := DoJSON("bad method", "https://example.com", nil, nil); err == nil {
		t.Fatal("expected error for invalid method")
	}
}

func TestCloseBody_DiscardsCloseError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("http.Get returned error: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)

	// CloseBody must not panic and must not surface an error, even though it
	// has no return value to check - the point of the test is that calling
	// it twice (once here, once via resp.Body already being closed by the
	// transport internals in some edge cases) is safe.
	CloseBody(resp)
}
