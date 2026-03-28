package showcase

import (
	"os"
	"os/exec"
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
				{Spot: 2, Anchor: "now"},
				{Spot: 3, Anchor: "1w", Arrow: "↖"},
				{Spot: 3, Anchor: "2w", Arrow: "←"},
				{Spot: 0, Anchor: "3w", Arrow: "·"},
				{Spot: 2, Anchor: "4w", Arrow: "↙"},
			},
		},
	})

	if !strings.Contains(content, "### 1. alpha 2↖3←3↙2") {
		t.Fatalf("rank history was not rendered in header: %s", content)
	}
}

func TestFormatGemtext_SanitizesMarkdownHeadingsInSummary(t *testing.T) {
	t.Parallel()

	g := &Generator{config: &config.Config{}}
	content := g.formatGemtext([]ProjectSummary{
		{
			Name:    "alpha",
			Summary: "# Alpha Project\n\nconf\n====\n\nParagraph body",
		},
	})

	if strings.Contains(content, "\n# Alpha Project\n") {
		t.Fatalf("markdown heading leaked into gemtext summary: %s", content)
	}
	if strings.Contains(content, "\n====\n") {
		t.Fatalf("setext underline leaked into gemtext summary: %s", content)
	}
	if !strings.Contains(content, "\nAlpha Project\n\nconf\n\nParagraph body\n\n") {
		t.Fatalf("sanitized summary not rendered as expected: %s", content)
	}
}

