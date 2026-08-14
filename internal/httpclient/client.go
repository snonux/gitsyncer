package httpclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const DefaultTimeout = 30 * time.Second

const (
	defaultTLSHandshakeTimeout   = 10 * time.Second
	defaultResponseHeaderTimeout = 15 * time.Second
	defaultIdleConnTimeout       = 90 * time.Second
	defaultMaxIdleConnsPerHost   = 10
)

var defaultTransport = func() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSHandshakeTimeout = defaultTLSHandshakeTimeout
	transport.ResponseHeaderTimeout = defaultResponseHeaderTimeout
	transport.IdleConnTimeout = defaultIdleConnTimeout
	transport.MaxIdleConnsPerHost = defaultMaxIdleConnsPerHost

	return transport
}()

var defaultClient = &http.Client{
	Timeout:   DefaultTimeout,
	Transport: defaultTransport,
}

func Do(req *http.Request) (*http.Response, error) {
	return defaultClient.Do(req)
}

func NewRequest(method, url string, body io.Reader) (*http.Request, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		cancel()
		return nil, nil, err
	}

	return req, cancel, nil
}

// CloseBody closes an HTTP response body. By the time this runs the body has
// already been fully read (or the caller is bailing out on an earlier
// error), so a close failure here cannot change the outcome that was already
// determined - the error is intentionally discarded rather than treated as
// actionable. This used to be a private closeResponseBody helper duplicated
// verbatim in the github and codeberg packages; it now lives here so DoJSON
// can use it too and so there is one place to change if that reasoning ever
// stops holding.
func CloseBody(resp *http.Response) {
	_ = resp.Body.Close()
}

// JSONResult is the outcome of one DoJSON round trip: the HTTP status code,
// the status line text (several forge error messages embed e.g. "404 Not
// Found"), and the full response body read into memory. The body is read
// eagerly so callers can use it for either an error message or a JSON decode
// after the connection has already been closed by DoJSON.
type JSONResult struct {
	StatusCode int
	Status     string
	Body       []byte
}

// DoJSON builds an HTTP request for method/url, attaches the given headers,
// executes it via Do, and reads the entire response body before closing it.
// It centralizes the "build request -> set headers -> Do -> defer close ->
// read body" plumbing that used to be duplicated across 15+ call sites in
// the github and codeberg clients (and, transitively, the release package
// that calls into them).
//
// DoJSON deliberately stops short of interpreting the status code or
// decoding JSON itself: different endpoints treat the same status
// differently (e.g. some callers treat 404 as "not found, not an error"
// while others treat it as a hard failure), and error message formats vary
// per call site. Callers inspect JSONResult.StatusCode/.Status and decode
// JSONResult.Body (via json.Unmarshal) themselves, keeping that call-site
// specific logic intact while sharing the mechanical HTTP plumbing here.
func DoJSON(method, url string, headers map[string]string, body io.Reader) (*JSONResult, error) {
	req, cancel, err := NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	defer cancel()

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := Do(req)
	if err != nil {
		return nil, err
	}
	defer CloseBody(resp)

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return &JSONResult{StatusCode: resp.StatusCode, Status: resp.Status, Body: data}, nil
}
