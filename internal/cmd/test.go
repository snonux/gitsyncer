package cmd

import (
	"fmt"
	"os"

	"github.com/snonux/gitsyncer/internal/cli"
	"github.com/snonux/gitsyncer/internal/codeberg"
	"github.com/snonux/gitsyncer/internal/config"
	"github.com/snonux/gitsyncer/internal/github"
	"github.com/spf13/cobra"
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

		for _, warning := range configTokenWarnings(cfg) {
			fmt.Println("  ⚠️  " + warning)
		}

		os.Exit(0)
	},
}

// configTokenWarnings reports GitHub/Codeberg organizations whose token
// doesn't actually resolve, and flags configs with neither forge configured.
// It checks resolvability via HasToken (the same config -> env var -> token
// file cascade the real sync path uses via forge.ResolveToken) rather than
// just whether the config file inlines a token - most setups intentionally
// leave the config token field empty and rely on a token file instead
// (e.g. ~/.gitsyncer_github_token), which checking only the config field
// would always flag as missing.
func configTokenWarnings(cfg *config.Config) []string {
	var warnings []string
	hasGitHub := false
	hasCodeberg := false

	for _, org := range cfg.Organizations {
		if org.Host == "git@github.com" {
			hasGitHub = true
			if !github.NewClient(org.GitHubToken, org.Name).HasToken() {
				warnings = append(warnings, "Warning: GitHub organization without token")
			}
		}
		if org.Host == "git@codeberg.org" {
			hasCodeberg = true
			if !codeberg.NewClient(org.CodebergToken, org.Name).HasToken() {
				warnings = append(warnings, "Warning: Codeberg organization without token")
			}
		}
	}

	if !hasGitHub && !hasCodeberg {
		warnings = append(warnings, "Warning: No GitHub or Codeberg organizations configured")
	}

	return warnings
}

func init() {
	rootCmd.AddCommand(testCmd)
	testCmd.AddCommand(testGitHubCmd)
	testCmd.AddCommand(testCodebergCmd)
	testCmd.AddCommand(testConfigCmd)
}
