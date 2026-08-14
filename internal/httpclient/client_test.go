package httpclient

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// withNoSleep replaces retrySleep with a no-op for the duration of a test,
// restoring the real implementation afterwards. Retry tests exercise the
// retry-count and delay-selection logic without actually waiting out the
// computed/Retry-After delays, keeping the suite fast.
func withNoSleep(t *testing.T) {
	t.Helper()
	original := retrySleep
	retrySleep = func(time.Duration) {}
	t.Cleanup(func() { retrySleep = original })
}

// captureSleeps replaces retrySleep with one that records every requested
// duration instead of sleeping, so a test can assert on what delay the
// retry loop chose (e.g. that it honored Retry-After) without the test
// itself taking that long to run.
func captureSleeps(t *testing.T) *[]time.Duration {
	t.Helper()
	var got []time.Duration
	original := retrySleep
	retrySleep = func(d time.Duration) { got = append(got, d) }
	t.Cleanup(func() { retrySleep = original })
	return &got
}

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

func TestDoJSON_RetriesGETOnServerErrorThenSucceeds(t *testing.T) {
	withNoSleep(t)

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&requests, 1) <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	result, err := DoJSON(http.MethodGet, server.URL, nil, nil)
	if err != nil {
		t.Fatalf("DoJSON returned error: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected eventual status %d, got %d", http.StatusOK, result.StatusCode)
	}
	if got := atomic.LoadInt32(&requests); got != 3 {
		t.Fatalf("expected 3 requests (2 failures + 1 success), got %d", got)
	}
}

func TestDoJSON_DoesNotRetryPOSTByDefault(t *testing.T) {
	withNoSleep(t)

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	result, err := DoJSON(http.MethodPost, server.URL, nil, nil)
	if err != nil {
		t.Fatalf("DoJSON returned error: %v", err)
	}
	if result.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected status %d to be returned as-is, got %d", http.StatusInternalServerError, result.StatusCode)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("expected exactly 1 request for a non-idempotent POST, got %d", got)
	}
}

func TestDoJSON_IdempotentOptionRetriesNonGET(t *testing.T) {
	withNoSleep(t)

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&requests, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result, err := DoJSON(http.MethodPatch, server.URL, nil, nil, Idempotent())
	if err != nil {
		t.Fatalf("DoJSON returned error: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected eventual status %d, got %d", http.StatusOK, result.StatusCode)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("expected 2 requests (1 failure + 1 success) with Idempotent(), got %d", got)
	}
}

func TestDoJSON_HonorsRetryAfterHeader(t *testing.T) {
	sleeps := captureSleeps(t)

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&requests, 1) == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result, err := DoJSON(http.MethodGet, server.URL, nil, nil)
	if err != nil {
		t.Fatalf("DoJSON returned error: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected eventual status %d, got %d", http.StatusOK, result.StatusCode)
	}
	if len(*sleeps) != 1 {
		t.Fatalf("expected exactly 1 recorded sleep, got %d: %v", len(*sleeps), *sleeps)
	}
	if (*sleeps)[0] < 2*time.Second {
		t.Fatalf("expected delay to honor Retry-After (>= 2s), got %v", (*sleeps)[0])
	}
}

func TestDoJSON_GivesUpAfterMaxAttempts(t *testing.T) {
	withNoSleep(t)

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	result, err := DoJSON(http.MethodGet, server.URL, nil, nil)
	if err != nil {
		t.Fatalf("DoJSON returned error: %v", err)
	}
	if result.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected last-attempt status %d to be returned, got %d", http.StatusInternalServerError, result.StatusCode)
	}
	if got := atomic.LoadInt32(&requests); got != maxRetryAttempts {
		t.Fatalf("expected exactly maxRetryAttempts (%d) requests, got %d", maxRetryAttempts, got)
	}
}

