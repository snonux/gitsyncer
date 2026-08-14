package release

import (
	"os/exec"
	"reflect"
	"testing"

	"codeberg.org/snonux/gitsyncer/internal/aitool"
)

func TestAvailableReleaseNotesTools_DefaultChainWithFallback(t *testing.T) {
	t.Parallel()

	gen := NewNotesGenerator("", nil)
	got := gen.availableReleaseNotesTools(fakeLookPathRelease("ollama", "claude"))
	want := []aitool.Tool{aitool.ToolOpencode, aitool.ToolClaude}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("availableReleaseNotesTools() = %#v, want %#v", got, want)
	}
}

func TestAvailableReleaseNotesTools_HonorsConfiguredPreferenceWithFallback(t *testing.T) {
	t.Parallel()

	gen := NewNotesGenerator("hexai", nil)

	got := gen.availableReleaseNotesTools(fakeLookPathRelease("claude", "amp"))
	want := []aitool.Tool{aitool.ToolClaude, aitool.ToolAmp}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("availableReleaseNotesTools() = %#v, want %#v", got, want)
	}
}

func TestAvailableReleaseNotesTools_AmpChainOnly(t *testing.T) {
	t.Parallel()

	gen := NewNotesGenerator("amp", nil)

	got := gen.availableReleaseNotesTools(fakeLookPathRelease("ollama", "amp"))
	want := []aitool.Tool{aitool.ToolAmp}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("availableReleaseNotesTools() = %#v, want %#v", got, want)
	}
}

func fakeLookPathRelease(tools ...string) func(string) (string, error) {
	available := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		available[tool] = struct{}{}
	}

	return func(file string) (string, error) {
		if _, ok := available[file]; ok {
			return "/usr/bin/" + file, nil
		}

		return "", exec.ErrNotFound
	}
}
