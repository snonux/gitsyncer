package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"codeberg.org/snonux/gitsyncer/internal/cli"
	"codeberg.org/snonux/gitsyncer/internal/state"
	"github.com/spf13/cobra"
)

var force bool

var manageCmd = &cobra.Command{
	Use:   "manage",
	Short: "Manage repositories and workspace",
	Long:  `Commands for managing repositories, workspace, and automated operations.`,
}

var deleteRepoCmd = &cobra.Command{
	Use:   "delete-repo [name]",
	Short: "Delete repository from all organizations",
	Long:  `Delete a specified repository from all configured organizations with confirmation.`,
	Args:  cobra.ExactArgs(1),
	Example: `  # Delete a repository from all organizations
  gitsyncer manage delete-repo old-project`,
	Run: func(cmd *cobra.Command, args []string) {
		os.Exit(cli.HandleDeleteRepo(cfg, args[0]))
	},
}

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean work directory",
	Long:  `Delete all repositories in the work directory with confirmation.`,
	Example: `  # Clean the work directory
  gitsyncer manage clean
  
  # Force clean without confirmation
  gitsyncer manage clean --force`,
	Run: func(cmd *cobra.Command, args []string) {
		flags := buildFlags()
		flags.Clean = true
		flags.Force = force

		// TODO: Implement clean handler
		fmt.Println("Clean command not yet implemented")
		os.Exit(1)
	},
}

var batchRunCmd = &cobra.Command{
	Use:   "batch-run",
	Short: "Weekly automated sync",
	Long: `Enable full sync and showcase generation, but only runs once per week.
This is designed for automated weekly synchronization from cron jobs or shell scripts.`,
	Example: `  # Run weekly batch sync
  gitsyncer manage batch-run
  
  # Force run even if already run this week
  gitsyncer manage batch-run --force`,
	Run: func(cmd *cobra.Command, args []string) {
		flags := buildFlags()
		flags.BatchRun = true
		flags.Force = force

		// Check state unless forced
		if !force {
			stateManager := state.NewManager(workDir)
			s, err := stateManager.Load()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to load state: %v\n", err)
			}

			if s.HasRunWithinWeek() {
				fmt.Printf("Batch run was already executed within the past week (last run: %s).\n",
					s.LastBatchRun.Format("2006-01-02 15:04:05"))
				stateFile := filepath.Join(workDir, ".gitsyncer-state.json")
				fmt.Printf("State file location: %s\n", stateFile)
				fmt.Println("Skipping batch run. Use --force to override.")
				os.Exit(0)
			}

			// Store state manager for later
			flags.BatchRunStateManager = stateManager
			flags.BatchRunState = s
		}

		fmt.Println("Starting weekly batch run (full sync + showcase)...")

		// Enable full sync and showcase
		flags.FullSync = true
		flags.Showcase = true
		flags.SyncCodebergPublic = true
		flags.SyncGitHubPublic = true
		flags.CreateGitHubRepos = true
		flags.CreateCodebergRepos = true

		// Run sync operations
		exitCode := cli.HandleSyncCodebergPublic(cfg, flags)
		if exitCode != 0 {
			os.Exit(exitCode)
		}

		exitCode = cli.HandleSyncGitHubPublic(cfg, flags)
		if exitCode != 0 {
			os.Exit(exitCode)
		}

		// Run showcase
		showcaseCode := cli.HandleShowcase(cfg, flags)
		if showcaseCode != 0 {
			os.Exit(showcaseCode)
		}

		// Save batch run state
		if flags.BatchRunStateManager != nil && flags.BatchRunState != nil {
			flags.BatchRunState.UpdateBatchRunTime()
			if err := flags.BatchRunStateManager.Save(flags.BatchRunState); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to save batch run state: %v\n", err)
			} else {
				stateFile := filepath.Join(workDir, ".gitsyncer-state.json")
				fmt.Printf("Batch run completed successfully. State saved to: %s\n", stateFile)
				fmt.Println("Next batch run allowed after one week.")
			}
		}

		os.Exit(0)
	},
}

func init() {
	rootCmd.AddCommand(manageCmd)
	manageCmd.AddCommand(deleteRepoCmd)
	manageCmd.AddCommand(cleanCmd)
	manageCmd.AddCommand(batchRunCmd)

	// Manage-specific flags
	cleanCmd.Flags().BoolVarP(&force, "force", "f", false, "force operation without confirmation")
	batchRunCmd.Flags().BoolVarP(&force, "force", "f", false, "force run even if already run this week")
}
