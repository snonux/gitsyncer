//go:build unix

package github

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestLoadToken_FIFOHasNoTokenAndDoesNotBlock mirrors
// codeberg.TestNewForgejoClient_FIFOHasNoTokenAndDoesNotBlock: a FIFO
// planted at the token path must be rejected by the O_NONBLOCK|O_NOFOLLOW
// hardened opener rather than blocking loadToken forever.
func TestLoadToken_FIFOHasNoTokenAndDoesNotBlock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "")
	tokenPath := filepath.Join(home, ".gitsyncer_github_token")
	if err := syscall.Mkfifo(tokenPath, 0600); err != nil {
		t.Fatalf("create token FIFO: %v", err)
	}

	done := make(chan string, 1)
	go func() {
		done <- loadToken("")
	}()

	select {
	case got := <-done:
		if got != "" {
			t.Fatalf("loadToken() = %q, want empty for FIFO", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("loading token from FIFO blocked")
	}

	if _, err := os.Stat(tokenPath); err != nil {
		t.Fatalf("stat token FIFO: %v", err)
	}
}
