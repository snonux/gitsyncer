package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"codeberg.org/snonux/gitsyncer/internal/cli"
	"codeberg.org/snonux/gitsyncer/internal/config"
)

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Test authentication and configuration",
	Long:  `Test various aspects of the gitsyncer configuration including authentication tokens.`,
}

var testGitHubCmd = &cobra.Command{
	Use:   "github-token",
	Short: "Test GitHub authentication",
	Example: `  # Test GitHub token authentication
  gitsyncer test github-token`,
	Run: func(cmd *cobra.Command, args []string) {
		os.Exit(cli.HandleTestGitHubToken())
	},
}

var testCodebergCmd = &cobra.Command{
	Use:   "codeberg-token",
	Short: "Test Codeberg authentication",
	Example: `  # Test Codeberg token authentication
  gitsyncer test codeberg-token`,
	Run: func(cmd *cobra.Command, args []string) {
		// TODO: Implement Codeberg token test
		fmt.Println("Codeberg token test not yet implemented")
		os.Exit(1)
	},
}

var testConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Validate configuration file",
	Example: `  # Validate configuration
  gitsyncer test config
  
  # Test specific config file
  gitsyncer test config -c ~/my-config.json`,
	Run: func(cmd *cobra.Command, args []string) {
		// Try to load and validate config
		cfg, err := config.Load(cfgFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Configuration validation failed: %v\n", err)
			os.Exit(1)
		}
		
		fmt.Println("Configuration validation successful!")
		fmt.Printf("  Organizations: %d\n", len(cfg.Organizations))
		fmt.Printf("  Repositories: %d\n", len(cfg.Repositories))
		
		// Check for common issues
		hasGitHub := false
		hasCodeberg := false
		for _, org := range cfg.Organizations {
			if org.Host == "git@github.com" {
				hasGitHub = true
				if org.GitHubToken == "" {
					fmt.Println("  ⚠️  Warning: GitHub organization without token")
				}
			}
			if org.Host == "git@codeberg.org" {
				hasCodeberg = true
				if org.CodebergToken == "" {
					fmt.Println("  ⚠️  Warning: Codeberg organization without token")
				}
			}
		}
		
		if !hasGitHub && !hasCodeberg {
			fmt.Println("  ⚠️  Warning: No GitHub or Codeberg organizations configured")
		}
		
		os.Exit(0)
	},
}

func init() {
	rootCmd.AddCommand(testCmd)
	testCmd.AddCommand(testGitHubCmd)
	testCmd.AddCommand(testCodebergCmd)
	testCmd.AddCommand(testConfigCmd)
}