package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/version"
)

var (
	cfgFile  string
	workDir  string
	cfg      *config.Config
	rootCmd = &cobra.Command{
		Use:   "gitsyncer",
		Short: "Synchronize git repositories across multiple platforms",
		Long: `GitSyncer is a tool for synchronizing git repositories between 
multiple organizations (e.g., GitHub and Codeberg). It automatically 
keeps all branches in sync across different git hosting platforms.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Skip config loading for version command
			if cmd.Use == "version" {
				return
			}
			
			// Load configuration
			var err error
			cfg, err = config.Load(cfgFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
				fmt.Fprintf(os.Stderr, "\nPlease create a configuration file with your organizations and repositories.\n")
				fmt.Fprintf(os.Stderr, "See 'gitsyncer help' for more information.\n")
				os.Exit(1)
			}
			
			// Use config WorkDir if no flag was explicitly provided
			if !cmd.Flags().Changed("work-dir") && cfg.WorkDir != "" {
				workDir = cfg.WorkDir
			}
		},
	}
)

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "configuration file (default: ~/.gitsyncer.json)")
	
	// Set default work directory
	home, err := os.UserHomeDir()
	defaultWorkDir := ".gitsyncer-work"
	if err == nil {
		defaultWorkDir = filepath.Join(home, "git", "gitsyncer-workdir")
	}
	
	rootCmd.PersistentFlags().StringVarP(&workDir, "work-dir", "w", defaultWorkDir, "working directory for operations")
	
	// Version command
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version.GetVersion())
		},
	})
	
}

