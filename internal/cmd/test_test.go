package cmd

import (
	"testing"

	"github.com/snonux/gitsyncer/internal/config"
)

// isolateFromRealTokenFiles points HOME at an empty temp dir so
// forge.ResolveToken's file-fallback step can't see the real
// ~/.gitsyncer_github_token / ~/.gitsyncer_codeberg_token on the machine
// running the test.
func isolateFromRealTokenFiles(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func TestConfigTokenWarnings_NoOrganizations(t *testing.T) {
	isolateFromRealTokenFiles(t)

	warnings := configTokenWarnings(&config.Config{})

	if len(warnings) != 1 || warnings[0] != "Warning: No GitHub or Codeberg organizations configured" {
		t.Fatalf("configTokenWarnings() = %v, want a single no-orgs warning", warnings)
	}
}

func TestConfigTokenWarnings_TokenResolvesFromEnvVar(t *testing.T) {
	// Regression test for the bug this fix addresses: previously the check
	// only looked at org.GitHubToken/org.CodebergToken (the inline config
	// field) and always warned when a setup intentionally left it empty in
	// favor of an env var or token file - the recommended, secure way to
	// configure tokens.
	isolateFromRealTokenFiles(t)
	t.Setenv("GITHUB_TOKEN", "gh-token-from-env")
	t.Setenv("CODEBERG_TOKEN", "cb-token-from-env")

	cfg := &config.Config{Organizations: []config.Organization{
		{Host: "git@github.com", Name: "acme"},
		{Host: "git@codeberg.org", Name: "acme"},
	}}

	if warnings := configTokenWarnings(cfg); len(warnings) != 0 {
		t.Fatalf("configTokenWarnings() = %v, want no warnings when tokens resolve via env vars", warnings)
	}
}

func TestConfigTokenWarnings_TokenResolvesFromInlineConfig(t *testing.T) {
	isolateFromRealTokenFiles(t)

	cfg := &config.Config{Organizations: []config.Organization{
		{Host: "git@github.com", Name: "acme", GitHubToken: "gh-inline"},
		{Host: "git@codeberg.org", Name: "acme", CodebergToken: "cb-inline"},
	}}

	if warnings := configTokenWarnings(cfg); len(warnings) != 0 {
		t.Fatalf("configTokenWarnings() = %v, want no warnings when tokens are inlined in config", warnings)
	}
}

func TestConfigTokenWarnings_WarnsWhenTokenTrulyMissing(t *testing.T) {
	isolateFromRealTokenFiles(t)

	cfg := &config.Config{Organizations: []config.Organization{
		{Host: "git@github.com", Name: "acme"},
		{Host: "git@codeberg.org", Name: "acme"},
	}}

	warnings := configTokenWarnings(cfg)
	if len(warnings) != 2 {
		t.Fatalf("configTokenWarnings() = %v, want warnings for both GitHub and Codeberg", warnings)
	}
}
