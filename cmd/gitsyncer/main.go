package main

import (
	"fmt"
	"os"
	"path/filepath"

	"codeberg.org/snonux/gitsyncer/internal/cli"
)

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
		if exitCode == 0 && flags.Showcase {
			showcaseCode := cli.HandleShowcase(cfg, flags)
			if showcaseCode != 0 {
				os.Exit(showcaseCode)
			}
		}
		os.Exit(exitCode)
	}

	// Handle sync all operation
	if flags.SyncAll {
		exitCode := cli.HandleSyncAll(cfg, flags)
		if exitCode == 0 && flags.Showcase {
			showcaseCode := cli.HandleShowcase(cfg, flags)
			if showcaseCode != 0 {
				os.Exit(showcaseCode)
			}
		}
		os.Exit(exitCode)
	}

	// Handle sync Codeberg public repos
	if flags.SyncCodebergPublic {
		exitCode := cli.HandleSyncCodebergPublic(cfg, flags)
		if exitCode != 0 || !flags.SyncGitHubPublic {
			if exitCode == 0 && flags.Showcase && !flags.SyncGitHubPublic {
				showcaseCode := cli.HandleShowcase(cfg, flags)
				if showcaseCode != 0 {
					os.Exit(showcaseCode)
				}
			}
			os.Exit(exitCode)
		}
	}

	// Handle sync GitHub public repos
	if flags.SyncGitHubPublic {
		exitCode := cli.HandleSyncGitHubPublic(cfg, flags)
		
		// Run showcase generation if requested and sync was successful
		if exitCode == 0 && flags.Showcase {
			showcaseCode := cli.HandleShowcase(cfg, flags)
			if showcaseCode != 0 {
				os.Exit(showcaseCode)
			}
		}
		
		os.Exit(exitCode)
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