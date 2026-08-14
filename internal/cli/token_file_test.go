package cli

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

// loadTokenWithFallback's token-file branch now reads through
// forge.ReadProtectedTokenFile instead of os.ReadFile (see internal/forge),
// so it must reject the same symlink/non-regular/overly-permissive attacks
// that github.loadToken and codeberg's token loaders already reject.

func TestLoadTokenWithFallback_AcceptsOwnerOnlyTokenFile(t *testing.T) {
	for _, mode := range []os.FileMode{0600, 0400} {
		t.Run(mode.String(), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("GITSYNCER_TEST_TOKEN", "")

			tokenFile := filepath.Join(home, ".gitsyncer_test_token")
			if err := os.WriteFile(tokenFile, []byte("  file-token\n"), mode); err != nil {
				t.Fatalf("write token file: %v", err)
			}
			if err := os.Chmod(tokenFile, mode); err != nil {
				t.Fatalf("set token file mode: %v", err)
			}

			got := loadTokenWithFallback("", "GITSYNCER_TEST_TOKEN", ".gitsyncer_test_token")
			if got != "file-token" {
				t.Fatalf("loadTokenWithFallback() = %q, want trimmed file token", got)
			}
		})
	}
}

func TestLoadTokenWithFallback_RejectsSymlinkTokenFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITSYNCER_TEST_TOKEN", "")

	target := filepath.Join(home, "token-target")
	if err := os.WriteFile(target, []byte("file-token"), 0600); err != nil {
		t.Fatalf("write symlink target: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(home, ".gitsyncer_test_token")); err != nil {
		t.Fatalf("create token symlink: %v", err)
	}

	got := loadTokenWithFallback("", "GITSYNCER_TEST_TOKEN", ".gitsyncer_test_token")
	if got != "" {
		t.Fatalf("loadTokenWithFallback() = %q, want empty for symlinked token file", got)
	}
}

func TestLoadTokenWithFallback_RejectsOverlyPermissiveTokenFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITSYNCER_TEST_TOKEN", "")

	tokenFile := filepath.Join(home, ".gitsyncer_test_token")
	if err := os.WriteFile(tokenFile, []byte("file-token"), 0644); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	if err := os.Chmod(tokenFile, 0644); err != nil {
		t.Fatalf("set token file mode: %v", err)
	}

	got := loadTokenWithFallback("", "GITSYNCER_TEST_TOKEN", ".gitsyncer_test_token")
	if got != "" {
		t.Fatalf("loadTokenWithFallback() = %q, want empty for group/other readable token file", got)
	}
}

func TestLoadTokenWithFallback_RejectsNonRegularTokenFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITSYNCER_TEST_TOKEN", "")

	listener, err := net.Listen("unix", filepath.Join(home, ".gitsyncer_test_token"))
	if err != nil {
		t.Fatalf("create token socket: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	got := loadTokenWithFallback("", "GITSYNCER_TEST_TOKEN", ".gitsyncer_test_token")
	if got != "" {
		t.Fatalf("loadTokenWithFallback() = %q, want empty for non-regular token file", got)
	}
}

func TestLoadTokenWithFallback_MissingTokenFileHasNoToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITSYNCER_TEST_TOKEN", "")

	got := loadTokenWithFallback("", "GITSYNCER_TEST_TOKEN", ".gitsyncer_test_token")
	if got != "" {
		t.Fatalf("loadTokenWithFallback() = %q, want empty when token file is missing", got)
	}
}
