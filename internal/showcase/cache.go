package showcase

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// summaryCacheDirName is the on-disk directory (relative to the sync work
// directory) where per-repo cached ProjectSummary JSON files live.
const summaryCacheDirName = ".gitsyncer-showcase-cache"

// SummaryCache owns persistence for showcase generation: reading and writing
// the per-repo JSON summary cache, and updating/applying the weekly rank-
// history snapshots (backed by the lower-level store in rank_history.go).
// Generator delegates all of this here so its own orchestration loop doesn't
// need to know about cache file layout or the rank-history file format.
type SummaryCache struct {
	workDir string
}

// NewSummaryCache creates a SummaryCache rooted at workDir (the same
// directory that holds the synced repositories).
func NewSummaryCache(workDir string) *SummaryCache {
	return &SummaryCache{workDir: workDir}
}

// dir returns the cache directory path.
func (c *SummaryCache) dir() string {
	return filepath.Join(c.workDir, summaryCacheDirName)
}

// FilePath returns the cache file path for a given repository. Exposed so
// callers can include it in log/diagnostic output without needing to know
// the cache directory layout themselves.
func (c *SummaryCache) FilePath(repoName string) string {
	return filepath.Join(c.dir(), repoName+".json")
}

// Load reads a cached project summary for repoName from disk.
func (c *SummaryCache) Load(repoName string) (*ProjectSummary, error) {
	data, err := os.ReadFile(c.FilePath(repoName))
	if err != nil {
		return nil, err
	}

	var summary ProjectSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, err
	}

	return &summary, nil
}

// Save writes summary to the cache, creating the cache directory if needed.
func (c *SummaryCache) Save(repoName string, summary *ProjectSummary) error {
	if err := os.MkdirAll(c.dir(), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.FilePath(repoName), data, 0644)
}

// LoadAll reads cached summaries for every name in repos that isExcluded
// rejects, keyed by repo name. Repos with no cache entry (or an unreadable
// one) are silently skipped, matching the historical "best effort" merge
// behavior used to rebuild the full showcase around a single freshly
// regenerated project.
func (c *SummaryCache) LoadAll(repos []string, isExcluded func(string) bool) map[string]ProjectSummary {
	summaries := make(map[string]ProjectSummary, len(repos))
	for _, repo := range repos {
		if isExcluded(repo) {
			continue
		}
		if cached, err := c.Load(repo); err == nil {
			summaries[repo] = *cached
		}
	}
	return summaries
}

// UpdateRankHistorySnapshot loads the rank-history store, upserts a snapshot
// for anchorDate built from summaries' current ordering, and saves the store
// back to disk. Only full showcase runs should call this: a single-repo
// update must not perturb the shared weekly ranking snapshot.
func (c *SummaryCache) UpdateRankHistorySnapshot(anchorDate time.Time, summaries []ProjectSummary) error {
	file := c.rankHistoryFile()
	store, err := loadRankHistory(file)
	if err != nil {
		return fmt.Errorf("failed to load rank history: %w", err)
	}

	upsertSnapshotForDate(store, anchorDate, buildCurrentRanks(summaries))
	if err := saveRankHistory(file, store); err != nil {
		return fmt.Errorf("failed to save rank history: %w", err)
	}

	return nil
}

// ApplyRankHistory loads the rank-history store and annotates each summary
// with its RankHistory field (the last rankHistoryPoints weekly points,
// relative to anchorDate).
func (c *SummaryCache) ApplyRankHistory(summaries []ProjectSummary, anchorDate time.Time) error {
	store, err := loadRankHistory(c.rankHistoryFile())
	if err != nil {
		return fmt.Errorf("failed to load rank history: %w", err)
	}

	applyRankHistoryToSummaries(summaries, store, anchorDate, rankHistoryPoints)
	return nil
}

// rankHistoryFile returns the path to the rank-history JSON file.
func (c *SummaryCache) rankHistoryFile() string {
	return filepath.Join(c.workDir, rankHistoryFilename)
}

// verifyImages checks that every image path recorded in summary still exists
// under showcaseDir. It's a validation helper for cached summaries.
func verifyImages(summary *ProjectSummary, showcaseDir string) error {
	if len(summary.Images) == 0 {
		return nil
	}

	for _, imgPath := range summary.Images {
		fullPath := filepath.Join(showcaseDir, imgPath)
		if _, err := os.Stat(fullPath); err != nil {
			return fmt.Errorf("image not found: %s", imgPath)
		}
	}

	return nil
}
