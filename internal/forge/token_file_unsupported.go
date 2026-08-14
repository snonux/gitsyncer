//go:build !unix

package forge

import (
	"fmt"
	"os"
)

// openProtectedTokenFile has no hardened implementation on non-unix
// platforms (no O_NOFOLLOW/O_NONBLOCK support), so it fails closed rather
// than silently falling back to an unprotected open.
func openProtectedTokenFile(string) (*os.File, error) {
	return nil, fmt.Errorf("secure token files are unsupported on this platform")
}
