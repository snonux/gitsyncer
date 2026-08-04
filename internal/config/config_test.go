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

func TestValidate_ForcePushRequiresBackupLocation(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Organizations: []Organization{
			{Host: "git@github.com", Name: "test-user", ForcePush: true},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want forcePush validation error")
	}
	if !strings.Contains(err.Error(), "forcePush requires backupLocation") {
		t.Fatalf("Validate() error = %q, want forcePush context", err)
	}
}

func TestValidate_ForgejoMustBeBackupOnly(t *testing.T) {
	t.Parallel()

	cfg := &Config{Organizations: []Organization{{
		Host:           "ssh://git@forgejo.example:2022",
		ForgejoAPIBase: "https://forgejo.example/api/v1",
		ForgejoOwner:   "snonux",
	}}}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "must set backupLocation") {
		t.Fatalf("Validate() error = %v, want backup-only validation", err)
	}
}

func TestValidate_ForgejoURLs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		host    string
		apiBase string
		want    string
	}{
		{name: "scp-like host", host: "git@forgejo.example:repos", apiBase: "https://forgejo.example/api/v1", want: "absolute ssh://"},
		{name: "HTTP Git host", host: "https://forgejo.example", apiBase: "https://forgejo.example/api/v1", want: "absolute ssh://"},
		{name: "SSH host path", host: "ssh://git@forgejo.example:2022/owner", apiBase: "https://forgejo.example/api/v1", want: "absolute ssh://"},
		{name: "relative API base", host: "ssh://git@forgejo.example:2022", apiBase: "/api/v1", want: "absolute HTTP(S)"},
		{name: "non-HTTP API base", host: "ssh://git@forgejo.example:2022", apiBase: "ftp://forgejo.example/api/v1", want: "absolute HTTP(S)"},
		{name: "API base userinfo", host: "ssh://git@forgejo.example:2022", apiBase: "https://user@forgejo.example/api/v1", want: "absolute HTTP(S)"},
		{name: "API base query", host: "ssh://git@forgejo.example:2022", apiBase: "https://forgejo.example/api/v1?token=bad", want: "absolute HTTP(S)"},
		{name: "API base fragment", host: "ssh://git@forgejo.example:2022", apiBase: "https://forgejo.example/api/v1#bad", want: "absolute HTTP(S)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Organizations: []Organization{{
				Host: tt.host, ForgejoAPIBase: tt.apiBase, ForgejoOwner: "owner", BackupLocation: true,
			}}}
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestForgejoToken_UsesEnvironmentOnly(t *testing.T) {
	t.Setenv("FORGEJO_TOKEN", " protected-token ")

	org := Organization{ForgejoAPIBase: "https://forgejo.example/api/v1"}
	if got := org.ForgejoToken(); got != "protected-token" {
		t.Fatalf("ForgejoToken() = %q, want protected environment token", got)
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

func TestCodebergSyncEnabled(t *testing.T) {
	t.Parallel()

	if (&Config{}).CodebergSyncEnabled() {
		t.Fatal("CodebergSyncEnabled() = true for zero-value Config, want false")
	}
	if (&Config{SyncCodeberg: true}).CodebergSyncEnabled() != true {
		t.Fatal("CodebergSyncEnabled() = false for SyncCodeberg:true, want true")
	}
	if (*Config)(nil).CodebergSyncEnabled() {
		t.Fatal("CodebergSyncEnabled() = true for nil Config, want false")
	}
}

func TestSyncOrganizations_ExcludesCodebergUnlessEnabled(t *testing.T) {
	t.Parallel()

	orgs := []Organization{
		{Host: "git@github.com", Name: "acme"},
		{Host: "git@codeberg.org", Name: "acme"},
		{Host: "user@nas.local:git", BackupLocation: true},
	}

	// Codeberg syncing disabled (default): Codeberg org is excluded, but
	// GitHub and backup locations are still returned.
	disabled := &Config{Organizations: orgs}
	got := disabled.SyncOrganizations()
	if len(got) != 2 {
		t.Fatalf("disabled SyncOrganizations() = %d orgs, want 2", len(got))
	}
	for _, o := range got {
		if o.IsCodeberg() {
			t.Fatalf("disabled SyncOrganizations() included Codeberg org %q", o.Host)
		}
	}

	// Codeberg syncing enabled: every organization is returned.
	enabled := &Config{Organizations: orgs, SyncCodeberg: true}
	got = enabled.SyncOrganizations()
	if len(got) != 3 {
		t.Fatalf("enabled SyncOrganizations() = %d orgs, want 3", len(got))
	}

	// The original Organizations slice must be left untouched.
	if len(disabled.Organizations) != 3 {
		t.Fatalf("Organizations mutated by SyncOrganizations(): len=%d", len(disabled.Organizations))
	}
}

func TestFilterSyncRepos_Allowlist(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Repositories: []string{"foo", "bar", "baz"},
	}
	got := cfg.FilterSyncRepos([]string{"foo", "extra", "bar", "nope"})
	if len(got) != 2 {
		t.Fatalf("FilterSyncRepos() = %v, want 2 entries", got)
	}
	for _, r := range got {
		if r != "foo" && r != "bar" {
			t.Fatalf("FilterSyncRepos() returned unexpected repo %q", r)
		}
	}
}

func TestFilterSyncRepos_DiscoveryModeWhenEmpty(t *testing.T) {
	t.Parallel()

	cfg := &Config{Repositories: nil}
	discovered := []string{"foo", "bar", "baz"}
	got := cfg.FilterSyncRepos(discovered)
	if len(got) != len(discovered) {
		t.Fatalf("FilterSyncRepos() with empty allowlist = %v, want all discovered", got)
	}

	var nilCfg *Config
	if got := nilCfg.FilterSyncRepos(discovered); len(got) != len(discovered) {
		t.Fatalf("nil FilterSyncRepos() = %v, want all discovered", got)
	}
}

func TestIsSyncRepo(t *testing.T) {
	t.Parallel()

	allowlist := &Config{Repositories: []string{"foo", "bar"}}
	if !allowlist.IsSyncRepo("foo") {
		t.Fatal("IsSyncRepo(foo) = false, want true")
	}
	if allowlist.IsSyncRepo("baz") {
		t.Fatal("IsSyncRepo(baz) = true, want false (not in allowlist)")
	}

	// Discovery mode (empty Repositories): every repo is a sync repo.
	discovery := &Config{}
	if !discovery.IsSyncRepo("anything") {
		t.Fatal("IsSyncRepo(anything) = false in discovery mode, want true")
	}

	var nilCfg *Config
	if nilCfg.IsSyncRepo("foo") {
		t.Fatal("nil IsSyncRepo() = true, want false")
	}
}
