package showcase

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	rankHistoryFilename = ".gitsyncer-showcase-rank-history.json"
	rankHistoryPoints   = 5
	rankHistoryVersion  = 1
)

// RankSnapshot stores ranking spots for all repositories on one day.
type RankSnapshot struct {
	Date  string         `json:"date"`
	Ranks map[string]int `json:"ranks"`
}

// RankHistoryStore stores all ranking snapshots used for weekly history.
type RankHistoryStore struct {
	Version   int            `json:"version"`
	Snapshots []RankSnapshot `json:"snapshots"`
}

// RepoRankHistory represents one history point for a repository.
type RepoRankHistory struct {
	Spot         int    `json:"spot"`
	Anchor       string `json:"anchor"`
	SnapshotDate string `json:"snapshotDate,omitempty"`
	Arrow        string `json:"arrow"`
}

func newRankHistoryStore() *RankHistoryStore {
	return &RankHistoryStore{
		Version:   rankHistoryVersion,
		Snapshots: []RankSnapshot{},
	}
}

func loadRankHistory(path string) (*RankHistoryStore, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return newRankHistoryStore(), nil
		}
		return nil, fmt.Errorf("read rank history file: %w", err)
	}

	var store RankHistoryStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("parse rank history file: %w", err)
	}

	if store.Version == 0 {
		store.Version = rankHistoryVersion
	}
	if store.Snapshots == nil {
		store.Snapshots = []RankSnapshot{}
	}

	sort.Slice(store.Snapshots, func(i, j int) bool {
		return store.Snapshots[i].Date < store.Snapshots[j].Date
	})

	return &store, nil
}

func saveRankHistory(path string, store *RankHistoryStore) error {
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal rank history: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write rank history file: %w", err)
	}
	return nil
}

func buildCurrentRanks(sorted []ProjectSummary) map[string]int {
	ranks := make(map[string]int, len(sorted))
	for i, summary := range sorted {
		ranks[summary.Name] = i + 1
	}
	return ranks
}

func upsertSnapshotForDate(store *RankHistoryStore, anchorDate time.Time, ranks map[string]int) {
	day := anchorDate.Format("2006-01-02")
	clonedRanks := make(map[string]int, len(ranks))
	for repo, rank := range ranks {
		clonedRanks[repo] = rank
	}

	for i := range store.Snapshots {
		if store.Snapshots[i].Date == day {
			store.Snapshots[i].Ranks = clonedRanks
			return
		}
	}

	store.Snapshots = append(store.Snapshots, RankSnapshot{
		Date:  day,
		Ranks: clonedRanks,
	})
	sort.Slice(store.Snapshots, func(i, j int) bool {
		return store.Snapshots[i].Date < store.Snapshots[j].Date
	})
}

func applyRankHistoryToSummaries(summaries []ProjectSummary, store *RankHistoryStore, anchorDate time.Time, points int) {
	for i := range summaries {
		summaries[i].RankHistory = computeRepoHistory(store, summaries[i].Name, anchorDate, points)
	}
}

func computeRepoHistory(store *RankHistoryStore, repo string, anchorDate time.Time, points int) []RepoRankHistory {
	history := make([]RepoRankHistory, 0, points)
	for i := 0; i < points; i++ {
		target := anchorDate.AddDate(0, 0, -7*i)
		targetDate := target.Format("2006-01-02")
		snapshot := latestSnapshotAtOrBefore(store, targetDate)

		item := RepoRankHistory{
			Anchor: anchorLabel(i),
			Arrow:  "·",
		}
		if snapshot != nil {
			item.SnapshotDate = snapshot.Date
			if spot, ok := snapshot.Ranks[repo]; ok {
				item.Spot = spot
			}
		}
		if i > 0 {
			item.Arrow = movementArrow(history[i-1].Spot, item.Spot)
		}

		history = append(history, item)
	}

	return history
}

func latestSnapshotAtOrBefore(store *RankHistoryStore, targetDate string) *RankSnapshot {
	var latest *RankSnapshot
	for i := range store.Snapshots {
		snapshot := &store.Snapshots[i]
		if snapshot.Date <= targetDate {
			if latest == nil || snapshot.Date > latest.Date {
				latest = snapshot
			}
		}
	}
	return latest
}

func movementArrow(currentSpot, olderSpot int) string {
	if currentSpot <= 0 || olderSpot <= 0 {
		return "·"
	}
	if currentSpot == olderSpot {
		return "="
	}
	if currentSpot < olderSpot {
		return "↑"
	}
	return "↓"
}

func formatRankHistoryForHeader(history []RepoRankHistory) string {
	if len(history) == 0 {
		return ""
	}

	tokens := make([]string, 0, len(history))
	for i, point := range history {
		if point.Spot <= 0 {
			continue
		}

		spot := fmt.Sprintf("#%d", point.Spot)
		if i == 0 {
			tokens = append(tokens, spot)
			continue
		}
		tokens = append(tokens, point.Arrow+spot)
	}

	if len(tokens) == 0 {
		return ""
	}

	return " · " + strings.Join(tokens, "")
}

func anchorLabel(i int) string {
	if i == 0 {
		return "now"
	}
	return fmt.Sprintf("%dw", i)
}
