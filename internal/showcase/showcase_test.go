package showcase

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"codeberg.org/snonux/gitsyncer/internal/config"
)

func TestIsBackupRepo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		repo string
		want bool
	}{
		{name: "exact bak suffix", repo: "foo.bak", want: true},
		{name: "bak dot suffix", repo: "foo.bak.20260222", want: true},
		{name: "bak dot with multiple segments", repo: "foo.bak.tmp.snapshot", want: true},
		{name: "backup word", repo: "foo.backup", want: false},
		{name: "bak as prefix", repo: "bak.foo", want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isBackupRepo(tc.repo)
			if got != tc.want {
				t.Fatalf("isBackupRepo(%q) = %v, want %v", tc.repo, got, tc.want)
			}
		})
	}
}

func TestIsExcluded_AdditiveRules(t *testing.T) {
	t.Parallel()

	g := &Generator{
		config: &config.Config{
			ExcludeFromShowcase: []string{"manual-exclude"},
		},
	}

	tests := []struct {
		name string
		repo string
		want bool
	}{
		{name: "excluded by config", repo: "manual-exclude", want: true},
		{name: "excluded by backup suffix", repo: "repo.bak", want: true},
		{name: "not excluded", repo: "normal-repo", want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := g.isExcluded(tc.repo)
			if got != tc.want {
				t.Fatalf("isExcluded(%q) = %v, want %v", tc.repo, got, tc.want)
			}
		})
	}
}

func TestFilterExcludedRepos_RemovesBackupAndConfigRepos(t *testing.T) {
	t.Parallel()

	g := &Generator{
		config: &config.Config{
			ExcludeFromShowcase: []string{"manual-exclude"},
		},
	}

	repos := []string{"normal", "manual-exclude", "mirror.bak", "mirror.bak.20260222", "keep"}
	want := []string{"normal", "keep"}

	got := g.filterExcludedRepos(repos)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterExcludedRepos() = %#v, want %#v", got, want)
	}
}

func TestFilterExcludedRepos_EmptyConfigStillRemovesBackupRepos(t *testing.T) {
	t.Parallel()

	g := &Generator{
		config: &config.Config{},
	}

	repos := []string{"normal", "archive.bak", "archive.bak.old", "keep"}
	want := []string{"normal", "keep"}

	got := g.filterExcludedRepos(repos)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterExcludedRepos() = %#v, want %#v", got, want)
	}
}

func TestFormatGemtext_IncludesRankHistoryInHeader(t *testing.T) {
	t.Parallel()

	g := &Generator{config: &config.Config{}}
	content := g.formatGemtext([]ProjectSummary{
		{
			Name:    "alpha",
			Summary: "alpha summary",
			RankHistory: []RepoRankHistory{
				{Spot: 1, Anchor: "now"},
				{Spot: 2, Anchor: "1w", Arrow: "↑"},
				{Spot: 2, Anchor: "2w", Arrow: "→"},
				{Spot: 0, Anchor: "3w", Arrow: "·"},
				{Spot: 4, Anchor: "4w", Arrow: "↓"},
			},
		},
	})

	if !strings.Contains(content, "### 1. alpha [#1(now) ↑#2(1w) →#2(2w) ·n/a(3w) ↓#4(4w)]") {
		t.Fatalf("rank history was not rendered in header: %s", content)
	}
}

func TestFindReadmeContent_UsesRepoPathWithoutChangingCWD(t *testing.T) {
	t.Parallel()

	repoPath := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	readmePath := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(readmePath, []byte("repo summary"), 0644); err != nil {
		t.Fatalf("failed to write readme: %v", err)
	}

	content, readmeFile, found := findReadmeContent(repoPath)
	if !found {
		t.Fatal("expected README to be found")
	}
	if readmeFile != "README.md" {
		t.Fatalf("expected README.md, got %q", readmeFile)
	}
	if string(content) != "repo summary" {
		t.Fatalf("unexpected README content: %q", string(content))
	}
}

func TestFallbackSummary_UsesFirstReadmeParagraph(t *testing.T) {
	t.Parallel()

	readme := []byte("first paragraph\n\nsecond paragraph")
	summary := fallbackSummary("repo", readme, true)

	if summary != "first paragraph" {
		t.Fatalf("expected first paragraph summary, got %q", summary)
	}
}
