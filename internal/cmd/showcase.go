package cmd

import (
	"fmt"
	"os"

	"github.com/snonux/gitsyncer/internal/cli"
	"github.com/spf13/cobra"
)

var (
	forceRegenerate bool
	showcaseAITool  string
	showcaseRepo    string
)

var showcaseCmd = &cobra.Command{
	Use:   "showcase",
	Short: "Generate AI-powered project showcase",
	Long: `Generate a comprehensive showcase of all your projects using AI.
This feature creates a formatted document with project summaries, statistics,
and code snippets. By default uses opencode (via ollama launch with glm-5.2:cloud), with fallback to hexai, claude, and amp.`,
	Example: `  # Generate showcase with cached summaries
  gitsyncer showcase
  
  # Force regeneration of all summaries
  gitsyncer showcase --force
  
  # Use a specific AI tool
  gitsyncer showcase --ai-tool opencode
  
  # Generate showcase for one repository
  gitsyncer showcase --repo gitsyncer`,
	Run: func(cmd *cobra.Command, args []string) {
		flags := buildFlags()
		flags.Showcase = true
		flags.Force = forceRegenerate
		flags.AITool = showcaseAITool
		if showcaseRepo != "" {
			flags.SyncRepo = showcaseRepo
		}

		if flags.SyncRepo != "" {
			fmt.Printf("Running showcase generation for repository: %s...\n", flags.SyncRepo)
		} else {
			fmt.Println("Running showcase generation for all repositories...")
		}
		exitCode := cli.HandleShowcaseOnly(cfg, flags)
		os.Exit(exitCode)
	},
}

func init() {
	rootCmd.AddCommand(showcaseCmd)

	// Showcase flags
	showcaseCmd.Flags().BoolVarP(&forceRegenerate, "force", "f", false, "force regeneration of cached summaries")
	showcaseCmd.Flags().StringVar(&showcaseAITool, "ai-tool", "opencode", "AI tool for summaries: opencode, hexai, claude, amp, or claude-code (default tries opencode→hexai→claude→amp)")
	showcaseCmd.Flags().StringVar(&showcaseRepo, "repo", "", "only generate showcase for a single repository")
}
