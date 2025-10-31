package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// loadDescriptionCache loads the per-repo canonical description cache
func loadDescriptionCache(workDir string) map[string]string {
	cache := make(map[string]string)
	cacheFile := filepath.Join(workDir, ".gitsyncer-descriptions-cache.json")
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return cache
	}
	if err := json.Unmarshal(data, &cache); err != nil {
		fmt.Printf("Warning: Failed to parse descriptions cache: %v\n", err)
		return make(map[string]string)
	}
	fmt.Printf("Loaded descriptions cache with %d entries\n", len(cache))
	return cache
}

// saveDescriptionCache saves the per-repo canonical description cache
func saveDescriptionCache(workDir string, cache map[string]string) error {
	cacheFile := filepath.Join(workDir, ".gitsyncer-descriptions-cache.json")
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal descriptions cache: %w", err)
	}
	if err := os.WriteFile(cacheFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write descriptions cache: %w", err)
	}
	return nil
}
