package httpclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"syscall"
	"time"
)

const DefaultTimeout = 30 * time.Second

const (
	defaultTLSHandshakeTimeout   = 10 * time.Second
	defaultResponseHeaderTimeout = 15 * time.Second
	defaultIdleConnTimeout       = 90 * time.Second
	defaultMaxIdleConnsPerHost   = 10
)

// Retry policy for DoJSON. Batch sync can make dozens of forge API calls in
// a row (one org/repo at a time), so a single transient 5xx or rate-limit
// response used to fail the whole run. DoJSON now retries such failures
// automatically, but only for requests it knows are safe to repeat:
//
//   - Idempotent by default: GET/HEAD, since repeating them can't cause a
//     duplicate side effect. Any other method (POST/PATCH/DELETE) is retried
//     only if the caller opts in via the Idempotent() option - retrying repo
//     or release creation blindly risks creating it twice if the first
//     attempt actually succeeded server-side but the response was lost.
//   - Retryable outcomes: a 5xx response, a 429 (rate limited) response, or
//     a transient network-level error (timeout, connection refused/reset).
//     Any other 4xx (bad request, 401, 404, ...) is treated as permanent and
//     returned immediately.
//   - Retry-After is honored: if a 429/5xx response carries a Retry-After
//     header (GitHub sends this), the wait before the next attempt is at
//     least that long, on top of the computed backoff.
//   - Bounded: at most maxRetryAttempts total attempts (including the
//     first), with exponential backoff capped at retryMaxDelay and full
//     jitter, so a persistently failing or rate-limited call fails
//     eventually and returns its last response/error instead of retrying
//     forever.
const (
	// maxRetryAttempts is the total number of attempts (including the
	// first) made for a retryable request. Chosen to absorb a couple of
	// transient blips without turning a rate-limited batch sync into a
	// long hang.
	maxRetryAttempts = 4
	retryBaseDelay   = 200 * time.Millisecond
	retryMaxDelay    = 5 * time.Second
)

// retrySleep waits between retry attempts. It is a package variable rather
// than a direct time.Sleep call so tests can replace it with a fast or
// duration-capturing stand-in instead of actually sleeping - that keeps the
// retry-count and Retry-After behavior testable without slowing down the
// test suite by seconds per test.
var retrySleep = time.Sleep

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
// Found"), the response headers (used internally to honor Retry-After, and
// available to callers for the same reason), and the full response body
// read into memory. The body is read eagerly so callers can use it for
// either an error message or a JSON decode after the connection has already
// been closed by DoJSON.
type JSONResult struct {
	StatusCode int
	Status     string
	Header     http.Header
	Body       []byte
}

// retryOptions configures a single DoJSON call's retry behavior. The zero
// value is the default policy: only GET/HEAD requests are retried.
type retryOptions struct {
	forceIdempotent bool
}

// Option customizes one DoJSON call. See Idempotent.
type Option func(*retryOptions)

