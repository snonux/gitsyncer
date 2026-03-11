package httpclient

import (
	"context"
	"io"
	"net/http"
	"time"
)

const DefaultTimeout = 30 * time.Second

var defaultClient = &http.Client{
	Timeout: DefaultTimeout,
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
