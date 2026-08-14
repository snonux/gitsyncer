package aitool

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// withFakeBinary puts an executable shell script named name on PATH for the
// duration of the test, so Runner implementations (which hardcode binary
// names like "claude" or "amp") can be exercised without depending on
// whether those real CLIs happen to be installed on the host running the
// tests.
func withFakeBinary(t *testing.T, name, script string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("fake binary shell scripts require a POSIX shell")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRunners_SuccessReturnsTrimmedOutput(t *testing.T) {
	tests := []struct {
		name   string
		binary string
		tool   Tool
	}{
		{"opencode", "ollama", ToolOpencode},
		{"hexai", "hexai", ToolHexAI},
		{"claude", "claude", ToolClaude},
		{"amp", "amp", ToolAmp},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFakeBinary(t, tt.binary, "echo '  padded output  '")

			got, err := NewRunner(tt.tool, "").Run("prompt", "stdin")
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got != "padded output" {
				t.Fatalf("Run() = %q, want %q", got, "padded output")
			}
		})
	}
}

func TestRunners_NonZeroExitIsError(t *testing.T) {
	withFakeBinary(t, "claude", "echo boom >&2; exit 1")

	_, err := NewRunner(ToolClaude, "").Run("prompt", "")
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
}

func TestRunners_EmptyOutputIsError(t *testing.T) {
	withFakeBinary(t, "amp", "true")

	_, err := NewRunner(ToolAmp, "").Run("prompt", "")
	if err == nil {
		t.Fatal("expected error for empty output")
	}
}

// TestRunners_StderrNoiseNotCapturedInOutput is a regression test for the
// bug where CLAUDE_DEBUG=1 debug logging on stderr got merged (via
// CombinedOutput) into the captured "output" and ended up published as
// release notes. The fake claude script writes debug noise to stderr and
// clean output to stdout; Run() must return only the stdout content.
func TestRunners_StderrNoiseNotCapturedInOutput(t *testing.T) {
	withFakeBinary(t, "claude", "echo '[DEBUG] noisy diagnostic line' >&2\necho 'clean release notes'")

	got, err := NewRunner(ToolClaude, "").Run("prompt", "")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got != "clean release notes" {
		t.Fatalf("Run() = %q, want %q (stderr debug noise must not be captured)", got, "clean release notes")
	}
	if strings.Contains(got, "DEBUG") {
		t.Fatalf("Run() = %q, contains stderr debug noise", got)
	}
}

func TestRunners_InBandErrorMessageIsError(t *testing.T) {
	withFakeBinary(t, "hexai", "echo 'Error: something went wrong'")

	_, err := NewRunner(ToolHexAI, "").Run("prompt", "")
	if err == nil {
		t.Fatal("expected error for in-band error message")
	}
}

func TestNewRunner_UnknownToolReturnsNil(t *testing.T) {
	t.Parallel()

	if got := NewRunner(Tool("bogus"), ""); got != nil {
		t.Fatalf("NewRunner() = %#v, want nil", got)
	}
}

func TestRunChain_StopsAtFirstSuccess(t *testing.T) {
	withFakeBinary(t, "ollama", "exit 1") // opencode fails
	withFakeBinary(t, "hexai", "echo used-hexai")
	withFakeBinary(t, "claude", "echo used-claude")

	chain := []Tool{ToolOpencode, ToolHexAI, ToolClaude}
	got, used, err := RunChain(chain, "", "prompt", "")
	if err != nil {
		t.Fatalf("RunChain() error = %v", err)
	}
	if used != ToolHexAI {
		t.Fatalf("RunChain() used = %q, want %q", used, ToolHexAI)
	}
	if got != "used-hexai" {
		t.Fatalf("RunChain() output = %q, want %q", got, "used-hexai")
	}
}

func TestRunChain_AllFailReturnsError(t *testing.T) {
	t.Parallel()

	_, _, err := RunChain(nil, "", "prompt", "")
	if err == nil {
		t.Fatal("expected error when chain is empty")
	}
}

func TestCombinedPrompt(t *testing.T) {
	t.Parallel()

	if got := combinedPrompt("p", ""); got != "p" {
		t.Fatalf("combinedPrompt() = %q, want %q", got, "p")
	}

	if got := combinedPrompt("p", "s"); got != "p\n\ns" {
		t.Fatalf("combinedPrompt() = %q, want %q", got, "p\n\ns")
	}
}

// Sanity-check that the registry covers exactly the tools Chain() can ever
// produce, so RunChain never silently no-ops on an entry NewRunner can't
// build.
func TestRegistry_CoversAllChainTools(t *testing.T) {
	t.Parallel()

	all := map[Tool]bool{}
	for _, preferred := range []string{"", "hexai", "claude", "amp"} {
		for _, tool := range Chain(preferred) {
			all[tool] = true
		}
	}

	for tool := range all {
		if NewRunner(tool, "") == nil {
			t.Fatalf("no runner registered for %q", tool)
		}
	}

	if !reflect.DeepEqual(map[Tool]bool{ToolOpencode: true, ToolHexAI: true, ToolClaude: true, ToolAmp: true}, all) {
		t.Fatalf("unexpected tool set from Chain(): %#v", all)
	}
}