// Idempotent marks a non-GET/HEAD request as safe to retry on a transient
// failure, opting it into the same retry treatment GET gets automatically.
// Use it only for calls that are genuinely safe to repeat (e.g. a PATCH that
// sets a field to an exact value). No current caller in this codebase needs
// it: POST calls that create a repo or release are deliberately left
// single-shot, since retrying them could create a duplicate if the first
// attempt actually succeeded server-side but the response was lost.
func Idempotent() Option {
	return func(o *retryOptions) { o.forceIdempotent = true }
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
//
// It also applies the package's retry policy (see the const block above):
// GET/HEAD requests, or any request passed the Idempotent() option, are
// retried on a 5xx/429 response or a transient network error, honoring
// Retry-After when present, up to maxRetryAttempts total tries. All other
// requests remain single-shot, and a non-retryable outcome (permanent
// 4xx error, non-network error, or a retryable-looking outcome on the final
// attempt) is returned to the caller exactly as before - this function's
// signature and default behavior for existing call sites are unchanged.
func DoJSON(method, url string, headers map[string]string, body io.Reader, opts ...Option) (*JSONResult, error) {
	var options retryOptions
	for _, opt := range opts {
		opt(&options)
	}

	bodyBytes, err := readAllIfNotNil(body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	attempts := 1
	if isIdempotentMethod(method) || options.forceIdempotent {
		attempts = maxRetryAttempts
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		result, err := doJSONOnce(method, url, headers, newBodyReader(bodyBytes))
		retry, delay := shouldRetry(attempt, attempts, result, err)
		if !retry {
			return result, err
		}
		lastErr = err
		retrySleep(delay)
	}

	// Unreachable: the loop always returns via the !retry branch above once
	// attempt reaches attempts, since shouldRetry(attempts, attempts, ...)
	// is always false. Kept as a safety net rather than a panic.
	return nil, lastErr
}

// doJSONOnce performs a single request/response round trip: build the
// request, attach headers, execute it, and read the body into memory. This
// is the part of DoJSON that used to be its entire body before retry
// support was added; it now runs once per attempt.
func doJSONOnce(method, url string, headers map[string]string, body io.Reader) (*JSONResult, error) {
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

	return &JSONResult{StatusCode: resp.StatusCode, Status: resp.Status, Header: resp.Header, Body: data}, nil
}

// shouldRetry decides whether the DoJSON retry loop should make another
// attempt and, if so, how long to wait first. It never retries once
// attempts is reached (the caller sets attempts to 1 for non-idempotent
// requests, so this also enforces "no retry by default" for those). Beyond
// that, it retries only a transient network error or a 429/5xx response,
// honoring the response's Retry-After header (if any) as a floor on the
// computed exponential-backoff delay.
func shouldRetry(attempt, attempts int, result *JSONResult, err error) (bool, time.Duration) {
	if attempt >= attempts {
		return false, 0
	}
	if err != nil {
		if !isRetryableNetError(err) {
			return false, 0
		}
		return true, retryBackoff(attempt)
	}
	if !isRetryableStatus(result.StatusCode) {
		return false, 0
	}

	delay := retryBackoff(attempt)
	if after, ok := retryAfterDelay(result.Header.Get("Retry-After")); ok && after > delay {
		delay = after
	}
	return true, delay
}

// isIdempotentMethod reports whether method is safe to retry automatically
// without an explicit opt-in. Only GET/HEAD qualify: they have no side
// effects, so repeating one after a lost response is always safe. Every
// other method needs the caller to pass Idempotent() explicitly.
func isIdempotentMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead:
		return true
	default:
		return false
	}
}

// isRetryableStatus reports whether an HTTP status code represents a
// transient failure worth retrying: any 5xx server error, or 429 (rate
// limited). Other 4xx statuses (400, 401, 404, ...) are permanent from the
// client's point of view and are never retried.
func isRetryableStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= 500
}

// isRetryableNetError reports whether err is a transient network-level
// failure - a timeout, or a connection refused/reset - as opposed to
// something retrying can't fix (a malformed URL, a canceled context, TLS
// verification failure, etc).
func isRetryableNetError(err error) bool {
	if err == nil {
		return false
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	return errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, io.ErrUnexpectedEOF)
}

// retryBackoff computes the delay before the attempt after the given
// (1-based) attempt number, using exponential backoff with full jitter -
// a uniformly random duration in [0, cap), where cap doubles each attempt
// up to retryMaxDelay. Full jitter (rather than a fixed or half-jittered
// delay) avoids many gitsyncer runs that hit a rate limit at the same time
// from backing off in lockstep and re-colliding on the next attempt.
func retryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}

	ceiling := retryBaseDelay * time.Duration(uint64(1)<<uint(attempt-1))
	if ceiling <= 0 || ceiling > retryMaxDelay {
		ceiling = retryMaxDelay
	}

	return time.Duration(rand.Int63n(int64(ceiling)))
}

// retryAfterDelay parses an HTTP Retry-After header value, per RFC 7231:
// either an integer number of seconds, or an HTTP-date. GitHub sets this on
// 429 (and sometimes 503) responses to state exactly how long to wait; when
// present it takes priority over the computed backoff (see shouldRetry) so
// the retry loop respects the server's own rate-limit accounting instead of
// guessing.
func retryAfterDelay(header string) (time.Duration, bool) {
	if header == "" {
		return 0, false
	}

	if seconds, err := strconv.Atoi(header); err == nil {
		if seconds < 0 {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}

	if when, err := http.ParseTime(header); err == nil {
		delay := time.Until(when)
		if delay < 0 {
			delay = 0
		}
		return delay, true
	}

	return 0, false
}

// readAllIfNotNil reads body fully into memory, returning nil if body is
// nil. DoJSON buffers the request body up front (bodies here are small JSON
// payloads, or nil for GET) so a retried attempt can replay the exact same
// bytes - an io.Reader can normally only be consumed once, which would
// otherwise send an empty body on any attempt after the first.
func readAllIfNotNil(body io.Reader) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	return io.ReadAll(body)
}

// newBodyReader returns a fresh reader over bodyBytes for one attempt, or
// nil if there was no request body. bodyBytes is read once per attempt by
// wrapping a new bytes.Reader around it, since http.Request consumes its
// body reader.
func newBodyReader(bodyBytes []byte) io.Reader {
	if bodyBytes == nil {
		return nil
	}
	return bytes.NewReader(bodyBytes)
}