func TestFormatGemtext_IncludesCgitLink(t *testing.T) {
	t.Parallel()

	g := &Generator{config: &config.Config{}}
	content := g.formatGemtext([]ProjectSummary{
		{
			Name:        "cpuinfo",
			Summary:     "summary",
			CodebergURL: "https://codeberg.org/snonux/cpuinfo",
			GitHubURL:   "https://github.com/snonux/cpuinfo",
			CgitURL:     "https://cgit.f3s.buetow.org/cpuinfo/",
		},
	})

	if !strings.Contains(content, "=> https://cgit.f3s.buetow.org/cpuinfo/ View in cgit\n") {
		t.Fatalf("cgit link was not rendered: %s", content)
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

func TestFallbackSummary_SkipsHeadingOnlyParagraphs(t *testing.T) {
	t.Parallel()

	readme := []byte("# repo title\n\n<img src=\"shot.png\" />\n\nactual summary paragraph")
	summary := fallbackSummary("repo", readme, true)

	if summary != "actual summary paragraph" {
		t.Fatalf("expected summary paragraph after heading and image, got %q", summary)
	}
}

func TestExtractUsefulSummary_SkipsNonProseParagraphs(t *testing.T) {
	t.Parallel()

	input := "<p align=\"center\">\n<img src=\"shot.png\" />\n</p>\n\n* first bullet\n* second bullet\n\nTOC:\n01. Intro\n02. Usage\n\nActual summary paragraph.\n\nSecond useful paragraph."
	got := extractUsefulSummary(input, 2)
	want := "Actual summary paragraph.\n\nSecond useful paragraph."

	if got != want {
		t.Fatalf("extractUsefulSummary() = %q, want %q", got, want)
	}
}

func TestExtractUsefulSummary_NormalizesManpageNameSection(t *testing.T) {
	t.Parallel()

	input := "NAME\n    cpuinfo - A small and humble tool to print out CPU data"
	got := extractUsefulSummary(input, 1)
	want := "cpuinfo - A small and humble tool to print out CPU data"

	if got != want {
		t.Fatalf("extractUsefulSummary() = %q, want %q", got, want)
	}
}

func TestExtractUsefulSummary_SkipsFencedCodeBlocks(t *testing.T) {
	t.Parallel()

	input := "```sh\nsudo dnf install wireguard-tools\nbundler install\n```\n\nActual summary paragraph."
	got := extractUsefulSummary(input, 1)
	want := "Actual summary paragraph."

	if got != want {
		t.Fatalf("extractUsefulSummary() = %q, want %q", got, want)
	}
}

func TestPrepareStatsRepoPath_UsesConfiguredBranchWithoutChangingMainCheckout(t *testing.T) {
	t.Parallel()

	repoPath := t.TempDir()
	runGit(t, repoPath, "init", "--initial-branch=main")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "user.email", "test@example.com")

	mainFile := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(mainFile, []byte("main branch"), 0644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	runGit(t, repoPath, "add", "README.md")
	runGit(t, repoPath, "commit", "-m", "main")

	runGit(t, repoPath, "checkout", "-b", "content-gemtext")
	branchOnlyFile := filepath.Join(repoPath, "branch-only.txt")
	if err := os.WriteFile(branchOnlyFile, []byte("content branch"), 0644); err != nil {
		t.Fatalf("write branch-only.txt: %v", err)
	}
	runGit(t, repoPath, "add", "branch-only.txt")
	runGit(t, repoPath, "commit", "-m", "content branch")
	runGit(t, repoPath, "checkout", "main")

	g := &Generator{
		config: &config.Config{
			ShowcaseStatsBranches: map[string]string{
				"foo.zone": "content-gemtext",
			},
		},
	}

	statsRepoPath, cleanup, err := g.prepareStatsRepoPath("foo.zone", repoPath)
	if err != nil {
		t.Fatalf("prepareStatsRepoPath() error = %v", err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Fatalf("cleanup() error = %v", err)
		}
	}()

	if statsRepoPath == repoPath {
		t.Fatal("expected a detached worktree path for configured stats branch")
	}
	if _, err := os.Stat(filepath.Join(statsRepoPath, "branch-only.txt")); err != nil {
		t.Fatalf("expected branch-only file in detached worktree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoPath, "branch-only.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected branch-only file to stay absent from main checkout, stat err = %v", err)
	}

	currentBranch := strings.TrimSpace(runGit(t, repoPath, "branch", "--show-current"))
	if currentBranch != "main" {
		t.Fatalf("current branch = %q, want %q", currentBranch, "main")
	}
}

func TestPrepareStatsRepoPath_UsesRemoteTrackingBranchWhenLocalBranchMissing(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	seedRepoPath := filepath.Join(rootDir, "seed")
	runGit(t, rootDir, "init", "--initial-branch=main", seedRepoPath)
	runGit(t, seedRepoPath, "config", "user.name", "Test User")
	runGit(t, seedRepoPath, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(seedRepoPath, "README.md"), []byte("main branch"), 0644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	runGit(t, seedRepoPath, "add", "README.md")
	runGit(t, seedRepoPath, "commit", "-m", "main")

	runGit(t, seedRepoPath, "checkout", "-b", "content-gemtext")
	if err := os.WriteFile(filepath.Join(seedRepoPath, "branch-only.txt"), []byte("content branch"), 0644); err != nil {
		t.Fatalf("write branch-only.txt: %v", err)
	}
	runGit(t, seedRepoPath, "add", "branch-only.txt")
	runGit(t, seedRepoPath, "commit", "-m", "content branch")
	runGit(t, seedRepoPath, "checkout", "main")

	remoteRepoPath := filepath.Join(rootDir, "remote.git")
	cloneCmd := exec.Command("git", "clone", "--bare", seedRepoPath, remoteRepoPath)
	if output, err := cloneCmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone --bare failed: %v\n%s", err, string(output))
	}

	cloneRepoPath := filepath.Join(rootDir, "clone")
	workingCloneCmd := exec.Command("git", "clone", remoteRepoPath, cloneRepoPath)
	if output, err := workingCloneCmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone failed: %v\n%s", err, string(output))
	}

	g := &Generator{
		config: &config.Config{
			ShowcaseStatsBranches: map[string]string{
				"foo.zone": "content-gemtext",
			},
		},
	}

	statsRepoPath, cleanup, err := g.prepareStatsRepoPath("foo.zone", cloneRepoPath)
	if err != nil {
		t.Fatalf("prepareStatsRepoPath() error = %v", err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Fatalf("cleanup() error = %v", err)
		}
	}()

	if _, err := os.Stat(filepath.Join(statsRepoPath, "branch-only.txt")); err != nil {
		t.Fatalf("expected branch-only file in detached worktree from remote branch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cloneRepoPath, "branch-only.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected branch-only file to stay absent from main checkout, stat err = %v", err)
	}

	currentBranch := strings.TrimSpace(runGit(t, cloneRepoPath, "branch", "--show-current"))
	if currentBranch != "main" {
		t.Fatalf("current branch = %q, want %q", currentBranch, "main")
	}
}
