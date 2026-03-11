package github

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestNewClient_DoesNotLogTokenDetails(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", " test-token \n")

	output := captureStdout(t, func() {
		client := NewClient("", "snonux")
		if !client.HasToken() {
			t.Fatal("expected token to be loaded")
		}
		if client.token != "test-token" {
			t.Fatalf("expected trimmed token, got %q", client.token)
		}
	})

	if output != "" {
		t.Fatalf("expected no stdout output, got %q", output)
	}
}

func TestClientRepoExists_DoesNotLogAuthorizationHeaderOnUnauthorized(t *testing.T) {
	const token = "super-secret-token"

	originalTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Authorization"); got != "Bearer "+token {
			t.Fatalf("expected bearer token header, got %q", got)
		}

		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Body:       io.NopCloser(strings.NewReader(`{"message":"bad credentials"}`)),
			Header:     make(http.Header),
		}, nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})

	client := NewClient(token, "snonux")

	output := captureStdout(t, func() {
		exists, err := client.RepoExists("gitsyncer")
		if err == nil {
			t.Fatal("expected unauthorized error")
		}
		if exists {
			t.Fatal("expected repo existence check to fail")
		}
		if !strings.Contains(err.Error(), "authentication failed (401)") {
			t.Fatalf("expected 401 error, got %v", err)
		}
	})

	if strings.Contains(output, token) {
		t.Fatalf("expected output to omit token, got %q", output)
	}
	if strings.Contains(output, "Authorization header") {
		t.Fatalf("expected output to omit authorization header log, got %q", output)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}

	os.Stdout = writer
	defer func() {
		os.Stdout = originalStdout
	}()

	outputCh := make(chan string, 1)
	go func() {
		var buffer bytes.Buffer
		_, _ = io.Copy(&buffer, reader)
		outputCh <- buffer.String()
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close stdout writer: %v", err)
	}

	output := <-outputCh
	if err := reader.Close(); err != nil {
		t.Fatalf("failed to close stdout reader: %v", err)
	}

	return output
}
