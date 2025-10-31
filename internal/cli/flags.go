package cli

import (
	"flag"
	"os"
	"path/filepath"

	"codeberg.org/snonux/gitsyncer/internal/state"
)

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

	// Internal fields for batch run state management (not set by flags)
	BatchRunStateManager *state.Manager
	BatchRunState        *state.State
}

// ParseFlags parses command-line flags and returns the flags struct
func ParseFlags() *Flags {
	f := &Flags{}

	flag.BoolVar(&f.VersionFlag, "version", false, "print version information")
	flag.BoolVar(&f.VersionFlag, "v", false, "print version information (short)")
	flag.StringVar(&f.ConfigPath, "config", "", "path to configuration file")
	flag.StringVar(&f.ConfigPath, "c", "", "path to configuration file (short)")
	flag.BoolVar(&f.ListOrgs, "list-orgs", false, "list configured organizations")
	flag.BoolVar(&f.ListRepos, "list-repos", false, "list configured repositories")
	flag.StringVar(&f.SyncRepo, "sync", "", "repository name to sync")
	flag.BoolVar(&f.SyncAll, "sync-all", false, "sync all configured repositories")
	flag.BoolVar(&f.SyncCodebergPublic, "sync-codeberg-public", false, "sync all public Codeberg repositories to GitHub")
	flag.BoolVar(&f.SyncGitHubPublic, "sync-github-public", false, "sync all public GitHub repositories to Codeberg")
	flag.BoolVar(&f.FullSync, "full", false, "full bidirectional sync (enables --sync-codeberg-public --sync-github-public --create-github-repos --create-codeberg-repos)")
	flag.BoolVar(&f.CreateGitHubRepos, "create-github-repos", false, "automatically create missing GitHub repositories")
	flag.BoolVar(&f.CreateCodebergRepos, "create-codeberg-repos", false, "automatically create missing Codeberg repositories")
	flag.BoolVar(&f.DryRun, "dry-run", false, "show what would be synced without actually syncing")
	flag.StringVar(&f.WorkDir, "work-dir", "", "working directory for cloning repositories (default: ~/git/gitsyncer-workdir)")
	flag.BoolVar(&f.TestGitHubToken, "test-github-token", false, "test GitHub token authentication")
	flag.BoolVar(&f.Clean, "clean", false, "delete all repositories in work directory (with confirmation)")
	flag.StringVar(&f.DeleteRepo, "delete-repo", "", "delete specified repository from all configured organizations (with confirmation)")
	flag.BoolVar(&f.Backup, "backup", false, "enable syncing to backup locations")
	flag.BoolVar(&f.Showcase, "showcase", false, "generate project showcase using AI (amp by default) after syncing")
	flag.BoolVar(&f.Force, "force", false, "force regeneration of cached data")
	flag.BoolVar(&f.BatchRun, "batch-run", false, "enable --full and --showcase (runs only once per week)")
	flag.BoolVar(&f.CheckReleases, "check-releases", false, "manually check for version tags without releases and create them (with confirmation)")
	flag.BoolVar(&f.NoCheckReleases, "no-check-releases", false, "disable automatic release checking after sync operations")
	flag.BoolVar(&f.AutoCreateReleases, "auto-create-releases", false, "automatically create releases without confirmation prompts")
	flag.BoolVar(&f.AIReleaseNotes, "ai-release-notes", false, "generate release notes using AI (amp by default) based on git diff")
	flag.BoolVar(&f.UpdateReleases, "update-releases", false, "update existing releases with new AI-generated notes")

	flag.Parse()

	// Set default WorkDir if not provided
	if f.WorkDir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			f.WorkDir = filepath.Join(home, "git", "gitsyncer-workdir")
		} else {
			// Fallback if we can't get home directory
			f.WorkDir = ".gitsyncer-work"
		}
	}

	// Handle --full flag by enabling all sync operations
	if f.FullSync {
		f.SyncCodebergPublic = true
		f.SyncGitHubPublic = true
		f.CreateGitHubRepos = true
		f.CreateCodebergRepos = true
	}

	// Handle --batch-run flag by enabling --full and --showcase
	if f.BatchRun {
		f.FullSync = true
		f.Showcase = true
		// Since we set FullSync, it will trigger the above logic too
		f.SyncCodebergPublic = true
		f.SyncGitHubPublic = true
		f.CreateGitHubRepos = true
		f.CreateCodebergRepos = true
	}

	return f
}
