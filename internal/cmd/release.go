package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"codeberg.org/snonux/gitsyncer/internal/cli"
)

var (
	autoRelease    bool
	noAINotes      bool
	updateExisting bool
	templatePath   string
)

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Manage releases across platforms",
	Long: `Check for version tags without releases and create them across 
GitHub and Codeberg. Supports AI-generated release notes using Claude.`,
}

var releaseCheckCmd = &cobra.Command{
	Use:   "check [repo]",
	Short: "Check for missing releases",
	Long: `Check for version tags that don't have corresponding releases.
If no repository is specified, checks all configured repositories.`,
	Args: cobra.MaximumNArgs(1),
	Example: `  # Check all repositories
  gitsyncer release check
  
  # Check specific repository
  gitsyncer release check myproject
  
  # Check with dry-run
  gitsyncer release check --dry-run`,
	Run: func(cmd *cobra.Command, args []string) {
		flags := buildFlags()
		flags.CheckReleases = true
		
		if len(args) > 0 {
			// Check specific repo
			exitCode := cli.HandleCheckReleasesForRepo(cfg, flags, args[0])
			os.Exit(exitCode)
		} else {
			// Check all repos
			exitCode := cli.HandleCheckReleases(cfg, flags)
			os.Exit(exitCode)
		}
	},
}

var releaseCreateCmd = &cobra.Command{
	Use:   "create [repo]",
	Short: "Create releases for version tags",
	Long: `Create releases for version tags that don't have them.
If no repository is specified, processes all configured repositories.`,
	Args: cobra.MaximumNArgs(1),
	Example: `  # Create releases (AI notes enabled by default)
  gitsyncer release create
  
  # Auto-create without prompts
  gitsyncer release create --auto
  
  # Create without AI-generated notes
  gitsyncer release create --no-ai-notes
  
  # Update existing releases with AI notes
  gitsyncer release create --update-existing
  
  # Create for specific repository without AI
  gitsyncer release create myproject --no-ai-notes`,
	Run: func(cmd *cobra.Command, args []string) {
		flags := buildFlags()
		flags.CheckReleases = true
		flags.AutoCreateReleases = autoRelease
		flags.AIReleaseNotes = !noAINotes
		flags.UpdateReleases = updateExisting
		
		if len(args) > 0 {
			// Create releases for specific repo
			exitCode := cli.HandleCheckReleasesForRepo(cfg, flags, args[0])
			os.Exit(exitCode)
		} else {
			// Create releases for all repos
			exitCode := cli.HandleCheckReleases(cfg, flags)
			os.Exit(exitCode)
		}
	},
}

func init() {
	rootCmd.AddCommand(releaseCmd)
	releaseCmd.AddCommand(releaseCheckCmd)
	releaseCmd.AddCommand(releaseCreateCmd)
	
	// Release flags
	releaseCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "preview what releases would be created")
	
	// Create-specific flags
	releaseCreateCmd.Flags().BoolVar(&autoRelease, "auto", false, "skip confirmation prompts")
	releaseCreateCmd.Flags().BoolVar(&noAINotes, "no-ai-notes", false, "disable AI-generated release notes (AI notes are enabled by default)")
	releaseCreateCmd.Flags().BoolVar(&updateExisting, "update-existing", false, "update existing releases with new AI-generated notes")
	releaseCreateCmd.Flags().StringVar(&templatePath, "template", "", "custom template for release notes")
}