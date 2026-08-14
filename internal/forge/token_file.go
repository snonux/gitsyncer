package forge

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ReadProtectedTokenFile reads a credential file at path using hardened
// semantics shared by every forge client (GitHub, Codeberg, Forgejo, and the
// CLI's release token loading). Token files live at well-known paths under
// the user's home directory, so a local attacker able to plant something at
// that path before gitsyncer runs could otherwise cause it to:
//
//   - follow a symlink and read an unrelated, possibly sensitive file, or
//   - open a FIFO and block the process indefinitely, or
//   - read a token file that is also readable by other local users/groups,
//     leaking the credential.
//
// openProtectedTokenFile (platform-specific) opens the path with
// O_NOFOLLOW|O_NONBLOCK so a symlink is rejected outright and a FIFO cannot
// block the open call. The Stat check below then rejects anything that
// isn't a plain regular file and any file with group/other permission bits
// set (mode&0077 != 0), i.e. it only accepts owner-only files such as 0600
// or 0400. The returned token has leading/trailing whitespace trimmed,
// since tokens are commonly stored with a trailing newline.
func ReadProtectedTokenFile(path string) (string, error) {
	file, err := openProtectedTokenFile(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("token file %s is not a regular file", path)
	}
	if info.Mode().Perm()&0077 != 0 {
		return "", fmt.Errorf("token file %s must not be readable or writable by group/other", path)
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// ResolveToken resolves a forge API token via the shared config -> env var ->
// token file cascade. This is the single source of truth for token
// precedence: it used to be reimplemented separately by the GitHub client's
// loadToken, the Codeberg client's loadToken and loadForgejoToken, and the
// CLI release pipeline's loadTokenWithFallback, with subtly different
// trimming behavior between the copies (task g01 consolidated them here,
// building on the shared ReadProtectedTokenFile extracted for task 901).
//
// Precedence, matching every caller's prior behavior:
//  1. configToken, if non-empty (e.g. a value from gitsyncer's config file).
//  2. The environment variable named envVar, if set and non-empty.
//  3. A protected token file named tokenFileName under the user's home
//     directory, read via ReadProtectedTokenFile.
//
// configToken and the environment variable are both trimmed here for
// consistency with the token file (ReadProtectedTokenFile already trims its
// result), since tokens are commonly copy-pasted or piped from a secret store
// with a trailing newline.
//
// Returns "" if none of the three sources yield a non-empty token, including
// when the home directory cannot be determined or the token file cannot be
// read (missing, wrong permissions, etc.) - callers treat "" as "no token
// configured" rather than a hard error, exactly as before consolidation.
func ResolveToken(configToken, envVar, tokenFileName string) string {
	if token := strings.TrimSpace(configToken); token != "" {
		return token
	}
	if token := strings.TrimSpace(os.Getenv(envVar)); token != "" {
		return token
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	token, err := ReadProtectedTokenFile(filepath.Join(home, tokenFileName))
	if err != nil {
		return ""
	}
	return token
}
