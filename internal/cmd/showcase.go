package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"codeberg.org/snonux/gitsyncer/internal/cli"
)

var (
	forceRegenerate bool
	outputPath      string
	outputFormat    string
	excludePattern  string
	showcaseAITool  string
)

var showcaseCmd = &cobra.Command{
	Use:   "showcase",
	Short: "Generate AI-powered project showcase",
	Long: `Generate a comprehensive showcase of all your projects using AI.
This feature creates a formatted document with project summaries, statistics, 
and code snippets. By default uses Claude, but can also use aichat.`,
	Example: `  # Generate showcase with cached summaries
  gitsyncer showcase
  
  # Force regeneration of all summaries
  gitsyncer showcase --force
  
  # Custom output path
  gitsyncer showcase --output ~/my-showcase.md
  
  # Different output format
  gitsyncer showcase --format markdown
  
  # Exclude certain repositories
  gitsyncer showcase --exclude "test-.*"
  
  # Use aichat instead of claude for AI summaries
  gitsyncer showcase --ai-tool aichat`,
	Run: func(cmd *cobra.Command, args []string) {
		flags := buildFlags()
		flags.Showcase = true
		flags.Force = forceRegenerate
		flags.AITool = showcaseAITool
		
		fmt.Println("Running showcase generation for all repositories...")
		exitCode := cli.HandleShowcaseOnly(cfg, flags)
		os.Exit(exitCode)
	},
}

func init() {
	rootCmd.AddCommand(showcaseCmd)
	
	// Showcase flags
	showcaseCmd.Flags().BoolVarP(&forceRegenerate, "force", "f", false, "force regeneration of cached summaries")
	showcaseCmd.Flags().StringVarP(&outputPath, "output", "o", "", "custom output path (default: ~/git/foo.zone-content/gemtext/about/showcase.gmi.tpl)")
	showcaseCmd.Flags().StringVar(&outputFormat, "format", "gemtext", "output format: gemtext, markdown, html")
	showcaseCmd.Flags().StringVar(&excludePattern, "exclude", "", "exclude repos matching pattern")
    showcaseCmd.Flags().StringVar(&showcaseAITool, "ai-tool", "claude", "AI tool to use for project summaries (claude or aichat)")
}
