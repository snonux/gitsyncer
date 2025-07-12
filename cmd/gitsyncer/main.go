package main

import (
	"fmt"
	"os"
	"path/filepath"

	"codeberg.org/snonux/gitsyncer/internal/cli"
	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/state"
)

// saveBatchRunState saves the batch run timestamp if this is a batch run
func saveBatchRunState(flags *cli.Flags) {
	if flags.BatchRun && flags.BatchRunStateManager != nil && flags.BatchRunState != nil {
		flags.BatchRunState.UpdateBatchRunTime()
		if err := flags.BatchRunStateManager.Save(flags.BatchRunState); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to save batch run state: %v\n", err)
		} else {
			stateFile := filepath.Join(flags.WorkDir, ".gitsyncer-state.json")
			fmt.Printf("Batch run completed successfully. State saved to: %s\n", stateFile)
			fmt.Println("Next batch run allowed after one week.")
		}
	}
}

// runReleaseCheckIfEnabled runs release checking after successful sync operations
func runReleaseCheckIfEnabled(cfg *config.Config, flags *cli.Flags) {
	// Run release checks automatically unless disabled
	if !flags.NoCheckReleases {
		fmt.Println("\nChecking for missing releases...")
		cli.HandleCheckReleases(cfg, flags)
	}
}

// runReleaseCheckForRepoIfEnabled runs release checking for a specific repository
func runReleaseCheckForRepoIfEnabled(cfg *config.Config, flags *cli.Flags, repoName string) {
	// Run release checks automatically unless disabled
	if !flags.NoCheckReleases {
		fmt.Println("\nChecking for missing releases...")
		cli.HandleCheckReleasesForRepo(cfg, flags, repoName)
	}
}

func main() {
	// Parse command-line flags
	flags := cli.ParseFlags()

	// Handle --full flag message
	if flags.FullSync {
		cli.ShowFullSyncMessage()
	}

	// Handle version flag
	if flags.VersionFlag {
		os.Exit(cli.HandleVersion())
	}
	
	// Handle test GitHub token flag
	if flags.TestGitHubToken {
		os.Exit(cli.HandleTestGitHubToken())
	}

	// Load configuration
	cfg, err := cli.LoadConfig(flags.ConfigPath)
	if err != nil {
		cli.ShowConfigHelp()
		os.Exit(1)
	}

	// Use config WorkDir only if no flag was explicitly provided
	// We check if WorkDir matches the default we set in ParseFlags
	home, _ := os.UserHomeDir()
	defaultWorkDir := filepath.Join(home, "git", "gitsyncer-workdir")
	if flags.WorkDir == defaultWorkDir && cfg.WorkDir != "" {
		// User didn't specify --work-dir, so use config value
		flags.WorkDir = cfg.WorkDir
	}

	// Handle --batch-run flag: check if it has run within the past week
	if flags.BatchRun {
		stateManager := state.NewManager(flags.WorkDir)
		s, err := stateManager.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to load state: %v\n", err)
			// Continue anyway on first run
		}
		
		if s.HasRunWithinWeek() {
			fmt.Printf("Batch run was already executed within the past week (last run: %s).\n", s.LastBatchRun.Format("2006-01-02 15:04:05"))
			stateFile := filepath.Join(flags.WorkDir, ".gitsyncer-state.json")
			fmt.Printf("State file location: %s\n", stateFile)
			fmt.Println("Skipping batch run. Use --full and --showcase directly to force execution.")
			os.Exit(0)
		}
		
		// If we get here, we can proceed with the batch run
		fmt.Println("Starting weekly batch run (--full --showcase)...")
		
		// Update the state to record this batch run (we'll save it after successful completion)
		// Store the state manager for later use
		flags.BatchRunStateManager = stateManager
		flags.BatchRunState = s
	}

	// Handle delete repository flag
	if flags.DeleteRepo != "" {
		os.Exit(cli.HandleDeleteRepo(cfg, flags.DeleteRepo))
	}

	// Handle list organizations flag
	if flags.ListOrgs {
		os.Exit(cli.HandleListOrgs(cfg))
	}

	// Handle list repositories flag
	if flags.ListRepos {
		os.Exit(cli.HandleListRepos(cfg))
	}

	// Handle sync operation
	if flags.SyncRepo != "" {
		exitCode := cli.HandleSync(cfg, flags)
		if exitCode == 0 {
			runReleaseCheckForRepoIfEnabled(cfg, flags, flags.SyncRepo)
			if flags.Showcase {
				showcaseCode := cli.HandleShowcase(cfg, flags)
				if showcaseCode != 0 {
					os.Exit(showcaseCode)
				}
			}
		}
		os.Exit(exitCode)
	}

	// Handle sync all operation
	if flags.SyncAll {
		exitCode := cli.HandleSyncAll(cfg, flags)
		if exitCode == 0 {
			runReleaseCheckIfEnabled(cfg, flags)
			if flags.Showcase {
				showcaseCode := cli.HandleShowcase(cfg, flags)
				if showcaseCode != 0 {
					os.Exit(showcaseCode)
				}
			}
		}
		os.Exit(exitCode)
	}

	// Handle sync Codeberg public repos
	if flags.SyncCodebergPublic {
		exitCode := cli.HandleSyncCodebergPublic(cfg, flags)
		if exitCode != 0 || !flags.SyncGitHubPublic {
			if exitCode == 0 {
				runReleaseCheckIfEnabled(cfg, flags)
				if flags.Showcase && !flags.SyncGitHubPublic {
					showcaseCode := cli.HandleShowcase(cfg, flags)
					if showcaseCode != 0 {
						os.Exit(showcaseCode)
					}
				}
			}
			os.Exit(exitCode)
		}
	}

	// Handle sync GitHub public repos
	if flags.SyncGitHubPublic {
		exitCode := cli.HandleSyncGitHubPublic(cfg, flags)
		
		if exitCode == 0 {
			// Run release checks after successful sync
			runReleaseCheckIfEnabled(cfg, flags)
			
			// Run showcase generation if requested
			if flags.Showcase {
				showcaseCode := cli.HandleShowcase(cfg, flags)
				if showcaseCode != 0 {
					os.Exit(showcaseCode)
				}
			}
			
			// Save batch run state if this was a successful batch run
			saveBatchRunState(flags)
		}
		
		os.Exit(exitCode)
	}
	
	// Handle check releases flag
	if flags.CheckReleases {
		os.Exit(cli.HandleCheckReleases(cfg, flags))
	}
	
	// Handle standalone showcase mode (no sync operations specified)
	if flags.Showcase {
		fmt.Println("Running showcase generation for all repositories (clone-only mode)...")
		os.Exit(cli.HandleShowcaseOnly(cfg, flags))
	}

	// Default: show usage
	cli.ShowUsage(cfg)
	os.Exit(1)
}