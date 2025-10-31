package cli

import (
	"fmt"
	"log"

	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/showcase"
)

// HandleShowcase handles the showcase generation after syncing
func HandleShowcase(cfg *config.Config, flags *Flags) int {
	// Determine which repositories to process
	var repoFilter []string
	if flags.SyncRepo != "" {
		// Only process the specific repository that was synced
		repoFilter = []string{flags.SyncRepo}
		fmt.Printf("\nGenerating showcase for %s...\n", flags.SyncRepo)
	} else {
		// Process all repositories for --sync-all or public sync operations
		fmt.Println("\nGenerating project showcase for all repositories...")
	}

	// Create showcase generator
	generator := showcase.New(cfg, flags.WorkDir)

	// Set AI tool if specified
	if flags.AITool != "" {
		generator.SetAITool(flags.AITool)
	}

	// Generate showcase with optional filter
	if err := generator.GenerateShowcase(repoFilter, flags.Force); err != nil {
		log.Printf("ERROR: Failed to generate showcase: %v\n", err)
		return 1
	}

	fmt.Println("Showcase generated successfully!")
	return 0
}
