package aitool

import (
	"fmt"
	"reflect"
	"testing"
)

func TestChain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		preferred string
		want      []Tool
	}{
		{
			name: "default chain when empty",
			want: []Tool{ToolPi, ToolOpencode, ToolHexAI, ToolClaude, ToolAmp},
		},
		{
			name:      "default chain when pi",
			preferred: "pi",
			want:      []Tool{ToolPi, ToolOpencode, ToolHexAI, ToolClaude, ToolAmp},
		},
		{
			name:      "openrouter alias uses pi chain",
			preferred: "openrouter",
			want:      []Tool{ToolPi, ToolOpencode, ToolHexAI, ToolClaude, ToolAmp},
		},
		{
			name:      "opencode chain",
			preferred: "opencode",
			want:      []Tool{ToolOpencode, ToolHexAI, ToolClaude, ToolAmp},
		},
		{
			name:      "hexai chain",
			preferred: "hexai",
			want:      []Tool{ToolHexAI, ToolClaude, ToolAmp},
		},
		{
			name:      "claude alias chain",
			preferred: "claude-code",
			want:      []Tool{ToolClaude, ToolAmp},
		},
		{
			name:      "amp only",
			preferred: "amp",
			want:      []Tool{ToolAmp},
		},
		{
			name:      "unknown tool",
			preferred: "unknown",
			want:      nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Chain(tt.preferred)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Chain(%q) = %#v, want %#v", tt.preferred, got, tt.want)
			}
		})
	}
}

func TestFirstAvailable(t *testing.T) {
	t.Parallel()

	lookPath := fakeLookPath("claude", "amp")
	got := FirstAvailable("", lookPath)
	if got != ToolClaude {
		t.Fatalf("FirstAvailable() = %q, want %q", got, ToolClaude)
	}
}

func TestFirstAvailable_NoToolsFound(t *testing.T) {
	t.Parallel()

	got := FirstAvailable("", fakeLookPath())
	if got != "" {
		t.Fatalf("FirstAvailable() = %q, want empty", got)
	}
}

func TestIsAvailable_OpencodeUsesOllamaBinary(t *testing.T) {
	t.Parallel()

	if !IsAvailable(ToolOpencode, fakeLookPath("ollama")) {
		t.Fatal("expected opencode to be available when ollama exists")
	}

	if IsAvailable(ToolOpencode, fakeLookPath("opencode")) {
		t.Fatal("expected opencode to be unavailable when only opencode binary exists")
	}
}

func TestIsAvailable_PiUsesPiBinary(t *testing.T) {
	t.Parallel()

	if !IsAvailable(ToolPi, fakeLookPath("pi")) {
		t.Fatal("expected pi to be available when pi exists")
	}

	if IsAvailable(ToolPi, fakeLookPath("ollama")) {
		t.Fatal("expected pi to be unavailable when only ollama exists")
	}
}

func TestAvailableChain_FiltersUnavailableTools(t *testing.T) {
	t.Parallel()

	got := AvailableChain("", fakeLookPath("hexai", "amp"))
	want := []Tool{ToolHexAI, ToolAmp}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AvailableChain() = %#v, want %#v", got, want)
	}
}

func fakeLookPath(tools ...string) LookPathFunc {
	available := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		available[tool] = struct{}{}
	}

	return func(file string) (string, error) {
		if _, ok := available[file]; ok {
			return "/usr/bin/" + file, nil
		}
		return "", fmt.Errorf("%s not found", file)
	}
}
