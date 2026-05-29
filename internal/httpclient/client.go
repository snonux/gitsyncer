package httpclient

import (
	"context"
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
