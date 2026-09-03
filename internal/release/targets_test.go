package release

import (
	"testing"

	"github.com/snonux/gitsyncer/internal/config"
	"github.com/snonux/gitsyncer/internal/forge"
)

func TestBuildReleaseTargets_IncludesForgejo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FORGEJO_TOKEN", "forgejo-test-token")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("CODEBERG_TOKEN", "")

	cfg := &config.Config{
		Organizations: []config.Organization{
			{
				Host:           "ssh://git@code.example:2022",
				ForgejoAPIBase: "https://code.example/api/v1",
				ForgejoOwner:   "snonux",
				Optional:       true,
			},
		},
	}

	targets := BuildReleaseTargets(cfg)
	var forgejo *Target
	for i := range targets {
		if targets[i].Name == "Forgejo" {
			forgejo = &targets[i]
			break
		}
	}
	if forgejo == nil {
		t.Fatalf("BuildReleaseTargets() missing Forgejo target; got %#v", targets)
	}
	if forgejo.Owner != "snonux" {
		t.Fatalf("Forgejo Owner = %q, want snonux", forgejo.Owner)
	}
	if forgejo.SyncRepoRequired {
		t.Fatal("Forgejo SyncRepoRequired = true, want false (no allowlist gate)")
	}
	if _, ok := forgejo.Client.(forge.ReleasesEnabler); !ok {
		t.Fatal("Forgejo client should implement ReleasesEnabler")
	}
}

func TestBuildReleaseTargets_OmitsForgejoWhenUnconfigured(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("FORGEJO_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("CODEBERG_TOKEN", "")

	cfg := &config.Config{
		Organizations: []config.Organization{
			{Host: "git@github.com", Name: "acme"},
		},
	}

	for _, target := range BuildReleaseTargets(cfg) {
		if target.Name == "Forgejo" {
			t.Fatalf("unexpected Forgejo target: %#v", target)
		}
	}
}
