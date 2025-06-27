package cli

import (
	"flag"
	"os"
	"path/filepath"
)

// Flags holds all command-line flag values
type Flags struct {
	VersionFlag        bool
	ConfigPath         string
	ListOrgs           bool
	ListRepos          bool
	SyncRepo           string
	SyncAll            bool
	SyncCodebergPublic bool
	SyncGitHubPublic   bool
	FullSync           bool
	CreateGitHubRepos  bool
	CreateCodebergRepos bool
	DryRun             bool
	WorkDir            string
	TestGitHubToken    bool
	Clean              bool
	DeleteRepo         string
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
	
	return f
}