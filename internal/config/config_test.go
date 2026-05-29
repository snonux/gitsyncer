package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate_ShowcaseStatsBranchesRejectsEmptyBranch(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Organizations: []Organization{
			{Host: "git@github.com", Name: "test-user"},
		},
		ShowcaseStatsBranches: map[string]string{
			"foo.zone": "   ",
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want branch validation error")
	}
	if !strings.Contains(err.Error(), "showcase_stats_branches") {
		t.Fatalf("Validate() error = %q, want showcase_stats_branches context", err)
	}
}

func TestValidate_DescriptionSyncFieldsMustBePaired(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Organizations: []Organization{
			{
				Host:                "ssh://git@example.com/repos",
				BackupLocation:      true,
				DescriptionSyncHost: "root@example.com",
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want description sync validation error")
	}
	if !strings.Contains(err.Error(), "descriptionSyncHost") {
		t.Fatalf("Validate() error = %q, want descriptionSyncHost context", err)
	}
}

func TestFindOrganization_ReturnsPointerToStoredElement(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Organizations: []Organization{
			{Host: "git@github.com", Name: "before"},
		},
	}

	org := cfg.FindOrganization("git@github.com")
	if org == nil {
		t.Fatal("FindOrganization() returned nil, want organization")
	}

	org.Name = "after"

	if cfg.Organizations[0].Name != "after" {
		t.Fatalf("Organizations[0].Name = %q, want %q", cfg.Organizations[0].Name, "after")
	}
}

func TestGetShowcaseOutputDir_Default(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	cfg := &Config{}
	got, err := cfg.GetShowcaseOutputDir()
	if err != nil {
		t.Fatalf("GetShowcaseOutputDir() error = %v", err)
	}

	want := filepath.Join(homeDir, "git", "foo.zone-content", "gemtext", "about")
	if got != want {
		t.Fatalf("GetShowcaseOutputDir() = %q, want %q", got, want)
	}
}

func TestGetShowcaseOutputDir_OverrideAndExpandHome(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	cfg := &Config{
		ShowcaseOutputDir: "~/custom/showcase",
	}
	got, err := cfg.GetShowcaseOutputDir()
	if err != nil {
		t.Fatalf("GetShowcaseOutputDir() error = %v", err)
	}

	want := filepath.Join(homeDir, "custom", "showcase")
	if got != want {
		t.Fatalf("GetShowcaseOutputDir() = %q, want %q", got, want)
	}
}

func TestGetShowcaseCgitHost_DefaultAndOverride(t *testing.T) {
	t.Parallel()

	defaultCfg := &Config{}
	if got := defaultCfg.GetShowcaseCgitHost(); got != "https://cgit.f3s.buetow.org" {
		t.Fatalf("GetShowcaseCgitHost() default = %q, want %q", got, "https://cgit.f3s.buetow.org")
	}

	customCfg := &Config{ShowcaseCgitHost: "https://example.test/cgit/"}
	if got := customCfg.GetShowcaseCgitHost(); got != "https://example.test/cgit" {
		t.Fatalf("GetShowcaseCgitHost() override = %q, want %q", got, "https://example.test/cgit")
	}
}
