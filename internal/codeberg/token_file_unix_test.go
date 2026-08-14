//go:build unix

package codeberg

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"codeberg.org/snonux/gitsyncer/internal/forge"
)

func TestNewForgejoClient_FIFOHasNoTokenAndDoesNotBlock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FORGEJO_TOKEN", "")
	tokenPath := filepath.Join(home, ".gitsyncer_forgejo_token")
	if err := syscall.Mkfifo(tokenPath, 0600); err != nil {
		t.Fatalf("create token FIFO: %v", err)
	}

	done := make(chan *Client, 1)
	go func() {
		done <- NewForgejoClient("https://forgejo.example/api/v1", "owner", forge.OwnerTypeUser)
	}()

	select {
	case client := <-done:
		if client.HasToken() {
			t.Fatal("HasToken() = true for FIFO, want false")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("loading token from FIFO blocked")
	}

	if _, err := os.Stat(tokenPath); err != nil {
		t.Fatalf("stat token FIFO: %v", err)
	}
}

// TestNewClient_FIFOHasNoTokenAndDoesNotBlock covers Client.loadToken's
// plain (non-Forgejo) token file fallback with the same FIFO attack as
// TestNewForgejoClient_FIFOHasNoTokenAndDoesNotBlock above, now that it also
// reads through the hardened opener.
func TestNewClient_FIFOHasNoTokenAndDoesNotBlock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEBERG_TOKEN", "")
	tokenPath := filepath.Join(home, ".gitsyncer_codeberg_token")
	if err := syscall.Mkfifo(tokenPath, 0600); err != nil {
		t.Fatalf("create token FIFO: %v", err)
	}

	done := make(chan *Client, 1)
	go func() {
		done <- NewClient("", "example-org")
	}()

	select {
	case client := <-done:
		if client.HasToken() {
			t.Fatal("HasToken() = true for FIFO, want false")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("loading token from FIFO blocked")
	}

	if _, err := os.Stat(tokenPath); err != nil {
		t.Fatalf("stat token FIFO: %v", err)
	}
}
