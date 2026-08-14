package cli

import (
	"os"
	"testing"
)

// withStdin temporarily replaces os.Stdin with r for the duration of fn, then
// restores the original value. Not run in parallel with other tests since
// os.Stdin is a shared global.
func withStdin(t *testing.T, r *os.File, fn func()) {
	t.Helper()

	original := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = original }()

	fn()
}

// TestPromptConfirmation_EmptyInputDeclines verifies that when Scanln
// returns an error (e.g. a bare newline, which fmt.Scanln reports as
// "unexpected newline"), promptConfirmation still safely defaults to
// declining rather than panicking or hanging - this is the discarded-error
// behavior that promptConfirmation's fmt.Scanln call relies on.
func TestPromptConfirmation_EmptyInputDeclines(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer func() { _ = r.Close() }()

	if _, err := w.WriteString("\n"); err != nil {
		t.Fatalf("failed to write to pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close pipe writer: %v", err)
	}

	var got bool
	withStdin(t, r, func() {
		got = promptConfirmation("Proceed?")
	})

	if got {
		t.Fatal("promptConfirmation() = true for empty input, want false")
	}
}

// TestPromptConfirmation_YesInputConfirms is a regression check that the
// happy path (explicit "y") still works after touching the Scanln call.
func TestPromptConfirmation_YesInputConfirms(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer func() { _ = r.Close() }()

	if _, err := w.WriteString("y\n"); err != nil {
		t.Fatalf("failed to write to pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close pipe writer: %v", err)
	}

	var got bool
	withStdin(t, r, func() {
		got = promptConfirmation("Proceed?")
	})

	if !got {
		t.Fatal("promptConfirmation() = false for \"y\" input, want true")
	}
}
