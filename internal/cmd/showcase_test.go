package cmd

import (
	"strings"
	"testing"
)

func TestShowcaseCommand_FlagsMatchImplementedBehavior(t *testing.T) {
	t.Parallel()

	if showcaseCmd.Flags().Lookup("output") != nil {
		t.Fatal("showcase command should not expose unsupported --output flag")
	}
	if showcaseCmd.Flags().Lookup("format") != nil {
		t.Fatal("showcase command should not expose unsupported --format flag")
	}
	if showcaseCmd.Flags().Lookup("exclude") != nil {
		t.Fatal("showcase command should not expose unsupported --exclude flag")
	}

	if showcaseCmd.Flags().Lookup("force") == nil {
		t.Fatal("showcase command must expose --force")
	}
	if showcaseCmd.Flags().Lookup("ai-tool") == nil {
		t.Fatal("showcase command must expose --ai-tool")
	}
	if showcaseCmd.Flags().Lookup("repo") == nil {
		t.Fatal("showcase command must expose --repo")
	}
}

func TestShowcaseCommand_ExamplesDoNotMentionUnsupportedFlags(t *testing.T) {
	t.Parallel()

	example := showcaseCmd.Example
	if strings.Contains(example, "--output") {
		t.Fatal("showcase example should not mention unsupported --output")
	}
	if strings.Contains(example, "--format") {
		t.Fatal("showcase example should not mention unsupported --format")
	}
	if strings.Contains(example, "--exclude") {
		t.Fatal("showcase example should not mention unsupported --exclude")
	}
}
