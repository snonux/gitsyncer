package github

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

// These tests exercise loadToken's file-based fallback (used when neither an
// explicit token nor GITHUB_TOKEN is set). loadToken now reads the token
// file through forge.ReadProtectedTokenFile instead of os.ReadFile, so a
// symlink, a non-regular file, or an overly-permissive file must all be
// rejected the same way loadForgejoToken already rejects them in the
// codeberg package.

func TestLoadToken_AcceptsOwnerOnlyTokenFile(t *testing.T) {
	for _, mode := range []os.FileMode{0600, 0400} {
		t.Run(mode.String(), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("GITHUB_TOKEN", "")
			tokenFile := filepath.Join(home, ".gitsyncer_github_token")
			if err := os.WriteFile(tokenFile, []byte("  file-token\n"), mode); err != nil {
				t.Fatalf("write token file: %v", err)
			}
			if err := os.Chmod(tokenFile, mode); err != nil {
				t.Fatalf("set token file mode: %v", err)
			}

			got := loadToken("")
			if got != "file-token" {
				t.Fatalf("loadToken() = %q, want trimmed file token", got)
			}
		})
	}
}

func TestLoadToken_RejectsSymlinkTokenFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "")

	target := filepath.Join(home, "token-target")
	if err := os.WriteFile(target, []byte("file-token"), 0600); err != nil {
		t.Fatalf("write symlink target: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(home, ".gitsyncer_github_token")); err != nil {
		t.Fatalf("create token symlink: %v", err)
	}

	if got := loadToken(""); got != "" {
		t.Fatalf("loadToken() = %q, want empty for symlinked token file", got)
	}
}

func TestLoadToken_RejectsOverlyPermissiveTokenFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "")

	tokenFile := filepath.Join(home, ".gitsyncer_github_token")
	if err := os.WriteFile(tokenFile, []byte("file-token"), 0644); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	if err := os.Chmod(tokenFile, 0644); err != nil {
		t.Fatalf("set token file mode: %v", err)
	}

	if got := loadToken(""); got != "" {
		t.Fatalf("loadToken() = %q, want empty for group/other readable token file", got)
	}
}

func TestLoadToken_RejectsNonRegularTokenFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "")

	listener, err := net.Listen("unix", filepath.Join(home, ".gitsyncer_github_token"))
	if err != nil {
		t.Fatalf("create token socket: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	if got := loadToken(""); got != "" {
		t.Fatalf("loadToken() = %q, want empty for non-regular token file", got)
	}
}

func TestLoadToken_MissingTokenFileHasNoToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "")

	if got := loadToken(""); got != "" {
		t.Fatalf("loadToken() = %q, want empty when token file is missing", got)
	}
}
