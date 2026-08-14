package forge

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

// These tests exercise ResolveToken's file-based fallback (used when neither
// an explicit config token nor the named environment variable is set).
// ResolveToken reads the token file through ReadProtectedTokenFile instead of
// os.ReadFile, so a symlink, a non-regular file, or an overly-permissive file
// must all be rejected. This coverage used to live in internal/cli as tests
// of the now-removed loadTokenWithFallback, and was duplicated in spirit by
// similar tests in internal/github and internal/codeberg; task g01 moved it
// here, next to the single implementation all three forges now call.

func TestResolveToken_AcceptsOwnerOnlyTokenFile(t *testing.T) {
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

			got := ResolveToken("", "GITSYNCER_TEST_TOKEN", ".gitsyncer_test_token")
			if got != "file-token" {
				t.Fatalf("ResolveToken() = %q, want trimmed file token", got)
			}
		})
	}
}

func TestResolveToken_RejectsSymlinkTokenFile(t *testing.T) {
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

	got := ResolveToken("", "GITSYNCER_TEST_TOKEN", ".gitsyncer_test_token")
	if got != "" {
		t.Fatalf("ResolveToken() = %q, want empty for symlinked token file", got)
	}
}

func TestResolveToken_RejectsOverlyPermissiveTokenFile(t *testing.T) {
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

	got := ResolveToken("", "GITSYNCER_TEST_TOKEN", ".gitsyncer_test_token")
	if got != "" {
		t.Fatalf("ResolveToken() = %q, want empty for group/other readable token file", got)
	}
}

func TestResolveToken_RejectsNonRegularTokenFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITSYNCER_TEST_TOKEN", "")

	listener, err := net.Listen("unix", filepath.Join(home, ".gitsyncer_test_token"))
	if err != nil {
		t.Fatalf("create token socket: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	got := ResolveToken("", "GITSYNCER_TEST_TOKEN", ".gitsyncer_test_token")
	if got != "" {
		t.Fatalf("ResolveToken() = %q, want empty for non-regular token file", got)
	}
}

func TestResolveToken_MissingTokenFileHasNoToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITSYNCER_TEST_TOKEN", "")

	got := ResolveToken("", "GITSYNCER_TEST_TOKEN", ".gitsyncer_test_token")
	if got != "" {
		t.Fatalf("ResolveToken() = %q, want empty when token file is missing", got)
	}
}

func TestResolveToken_PrecedenceConfigOverEnvOverFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITSYNCER_TEST_TOKEN", "env-token")

	tokenFile := filepath.Join(home, ".gitsyncer_test_token")
	if err := os.WriteFile(tokenFile, []byte("file-token"), 0600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	if got := ResolveToken("config-token", "GITSYNCER_TEST_TOKEN", ".gitsyncer_test_token"); got != "config-token" {
		t.Fatalf("ResolveToken() = %q, want config token to take precedence", got)
	}
	if got := ResolveToken("", "GITSYNCER_TEST_TOKEN", ".gitsyncer_test_token"); got != "env-token" {
		t.Fatalf("ResolveToken() = %q, want env token when config is empty", got)
	}
}

func TestResolveToken_TrimsConfigAndEnvValues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Setenv("GITSYNCER_TEST_TOKEN", "")
	if got := ResolveToken("  config-token\n", "GITSYNCER_TEST_TOKEN", ".gitsyncer_test_token"); got != "config-token" {
		t.Fatalf("ResolveToken() = %q, want trimmed config token", got)
	}

	t.Setenv("GITSYNCER_TEST_TOKEN", "  env-token\n")
	if got := ResolveToken("", "GITSYNCER_TEST_TOKEN", ".gitsyncer_test_token"); got != "env-token" {
		t.Fatalf("ResolveToken() = %q, want trimmed env token", got)
	}
}
