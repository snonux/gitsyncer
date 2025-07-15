package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"codeberg.org/snonux/gitsyncer/internal/cli"
)

var (
	dryRun         bool
	backup         bool
	createRepos    bool
	noReleases     bool
	autoCreate     bool
	noAIReleaseNotes bool
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronize repositories between platforms",
	Long: `Synchronize git repositories across multiple platforms.
This command provides various sync operations for keeping repositories
in sync between GitHub, Codeberg, and other configured platforms.`,
}

var syncRepoCmd = &cobra.Command{
	Use:   "repo [name]",
	Short: "Sync a specific repository",
	Long:  `Synchronize a specific repository across all configured organizations.`,
	Args:  cobra.ExactArgs(1),
	Example: `  # Sync a single repository (AI release notes enabled by default)
  gitsyncer sync repo myproject
  
  # Sync with backup locations
  gitsyncer sync repo myproject --backup
  
  # Preview what would be synced
  gitsyncer sync repo myproject --dry-run
  
  # Sync without AI-generated release notes
  gitsyncer sync repo myproject --no-ai-release-notes`,
	Run: func(cmd *cobra.Command, args []string) {
		flags := buildFlags()
		flags.SyncRepo = args[0]
		
		exitCode := cli.HandleSync(cfg, flags)
		if exitCode == 0 && !noReleases {
			cli.HandleCheckReleasesForRepo(cfg, flags, args[0])
		}
		os.Exit(exitCode)
	},
}

var syncAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Sync all configured repositories",
	Long:  `Synchronize all repositories listed in the configuration file.`,
	Example: `  # Sync all configured repositories
  gitsyncer sync all
  
  # Include backup locations
  gitsyncer sync all --backup
  
  # Preview changes
  gitsyncer sync all --dry-run`,
	Run: func(cmd *cobra.Command, args []string) {
		flags := buildFlags()
		flags.SyncAll = true
		
		exitCode := cli.HandleSyncAll(cfg, flags)
		if exitCode == 0 && !noReleases {
			cli.HandleCheckReleases(cfg, flags)
		}
		os.Exit(exitCode)
	},
}

var syncCodebergToGitHubCmd = &cobra.Command{
	Use:   "codeberg-to-github",
	Short: "Sync public Codeberg repos to GitHub",
	Long:  `Synchronize all public repositories from Codeberg to GitHub.`,
	Example: `  # Sync Codeberg public repos to GitHub
  gitsyncer sync codeberg-to-github
  
  # Auto-create missing GitHub repos
  gitsyncer sync codeberg-to-github --create-repos
  
  # Preview what would be synced
  gitsyncer sync codeberg-to-github --dry-run`,
	Run: func(cmd *cobra.Command, args []string) {
		flags := buildFlags()
		flags.SyncCodebergPublic = true
		
		if createRepos || autoCreate {
			flags.CreateGitHubRepos = true
		}
		
		exitCode := cli.HandleSyncCodebergPublic(cfg, flags)
		if exitCode == 0 && !noReleases {
			cli.HandleCheckReleases(cfg, flags)
		}
		os.Exit(exitCode)
	},
}

var syncGitHubToCodebergCmd = &cobra.Command{
	Use:   "github-to-codeberg",
	Short: "Sync public GitHub repos to Codeberg",
	Long:  `Synchronize all public repositories from GitHub to Codeberg.`,
	Example: `  # Sync GitHub public repos to Codeberg
  gitsyncer sync github-to-codeberg
  
  # Auto-create missing Codeberg repos
  gitsyncer sync github-to-codeberg --create-repos
  
  # Preview what would be synced
  gitsyncer sync github-to-codeberg --dry-run`,
	Run: func(cmd *cobra.Command, args []string) {
		flags := buildFlags()
		flags.SyncGitHubPublic = true
		
		if createRepos || autoCreate {
			flags.CreateCodebergRepos = true
		}
		
		exitCode := cli.HandleSyncGitHubPublic(cfg, flags)
		if exitCode == 0 && !noReleases {
			cli.HandleCheckReleases(cfg, flags)
		}
		os.Exit(exitCode)
	},
}

var syncBidirectionalCmd = &cobra.Command{
	Use:   "bidirectional",
	Short: "Full bidirectional sync of all public repos",
	Long: `Perform a complete bidirectional synchronization of all public 
repositories between GitHub and Codeberg. This is equivalent to the old --full flag.`,
	Example: `  # Full bidirectional sync
  gitsyncer sync bidirectional
  
  # Preview what would be synced
  gitsyncer sync bidirectional --dry-run
  
  # Include backup locations
  gitsyncer sync bidirectional --backup`,
	Run: func(cmd *cobra.Command, args []string) {
		flags := buildFlags()
		flags.FullSync = true
		flags.SyncCodebergPublic = true
		flags.SyncGitHubPublic = true
		flags.CreateGitHubRepos = true
		flags.CreateCodebergRepos = true
		
		// First sync Codeberg to GitHub
		exitCode := cli.HandleSyncCodebergPublic(cfg, flags)
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		
		// Then sync GitHub to Codeberg
		exitCode = cli.HandleSyncGitHubPublic(cfg, flags)
		if exitCode == 0 && !noReleases {
			cli.HandleCheckReleases(cfg, flags)
		}
		os.Exit(exitCode)
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
	
	// Add subcommands
	syncCmd.AddCommand(syncRepoCmd)
	syncCmd.AddCommand(syncAllCmd)
	syncCmd.AddCommand(syncCodebergToGitHubCmd)
	syncCmd.AddCommand(syncGitHubToCodebergCmd)
	syncCmd.AddCommand(syncBidirectionalCmd)
	
	// Sync flags (available for all sync subcommands)
	syncCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "preview what would be synced")
	syncCmd.PersistentFlags().BoolVar(&backup, "backup", false, "include backup locations")
	syncCmd.PersistentFlags().BoolVar(&createRepos, "create-repos", false, "auto-create missing repositories")
	syncCmd.PersistentFlags().BoolVar(&noReleases, "no-releases", false, "skip release checking after sync")
	syncCmd.PersistentFlags().BoolVar(&autoCreate, "auto-create-releases", false, "automatically create releases without confirmation")
	syncCmd.PersistentFlags().BoolVar(&noAIReleaseNotes, "no-ai-release-notes", false, "disable AI-generated release notes (AI notes are enabled by default)")
}

func buildFlags() *cli.Flags {
	return &cli.Flags{
		ConfigPath:   cfgFile,
		WorkDir:      workDir,
		DryRun:       dryRun,
		Backup:       backup,
		NoCheckReleases: noReleases,
		AutoCreateReleases: autoCreate,
		AIReleaseNotes: !noAIReleaseNotes,
		CreateGitHubRepos: createRepos,
		CreateCodebergRepos: createRepos,
	}
}