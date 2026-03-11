package showcase

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMovementArrow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current int
		older   int
		want    string
	}{
		{name: "same spot", current: 3, older: 3, want: "→"},
		{name: "improved", current: 2, older: 5, want: "↑"},
		{name: "worse", current: 6, older: 4, want: "↓"},
		{name: "missing older", current: 2, older: 0, want: "·"},
		{name: "missing current", current: 0, older: 2, want: "·"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := movementArrow(tc.current, tc.older)
			if got != tc.want {
				t.Fatalf("movementArrow(%d,%d) = %q, want %q", tc.current, tc.older, got, tc.want)
			}
		})
	}
}

func TestComputeRepoHistory_LatestSnapshotAtOrBeforeAnchor(t *testing.T) {
	t.Parallel()

	store := &RankHistoryStore{
		Version: rankHistoryVersion,
		Snapshots: []RankSnapshot{
			{Date: "2026-01-18", Ranks: map[string]int{"alpha": 4}},
			{Date: "2026-02-01", Ranks: map[string]int{"alpha": 3}},
			{Date: "2026-02-15", Ranks: map[string]int{"alpha": 2}},
			{Date: "2026-02-22", Ranks: map[string]int{"alpha": 1}},
		},
	}

	anchor, _ := time.Parse("2006-01-02", "2026-02-22")
	history := computeRepoHistory(store, "alpha", anchor, 5)

	wantSpots := []int{1, 2, 3, 3, 4}
	if len(history) != len(wantSpots) {
		t.Fatalf("history len = %d, want %d", len(history), len(wantSpots))
	}
	for i, want := range wantSpots {
		if history[i].Spot != want {
			t.Fatalf("history[%d].Spot = %d, want %d", i, history[i].Spot, want)
		}
	}
}

func TestComputeRepoHistory_UsesNAWhenNoRepoSpotAtAnchor(t *testing.T) {
	t.Parallel()

	store := &RankHistoryStore{
		Version: rankHistoryVersion,
		Snapshots: []RankSnapshot{
			{Date: "2026-02-22", Ranks: map[string]int{"alpha": 1}},
			{Date: "2026-02-15", Ranks: map[string]int{"beta": 3}},
		},
	}

	anchor, _ := time.Parse("2006-01-02", "2026-02-22")
	history := computeRepoHistory(store, "alpha", anchor, 2)

	if history[0].Spot != 1 {
		t.Fatalf("history[0].Spot = %d, want 1", history[0].Spot)
	}
	if history[1].Spot != 0 {
		t.Fatalf("history[1].Spot = %d, want 0", history[1].Spot)
	}
	if history[1].Arrow != "·" {
		t.Fatalf("history[1].Arrow = %q, want %q", history[1].Arrow, "·")
	}
}

func TestUpsertSnapshotForDate_Idempotent(t *testing.T) {
	t.Parallel()

	store := newRankHistoryStore()
	anchor, _ := time.Parse("2006-01-02", "2026-02-22")

	upsertSnapshotForDate(store, anchor, map[string]int{"a": 1})
	upsertSnapshotForDate(store, anchor, map[string]int{"a": 2, "b": 1})

	if len(store.Snapshots) != 1 {
		t.Fatalf("len(store.Snapshots) = %d, want 1", len(store.Snapshots))
	}
	if got := store.Snapshots[0].Ranks["a"]; got != 2 {
		t.Fatalf("store.Snapshots[0].Ranks[\"a\"] = %d, want 2", got)
	}
}

func TestRankHistoryReadWriteRoundTrip(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	path := filepath.Join(tmp, rankHistoryFilename)
	store := &RankHistoryStore{
		Version: rankHistoryVersion,
		Snapshots: []RankSnapshot{
			{Date: "2026-02-22", Ranks: map[string]int{"alpha": 1}},
		},
	}
	if err := saveRankHistory(path, store); err != nil {
		t.Fatalf("saveRankHistory() error = %v", err)
	}

	got, err := loadRankHistory(path)
	if err != nil {
		t.Fatalf("loadRankHistory() error = %v", err)
	}
	if got.Version != rankHistoryVersion {
		t.Fatalf("got.Version = %d, want %d", got.Version, rankHistoryVersion)
	}
	if len(got.Snapshots) != 1 {
		t.Fatalf("len(got.Snapshots) = %d, want 1", len(got.Snapshots))
	}
}

func TestFormatRankHistoryForHeader(t *testing.T) {
	t.Parallel()

	header := formatRankHistoryForHeader([]RepoRankHistory{
		{Spot: 3, Anchor: "now"},
		{Spot: 2, Anchor: "1w", Arrow: "↓"},
		{Spot: 2, Anchor: "2w", Arrow: "→"},
		{Spot: 0, Anchor: "3w", Arrow: "·"},
	})

	if !strings.Contains(header, "#3(now)") {
		t.Fatalf("header missing current spot: %s", header)
	}
	if !strings.Contains(header, "↓#2(1w)") {
		t.Fatalf("header missing down movement: %s", header)
	}
	if strings.Contains(header, "n/a") {
		t.Fatalf("header should omit missing history points: %s", header)
	}
}
