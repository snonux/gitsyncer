package cmd

import (
	"testing"

	"github.com/snonux/gitsyncer/internal/config"
)

func isolateFromRealTokenFiles(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func TestConfigTokenWarnings_NoOrganizations(t *testing.T) {
	isolateFromRealTokenFiles(t)
	warnings := configTokenWarnings(&config.Config{})
	if len(warnings) != 1 || warnings[0] != "Warning: No GitHub or Forgejo organizations configured" {
		t.Fatalf("configTokenWarnings() = %v, want a single no-orgs warning", warnings)
	}
}

func TestConfigTokenWarnings_TokenResolvesFromEnvVar(t *testing.T) {
	isolateFromRealTokenFiles(t)
	t.Setenv("GITHUB_TOKEN", "gh-token-from-env")
	t.Setenv("FORGEJO_TOKEN", "forgejo-token-from-env")

	cfg := &config.Config{Organizations: []config.Organization{
		{Host: "git@github.com", Name: "acme"},
		{Host: "ssh://git@forgejo.example:2022", ForgejoAPIBase: "https://forgejo.example/api/v1", ForgejoOwner: "acme"},
	}}
	if warnings := configTokenWarnings(cfg); len(warnings) != 0 {
		t.Fatalf("configTokenWarnings() = %v, want no warnings when tokens resolve via env vars", warnings)
	}
}

func TestConfigTokenWarnings_TokenResolvesFromInlineConfig(t *testing.T) {
	isolateFromRealTokenFiles(t)
	cfg := &config.Config{Organizations: []config.Organization{{
		Host: "git@github.com", Name: "acme", GitHubToken: "gh-inline",
	}}}
	if warnings := configTokenWarnings(cfg); len(warnings) != 0 {
		t.Fatalf("configTokenWarnings() = %v, want no warnings when tokens are inlined", warnings)
	}
}

func TestConfigTokenWarnings_WarnsWhenTokenTrulyMissing(t *testing.T) {
	isolateFromRealTokenFiles(t)
	cfg := &config.Config{Organizations: []config.Organization{
		{Host: "git@github.com", Name: "acme"},
		{Host: "ssh://git@forgejo.example:2022", ForgejoAPIBase: "https://forgejo.example/api/v1", ForgejoOwner: "acme"},
	}}
	warnings := configTokenWarnings(cfg)
	if len(warnings) != 2 {
		t.Fatalf("configTokenWarnings() = %v, want warnings for GitHub and Forgejo", warnings)
	}
}
