package config

import (
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/snonux/gitsyncer/internal/forge"
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

func TestSyncBranches_DefaultGlobalAndRepositoryOverrides(t *testing.T) {
	t.Parallel()

	defaults := (&Config{}).SyncBranches("demo")
	if want := []string{"main", "master", "develop"}; !reflect.DeepEqual(defaults, want) {
		t.Fatalf("SyncBranches() = %v, want defaults %v", defaults, want)
	}

	cfg := &Config{
		Branches: []string{"main", "release"},
		RepositoryBranches: map[string][]string{
			"demo": {"trunk", "stable"},
		},
	}
	if got, want := cfg.SyncBranches("other"), []string{"main", "release"}; !reflect.DeepEqual(got, want) {
		t.Errorf("SyncBranches(other) = %v, want global %v", got, want)
	}
	if got, want := cfg.SyncBranches("demo"), []string{"trunk", "stable"}; !reflect.DeepEqual(got, want) {
		t.Errorf("SyncBranches(demo) = %v, want repository override %v", got, want)
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

func TestValidate_ForgejoIsBidirectionalByDefault(t *testing.T) {
	t.Parallel()

	cfg := &Config{Organizations: []Organization{{
		Host:           "ssh://git@forgejo.example:2022",
		ForgejoAPIBase: "https://forgejo.example/api/v1",
		ForgejoOwner:   "snonux",
	}}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil for a default bidirectional Forgejo peer", err)
	}
	if cfg.Organizations[0].IsWriteOnly() {
		t.Fatal("default Forgejo organization must not be write-only")
	}
}

// forgejoPushOnlyOrg is a one-way Forgejo mirror as used in the tests below.
func forgejoPushOnlyOrg() Organization {
	return Organization{
		Host:           "ssh://git@forgejo.example:2022",
		ForgejoAPIBase: "https://forgejo.example/api/v1",
		ForgejoOwner:   "snonux",
		PushOnly:       true,
	}
}

func TestValidate_ForgejoMayBePushOnlyMirror(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		mutate func(*Organization)
	}{
		{name: "pushOnly alone", mutate: func(*Organization) {}},
		{name: "pushOnly and optional", mutate: func(o *Organization) { o.Optional = true }},
		{name: "pushOnly with forcePush", mutate: func(o *Organization) { o.ForcePush = true }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			org := forgejoPushOnlyOrg()
			tt.mutate(&org)
			cfg := &Config{Organizations: []Organization{org}}
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestValidate_BackupLocationAndPushOnlyAreMutuallyExclusive(t *testing.T) {
	t.Parallel()

	org := forgejoPushOnlyOrg()
	org.BackupLocation = true
	cfg := &Config{Organizations: []Organization{org}}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "backupLocation and pushOnly are mutually exclusive") {
		t.Fatalf("Validate() error = %v, want mutual-exclusion validation", err)
	}
}

func TestIsWriteOnly(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		org  Organization
		want bool
	}{
		{name: "regular peer", org: Organization{}, want: false},
		{name: "optional peer", org: Organization{Optional: true}, want: false},
		{name: "backup", org: Organization{BackupLocation: true}, want: true},
		{name: "pushOnly", org: Organization{PushOnly: true}, want: true},
	} {
		if got := tt.org.IsWriteOnly(); got != tt.want {
			t.Errorf("%s: IsWriteOnly() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestMirrorsAllGitHubRepos(t *testing.T) {
	t.Parallel()

	var nilCfg *Config
	if nilCfg.MirrorsAllGitHubRepos() {
		t.Fatal("nil config must not mirror all repos")
	}
	bidirectional := &Config{Organizations: []Organization{
		{Host: "git@github.com", Name: "acme"},
		{Host: "ssh://git@forgejo.example:2022", Optional: true},
		{Host: "paul@nas:/git", BackupLocation: true},
	}}
	if bidirectional.MirrorsAllGitHubRepos() {
		t.Fatal("config without a pushOnly org must not mirror all repos")
	}
	mirrored := &Config{Organizations: []Organization{
		{Host: "git@github.com", Name: "acme"},
		forgejoPushOnlyOrg(),
	}}
	if !mirrored.MirrorsAllGitHubRepos() {
		t.Fatal("config with a pushOnly org must mirror all repos")
	}
}

func TestValidate_ForgejoMayBeOptionalBidirectionalPeer(t *testing.T) {
	t.Parallel()

	cfg := &Config{Organizations: []Organization{{
		Host:           "ssh://git@forgejo.example:2022",
		ForgejoAPIBase: "https://forgejo.example/api/v1",
		ForgejoOwner:   "snonux",
		Optional:       true,
	}}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidate_BackupAndOptionalAreMutuallyExclusive(t *testing.T) {
	t.Parallel()

	cfg := &Config{Organizations: []Organization{{
		Host:           "ssh://git@forgejo.example:2022",
		ForgejoAPIBase: "https://forgejo.example/api/v1",
		ForgejoOwner:   "snonux",
		BackupLocation: true,
		Optional:       true,
	}}}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("Validate() error = %v, want mutually-exclusive validation", err)
	}
}

func TestValidate_ForgejoOwnerType(t *testing.T) {
	t.Parallel()

	t.Run("defaults to user", func(t *testing.T) {
		cfg := &Config{Organizations: []Organization{{
			Host: "ssh://git@forgejo.example:2022", ForgejoAPIBase: "https://forgejo.example/api/v1",
			ForgejoOwner: "snonux", BackupLocation: true,
		}}}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
		if got := cfg.Organizations[0].ForgejoOwnerType; got != "" {
			t.Fatalf("ForgejoOwnerType = %q, want omitted value retained", got)
		}
	})

	t.Run("accepts organization", func(t *testing.T) {
		cfg := &Config{Organizations: []Organization{{
			Host: "ssh://git@forgejo.example:2022", ForgejoAPIBase: "https://forgejo.example/api/v1",
			ForgejoOwner: "snonux", ForgejoOwnerType: forge.OwnerTypeOrganization, BackupLocation: true,
		}}}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})

	t.Run("rejects invalid value", func(t *testing.T) {
		cfg := &Config{Organizations: []Organization{{
			Host: "ssh://git@forgejo.example:2022", ForgejoAPIBase: "https://forgejo.example/api/v1",
			ForgejoOwner: "snonux", ForgejoOwnerType: "team", BackupLocation: true,
		}}}
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), "forgejo_owner_type") {
			t.Fatalf("Validate() error = %v, want owner type validation", err)
		}
	})
}

func TestValidate_ForgejoURLs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		host    string
		apiBase string
		owner   string
		want    string
	}{
		{name: "scp-like host", host: "git@forgejo.example:repos", apiBase: "https://forgejo.example/api/v1", want: "forgejo host: parse"},
		{name: "HTTP Git host", host: "https://forgejo.example", apiBase: "https://forgejo.example/api/v1", want: "scheme must be ssh"},
		{name: "SSH missing hostname", host: "ssh://git@", apiBase: "https://forgejo.example/api/v1", want: "missing hostname"},
		{name: "SSH missing user", host: "ssh://forgejo.example:2022", apiBase: "https://forgejo.example/api/v1", want: "missing user"},
		{name: "SSH missing username", host: "ssh://@forgejo.example:2022", apiBase: "https://forgejo.example/api/v1", want: "missing username"},
		{name: "SSH host path", host: "ssh://git@forgejo.example:2022/owner", apiBase: "https://forgejo.example/api/v1", want: "path must be empty"},
		{name: "SSH host query", host: "ssh://git@forgejo.example:2022?x=1", apiBase: "https://forgejo.example/api/v1", want: "query string is not allowed"},
		{name: "SSH host fragment", host: "ssh://git@forgejo.example:2022#frag", apiBase: "https://forgejo.example/api/v1", want: "fragment is not allowed"},
		{name: "relative API base", host: "ssh://git@forgejo.example:2022", apiBase: "/api/v1", want: "absolute HTTP(S)"},
		{name: "non-HTTP API base", host: "ssh://git@forgejo.example:2022", apiBase: "ftp://forgejo.example/api/v1", want: "absolute HTTP(S)"},
		{name: "API base userinfo", host: "ssh://git@forgejo.example:2022", apiBase: "https://user@forgejo.example/api/v1", want: "absolute HTTP(S)"},
		{name: "API base query", host: "ssh://git@forgejo.example:2022", apiBase: "https://forgejo.example/api/v1?token=bad", want: "absolute HTTP(S)"},
		{name: "API base fragment", host: "ssh://git@forgejo.example:2022", apiBase: "https://forgejo.example/api/v1#bad", want: "absolute HTTP(S)"},
		{name: "SSH password", host: "ssh://git:secret@forgejo.example:2022", apiBase: "https://forgejo.example/api/v1", want: "user must not include a password"},
		{name: "nonnumeric SSH port", host: "ssh://git@forgejo.example:ssh", apiBase: "https://forgejo.example/api/v1", want: "forgejo host: parse"},
		{name: "zero SSH port", host: "ssh://git@forgejo.example:0", apiBase: "https://forgejo.example/api/v1", want: "invalid port"},
		{name: "out of range SSH port", host: "ssh://git@forgejo.example:65536", apiBase: "https://forgejo.example/api/v1", want: "invalid port"},
		{name: "owner slash", host: "ssh://git@forgejo.example:2022", apiBase: "https://forgejo.example/api/v1", owner: "group/owner", want: "safe path segment"},
		{name: "owner dot segment", host: "ssh://git@forgejo.example:2022", apiBase: "https://forgejo.example/api/v1", owner: "..", want: "safe path segment"},
		{name: "owner query trick", host: "ssh://git@forgejo.example:2022", apiBase: "https://forgejo.example/api/v1", owner: "owner?admin=1", want: "safe path segment"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner := tt.owner
			if owner == "" {
				owner = "owner"
			}
			cfg := &Config{Organizations: []Organization{{
				Host: tt.host, ForgejoAPIBase: tt.apiBase, ForgejoOwner: owner, BackupLocation: true,
			}}}
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

// TestValidateForgejoHost exercises the extracted helper directly, one
// sub-condition at a time, to confirm each failure mode reports its own
// distinct message rather than sharing one opaque "must be an absolute
// ssh:// URL" error.
func TestValidateForgejoHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string // substring expected in the error, or "" for success
	}{
		{name: "valid", raw: "ssh://git@forgejo.example:2022", want: ""},
		{name: "valid trailing slash", raw: "ssh://git@forgejo.example:2022/", want: ""},
		{name: "wrong scheme", raw: "https://git@forgejo.example:2022", want: "scheme must be ssh"},
		{name: "missing hostname", raw: "ssh://git@", want: "missing hostname"},
		{name: "missing user", raw: "ssh://forgejo.example:2022", want: "missing user"},
		{name: "missing username", raw: "ssh://@forgejo.example:2022", want: "missing username"},
		{name: "password in userinfo", raw: "ssh://git:secret@forgejo.example:2022", want: "must not include a password"},
		{name: "extra path segment", raw: "ssh://git@forgejo.example:2022/owner", want: "path must be empty"},
		{name: "query string", raw: "ssh://git@forgejo.example:2022?x=1", want: "query string is not allowed"},
		{name: "fragment", raw: "ssh://git@forgejo.example:2022#frag", want: "fragment is not allowed"},
		{name: "zero port", raw: "ssh://git@forgejo.example:0", want: "invalid port"},
		{name: "out of range port", raw: "ssh://git@forgejo.example:65536", want: "invalid port"},
	}

	seenMessages := map[string]string{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.raw)
			if err != nil {
				t.Fatalf("url.Parse(%q) failed: %v", tt.raw, err)
			}

			err = validateForgejoHost(u)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("validateForgejoHost(%q) = %v, want nil", tt.raw, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validateForgejoHost(%q) error = %v, want containing %q", tt.raw, err, tt.want)
			}
			// Each failure condition must produce a message distinct from
			// every other failure condition's message, otherwise callers
			// can't tell which check actually failed.
			if other, ok := seenMessages[err.Error()]; ok {
				t.Fatalf("validateForgejoHost(%q) error %q duplicates message from case %q", tt.raw, err.Error(), other)
			}
			seenMessages[err.Error()] = tt.name
		})
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

func TestFindForgejoOrg(t *testing.T) {
	t.Parallel()

	if got := (&Config{}).FindForgejoOrg(); got != nil {
		t.Fatalf("FindForgejoOrg() = %#v, want nil", got)
	}

	cfg := &Config{Organizations: []Organization{{Host: "git@github.com", Name: "owner"}, {
		Host: "ssh://git@code.example:2022", ForgejoAPIBase: "https://code.example/api/v1", ForgejoOwner: "forge-owner", BackupLocation: true,
	}}}
	if got := cfg.FindForgejoOrg(); got == nil || got.ForgejoOwner != "forge-owner" {
		t.Fatalf("FindForgejoOrg() = %#v, want Forgejo organization", got)
	}
}

func TestSyncOrganizationsReturnsConfiguredOrganizations(t *testing.T) {
	t.Parallel()

	orgs := []Organization{
		{Host: "git@github.com", Name: "acme"},
		{Host: "ssh://git@forgejo.example:2022", Name: "acme", ForgejoAPIBase: "https://forgejo.example/api/v1", ForgejoOwner: "acme", BackupLocation: true},
		{Host: "user@nas.local:git", BackupLocation: true},
	}
	cfg := &Config{Organizations: orgs}
	got := cfg.SyncOrganizations()
	if len(got) != len(orgs) {
		t.Fatalf("SyncOrganizations() = %d orgs, want %d", len(got), len(orgs))
	}
	if len(cfg.Organizations) != len(orgs) {
		t.Fatalf("Organizations mutated by SyncOrganizations(): len=%d", len(cfg.Organizations))
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
