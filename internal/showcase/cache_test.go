package showcase

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSummaryCache_SaveThenLoadRoundTrips(t *testing.T) {
	t.Parallel()

	c := NewSummaryCache(t.TempDir())
	want := &ProjectSummary{Name: "cpuinfo", Summary: "does cpu things"}

	if err := c.Save("cpuinfo", want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := c.Load("cpuinfo")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Name != want.Name || got.Summary != want.Summary {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestSummaryCache_LoadMissingReturnsError(t *testing.T) {
	t.Parallel()

	c := NewSummaryCache(t.TempDir())
	if _, err := c.Load("does-not-exist"); err == nil {
		t.Fatal("Load() on missing cache entry: want error, got nil")
	}
}

func TestSummaryCache_LoadAllSkipsExcludedAndMissing(t *testing.T) {
	t.Parallel()

	c := NewSummaryCache(t.TempDir())
	if err := c.Save("keep-me", &ProjectSummary{Name: "keep-me"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := c.Save("exclude-me", &ProjectSummary{Name: "exclude-me"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	isExcluded := func(repo string) bool { return repo == "exclude-me" }
	got := c.LoadAll([]string{"keep-me", "exclude-me", "never-cached"}, isExcluded)

	if len(got) != 1 {
		t.Fatalf("LoadAll() returned %d entries, want 1: %+v", len(got), got)
	}
	if _, ok := got["keep-me"]; !ok {
		t.Fatalf("LoadAll() missing expected entry %q: %+v", "keep-me", got)
	}
}

func TestSummaryCache_RankHistorySnapshotRoundTrips(t *testing.T) {
	t.Parallel()

	c := NewSummaryCache(t.TempDir())
	anchor := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	summaries := []ProjectSummary{
		{Name: "top", Metadata: &RepoMetadata{Score: 100}},
		{Name: "bottom", Metadata: &RepoMetadata{Score: 1}},
	}

	if err := c.UpdateRankHistorySnapshot(anchor, summaries); err != nil {
		t.Fatalf("UpdateRankHistorySnapshot() error = %v", err)
	}

	if err := c.ApplyRankHistory(summaries, anchor); err != nil {
		t.Fatalf("ApplyRankHistory() error = %v", err)
	}

	if len(summaries[0].RankHistory) == 0 || summaries[0].RankHistory[0].Spot != 1 {
		t.Fatalf("ApplyRankHistory() top summary history = %+v, want current spot 1", summaries[0].RankHistory)
	}
	if len(summaries[1].RankHistory) == 0 || summaries[1].RankHistory[0].Spot != 2 {
		t.Fatalf("ApplyRankHistory() bottom summary history = %+v, want current spot 2", summaries[1].RankHistory)
	}
}

func TestVerifyImages_ReportsMissingFile(t *testing.T) {
	t.Parallel()

	showcaseDir := t.TempDir()
	summary := &ProjectSummary{Images: []string{"missing.png"}}

	if err := verifyImages(summary, showcaseDir); err == nil {
		t.Fatal("verifyImages() with missing image: want error, got nil")
	}
}

func TestVerifyImages_NoErrorWhenAllImagesExist(t *testing.T) {
	t.Parallel()

	showcaseDir := t.TempDir()
	imgPath := filepath.Join(showcaseDir, "present.png")
	if err := os.WriteFile(imgPath, []byte("fake-png"), 0644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	summary := &ProjectSummary{Images: []string{"present.png"}}
	if err := verifyImages(summary, showcaseDir); err != nil {
		t.Fatalf("verifyImages() error = %v, want nil", err)
	}
}
