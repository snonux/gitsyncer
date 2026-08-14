//go:build unix

package cli

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestLoadTokenWithFallback_FIFOHasNoTokenAndDoesNotBlock mirrors the
// equivalent FIFO tests in the github and codeberg packages: a FIFO planted
// at the token path must be rejected by the hardened opener rather than
// blocking loadTokenWithFallback forever.
func TestLoadTokenWithFallback_FIFOHasNoTokenAndDoesNotBlock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITSYNCER_TEST_TOKEN", "")
	tokenPath := filepath.Join(home, ".gitsyncer_test_token")
	if err := syscall.Mkfifo(tokenPath, 0600); err != nil {
		t.Fatalf("create token FIFO: %v", err)
	}

	done := make(chan string, 1)
	go func() {
		done <- loadTokenWithFallback("", "GITSYNCER_TEST_TOKEN", ".gitsyncer_test_token")
	}()

	select {
	case got := <-done:
		if got != "" {
			t.Fatalf("loadTokenWithFallback() = %q, want empty for FIFO", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("loading token from FIFO blocked")
	}

	if _, err := os.Stat(tokenPath); err != nil {
		t.Fatalf("stat token FIFO: %v", err)
	}
}