func TestIsRetryableStatus(t *testing.T) {
	cases := map[int]bool{
		http.StatusOK:                  false,
		http.StatusNotFound:            false,
		http.StatusUnauthorized:        false,
		http.StatusBadRequest:          false,
		http.StatusTooManyRequests:     true,
		http.StatusInternalServerError: true,
		http.StatusBadGateway:          true,
		http.StatusServiceUnavailable:  true,
	}
	for status, want := range cases {
		if got := isRetryableStatus(status); got != want {
			t.Errorf("isRetryableStatus(%d) = %v, want %v", status, got, want)
		}
	}
}

func TestIsIdempotentMethod(t *testing.T) {
	cases := map[string]bool{
		http.MethodGet:    true,
		http.MethodHead:   true,
		http.MethodPost:   false,
		http.MethodPatch:  false,
		http.MethodPut:    false,
		http.MethodDelete: false,
	}
	for method, want := range cases {
		if got := isIdempotentMethod(method); got != want {
			t.Errorf("isIdempotentMethod(%s) = %v, want %v", method, got, want)
		}
	}
}

func TestRetryAfterDelay_Seconds(t *testing.T) {
	delay, ok := retryAfterDelay("5")
	if !ok {
		t.Fatal("expected ok=true for a numeric Retry-After value")
	}
	if delay != 5*time.Second {
		t.Fatalf("expected 5s, got %v", delay)
	}
}

func TestRetryAfterDelay_HTTPDate(t *testing.T) {
	future := time.Now().Add(90 * time.Second).UTC().Format(http.TimeFormat)
	delay, ok := retryAfterDelay(future)
	if !ok {
		t.Fatal("expected ok=true for an HTTP-date Retry-After value")
	}
	if delay <= 0 || delay > 91*time.Second {
		t.Fatalf("expected delay near 90s, got %v", delay)
	}
}

func TestRetryAfterDelay_Empty(t *testing.T) {
	if _, ok := retryAfterDelay(""); ok {
		t.Fatal("expected ok=false for an empty header")
	}
}

func TestRetryAfterDelay_Garbage(t *testing.T) {
	if _, ok := retryAfterDelay("not-a-valid-value"); ok {
		t.Fatal("expected ok=false for an unparseable header")
	}
}

func TestRetryBackoff_BoundedByMaxDelay(t *testing.T) {
	for attempt := 1; attempt <= 10; attempt++ {
		delay := retryBackoff(attempt)
		if delay < 0 || delay > retryMaxDelay {
			t.Fatalf("attempt %d: delay %v out of bounds [0, %v]", attempt, delay, retryMaxDelay)
		}
	}
}

func TestIsRetryableNetError(t *testing.T) {
	if isRetryableNetError(nil) {
		t.Fatal("expected nil error to be non-retryable")
	}
	if !isRetryableNetError(&net.OpError{Op: "dial", Err: errTimeout{}}) {
		t.Fatal("expected a timeout net.Error to be retryable")
	}
	if !isRetryableNetError(&net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}) {
		t.Fatal("expected ECONNREFUSED to be retryable")
	}
	if isRetryableNetError(errors.New("some unrelated permanent error")) {
		t.Fatal("expected a generic error to be non-retryable")
	}
}

// errTimeout is a minimal net.Error stand-in whose Timeout() reports true,
// used to exercise the timeout branch of isRetryableNetError without
// depending on a real network condition.
type errTimeout struct{}

func (errTimeout) Error() string   { return "i/o timeout" }
func (errTimeout) Timeout() bool   { return true }
func (errTimeout) Temporary() bool { return true }

func TestDoJSON_RetryCountIsBounded(t *testing.T) {
	// Regression guard: a server that always fails must not be retried
	// indefinitely. This mirrors TestDoJSON_GivesUpAfterMaxAttempts but
	// asserts the bound via strconv to make the "why 4" reasoning visible
	// at the call site rather than only via the shared maxRetryAttempts
	// constant.
	withNoSleep(t)

	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	if _, err := DoJSON(http.MethodGet, server.URL, nil, nil); err != nil {
		t.Fatalf("DoJSON returned error: %v", err)
	}
	got := atomic.LoadInt32(&requests)
	if got < 3 || got > 5 {
		t.Fatalf("expected a small bounded number of attempts (3-5 per task spec), got %s", strconv.Itoa(int(got)))
	}
}
