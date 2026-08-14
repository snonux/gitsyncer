package forge

import (
	"fmt"
	"io"
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
