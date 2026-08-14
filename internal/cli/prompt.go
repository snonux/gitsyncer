package cli

import (
	"fmt"
	"strings"
)

// promptConfirmation asks for user confirmation on the console. An empty or
// non-yes response declines. The fmt.Scanln error is intentionally discarded:
// Scanln returns an error for a bare newline or closed stdin, either of which
// leaves the response empty and is already treated as a decline below, so
// there is nothing extra to do with the error.
func promptConfirmation(message string) bool {
	fmt.Printf("%s [y/N]: ", message)

	var response string
	_, _ = fmt.Scanln(&response)

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}
