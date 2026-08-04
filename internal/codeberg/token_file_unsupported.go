//go:build !unix

package codeberg

import (
	"fmt"
	"os"
)

func openProtectedTokenFile(string) (*os.File, error) {
	return nil, fmt.Errorf("secure token files are unsupported on this platform")
}
