package release

// Persistent on-disk cache of AI-generated release notes, keyed by
// "<repo>:<tag>", so repeated runs do not re-generate notes for a release
// that was already processed.

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadAIReleaseNotesCache loads the AI release notes cache from disk. A
// missing or unparsable cache file yields an empty cache rather than an
// error, since the cache is a performance optimization, not required state.
func LoadAIReleaseNotesCache(cacheFile string) map[string]string {
	cache := make(map[string]string)

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		// Cache file doesn't exist yet, return empty cache
		return cache
	}

	if err := json.Unmarshal(data, &cache); err != nil {
		fmt.Printf("Warning: Failed to parse AI release notes cache: %v\n", err)
		return make(map[string]string)
	}

	fmt.Printf("Loaded AI release notes cache with %d entries\n", len(cache))
	return cache
}

// SaveAIReleaseNotesCache saves the AI release notes cache to disk.
func SaveAIReleaseNotesCache(cacheFile string, cache map[string]string) error {
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	if err := os.WriteFile(cacheFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	// Don't print on every save since we save after each generation
	return nil
}
