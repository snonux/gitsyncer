package cli

import "codeberg.org/snonux/gitsyncer/internal/state"

// Flags holds all command-line flag values
type Flags struct {
	VersionFlag         bool
	ConfigPath          string
	ListOrgs            bool
	ListRepos           bool
	SyncRepo            string
	SyncAll             bool
	SyncCodebergPublic  bool
	SyncGitHubPublic    bool
	FullSync            bool
	CreateGitHubRepos   bool
	CreateCodebergRepos bool
	DryRun              bool
	WorkDir             string
	TestGitHubToken     bool
	Clean               bool
	DeleteRepo          string
	Backup              bool
	Showcase            bool
	Force               bool
	BatchRun            bool
	CheckReleases       bool
	NoCheckReleases     bool
	AutoCreateReleases  bool
	AIReleaseNotes      bool
	UpdateReleases      bool
	AITool              string
	Throttle            bool

	// Internal fields for batch run state management (not set by flags)
	BatchRunStateManager *state.Manager
	BatchRunState        *state.State
}
