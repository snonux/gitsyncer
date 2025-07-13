package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"codeberg.org/snonux/gitsyncer/internal/cli"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List organizations and repositories",
	Long:  `Display configured organizations and repositories from the configuration file.`,
}

var listOrgsCmd = &cobra.Command{
	Use:   "orgs",
	Short: "List configured organizations",
	Example: `  # List all configured organizations
  gitsyncer list orgs`,
	Run: func(cmd *cobra.Command, args []string) {
		os.Exit(cli.HandleListOrgs(cfg))
	},
}

var listReposCmd = &cobra.Command{
	Use:   "repos",
	Short: "List configured repositories",
	Example: `  # List all configured repositories
  gitsyncer list repos`,
	Run: func(cmd *cobra.Command, args []string) {
		os.Exit(cli.HandleListRepos(cfg))
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.AddCommand(listOrgsCmd)
	listCmd.AddCommand(listReposCmd)
}