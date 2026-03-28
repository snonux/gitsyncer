package config

import (
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
