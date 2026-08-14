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

func TestFormatGemtext_OmitsRankHistoryFromHeader(t *testing.T) {
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

	if !strings.Contains(content, "### 1. alpha\n") {
		t.Fatalf("header should just be the repo name: %s", content)
	}
	if strings.ContainsAny(content, "↖↙←") {
		t.Fatalf("rank history arrows should no longer be rendered in header: %s", content)
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

func TestFormatGemtext_IncludesForgejoLinkAndHint(t *testing.T) {
	t.Parallel()

	g := &Generator{config: &config.Config{}}
	content := g.formatGemtext([]ProjectSummary{
		{
			Name:       "ggaze",
			Summary:    "summary",
			ForgejoURL: "https://code.f3s.buetow.org/snonux/ggaze",
		},
	})

	if !strings.Contains(content, "=> https://code.f3s.buetow.org/snonux/ggaze View on Forgejo\n") {
		t.Fatalf("Forgejo link was not rendered: %s", content)
	}
	if !strings.Contains(content, "For Forgejo access go to code dot f3s dot buetow dot org slash snonux slash ggaze\n") {
		t.Fatalf("Forgejo hint text was not rendered: %s", content)
	}
}

func TestBuildProjectLinks_UsesForgejoOrganization(t *testing.T) {
	t.Parallel()

	g := &Generator{config: &config.Config{Organizations: []config.Organization{{
		Host: "ssh://git@code.f3s.buetow.org:2022", ForgejoAPIBase: "https://code.f3s.buetow.org/api/v1", ForgejoOwner: "snonux", BackupLocation: true,
	}}}}
	_, _, forgejoURL := g.buildProjectLinks("cpuinfo", "")

	if forgejoURL != "https://code.f3s.buetow.org/snonux/cpuinfo" {
		t.Fatalf("buildProjectLinks() Forgejo URL = %q, want %q", forgejoURL, "https://code.f3s.buetow.org/snonux/cpuinfo")
	}
}

func TestBuildProjectLinks_LegacyShowcaseHostIsIgnored(t *testing.T) {
	t.Parallel()

	g := &Generator{
		config: &config.Config{ShowcaseCgitHost: "https://legacy.example/git/"},
	}
	_, _, forgejoURL := g.buildProjectLinks("cpuinfo", "")

	if forgejoURL != "" {
		t.Fatalf("buildProjectLinks() Forgejo URL = %q without a Forgejo organization, want empty", forgejoURL)
	}
}

func TestBuildProjectLinks_CodebergLinkOnlyWhenSyncedToCodeberg(t *testing.T) {
	t.Parallel()

	// Build a throwaway git repo with a codeberg remote so that
	// repoHasCodebergRemote detects an active Codeberg sync.
	repoPath := t.TempDir()
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, args := range [][]string{
		{"init"},
		{"remote", "add", "codeberg_org", "git@codeberg.org:snonux/cpuinfo.git"},
	} {
		cmd := exec.Command("git", append([]string{"-C", repoPath}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	codebergOrg := config.Organization{Host: "git@codeberg.org", Name: "snonux"}

	// Codeberg sync disabled: no codeberg link even with a codeberg remote and
	// the repo in the allowlist.
	disabled := &Generator{config: &config.Config{
		Organizations: []config.Organization{codebergOrg},
		Repositories:  []string{"cpuinfo"},
	}}
	if codebergURL, _, _ := disabled.buildProjectLinks("cpuinfo", repoPath); codebergURL != "" {
		t.Fatalf("disabled buildProjectLinks() codeberg URL = %q, want empty", codebergURL)
	}

	// Codeberg sync enabled and repo is in the allowlist and has a codeberg
	// remote: link emitted.
	enabled := &Generator{config: &config.Config{
		Organizations: []config.Organization{codebergOrg},
		Repositories:  []string{"cpuinfo"},
		SyncCodeberg:  true,
	}}
	codebergURL, _, _ := enabled.buildProjectLinks("cpuinfo", repoPath)
	want := "https://codeberg.org/snonux/cpuinfo"
	if codebergURL != want {
		t.Fatalf("enabled buildProjectLinks() codeberg URL = %q, want %q", codebergURL, want)
	}

	// Codeberg sync enabled and repo has a codeberg remote, but repo is NOT in
	// the allowlist: no link (it is not one of the repos we sync with Codeberg).
	if codebergURL, _, _ := enabled.buildProjectLinks("other-repo", repoPath); codebergURL != "" {
		t.Fatalf("non-allowlisted buildProjectLinks() codeberg URL = %q, want empty", codebergURL)
	}

	// Codeberg sync enabled, repo in allowlist, but repo has no codeberg remote
	// (not actually on Codeberg yet): no link (would point to a dead URL).
	plainRepo := t.TempDir()
	if err := os.MkdirAll(plainRepo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if out, err := exec.Command("git", "-C", plainRepo, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if codebergURL, _, _ := enabled.buildProjectLinks("cpuinfo", plainRepo); codebergURL != "" {
		t.Fatalf("buildProjectLinks() codeberg URL = %q, want empty when repo not on Codeberg", codebergURL)
	}

	// Discovery mode (Repositories empty): a repo with a codeberg remote still
	// gets a link when sync_codeberg is enabled.
	discovery := &Generator{config: &config.Config{
		Organizations: []config.Organization{codebergOrg},
		SyncCodeberg:  true,
	}}
	if codebergURL, _, _ := discovery.buildProjectLinks("cpuinfo", repoPath); codebergURL != want {
		t.Fatalf("discovery buildProjectLinks() codeberg URL = %q, want %q", codebergURL, want)
	}
}

func TestFormatGemtext_ZeroProjectsReleasePercentagesAreZero(t *testing.T) {
	t.Parallel()

	g := &Generator{config: &config.Config{}}
	content := g.formatGemtext(nil)

	if !strings.Contains(content, "* 🚀 Release Status: 0 released, 0 experimental (0.0% with releases, 0.0% experimental)\n") {
		t.Fatalf("unexpected release status for zero projects: %s", content)
	}
	if strings.Contains(content, "NaN") || strings.Contains(content, "Inf") {
		t.Fatalf("release status should not include NaN/Inf: %s", content)
	}
}

func TestFormatGemtext_ReleaseStatusPercentagesForNonEmptySummaries(t *testing.T) {
	t.Parallel()

	g := &Generator{config: &config.Config{}}
	content := g.formatGemtext([]ProjectSummary{
		{
			Name:    "released",
			Summary: "released summary",
			Metadata: &RepoMetadata{
				HasReleases: true,
			},
		},
		{
			Name:    "experimental",
			Summary: "experimental summary",
			Metadata: &RepoMetadata{
				HasReleases: false,
			},
		},
	})

	if !strings.Contains(content, "* 🚀 Release Status: 1 released, 1 experimental (50.0% with releases, 50.0% experimental)\n") {
		t.Fatalf("unexpected release status for non-empty summaries: %s", content)
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

func TestResolveSummary_CachedSummaryFallsBackToReadmeWhenNoUsefulParagraph(t *testing.T) {
	t.Parallel()

	g := &Generator{}
	readmeContent := []byte("useful README summary paragraph")

	got := g.resolveSummary(
		"repo",
		t.TempDir(),
		nil, // cache hit: the chain is never consulted
		readmeContent,
		true,
		"* item one\n* item two",
		true,
	)

	if got != "useful README summary paragraph" {
		t.Fatalf("resolveSummary() = %q, want README fallback", got)
	}
}

func TestResolveSummary_NoCacheAndNoReadmeUsesGenericFallback(t *testing.T) {
	t.Parallel()

	g := &Generator{}

	got := g.resolveSummary(
		"repo",
		t.TempDir(),
		nil, // empty chain: RunChain fails immediately without invoking any real tool
		nil,
		false,
		"",
		false,
	)

	if got != "repo: source code repository." {
		t.Fatalf("resolveSummary() = %q, want generic fallback", got)
	}
}

func TestCollectAssets_ReturnsImagesAndSkipsSnippetWithoutLanguages(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	repoName := "demo"
	repoPath := filepath.Join(t.TempDir(), repoName)
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("![screenshot](shot.png)"), 0644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "shot.png"), []byte("png"), 0644); err != nil {
		t.Fatalf("write shot.png: %v", err)
	}

	g := &Generator{}
	images, snippet, language, err := g.collectAssets(repoName, repoPath, repoPath, nil)
	if err != nil {
		t.Fatalf("collectAssets() error = %v", err)
	}

	if len(images) != 1 || images[0] != filepath.Join("showcase", repoName, "image-1.png") {
		t.Fatalf("collectAssets() images = %#v", images)
	}
	if snippet != "" || language != "" {
		t.Fatalf("collectAssets() snippet/language = %q/%q, want empty", snippet, language)
	}

	copiedPath := filepath.Join(homeDir, "git", "foo.zone-content", "gemtext", "about", images[0])
	if _, err := os.Stat(copiedPath); err != nil {
		t.Fatalf("expected copied image at %s: %v", copiedPath, err)
	}
}

func TestCollectAssets_ContinuesWhenSnippetExtractionFails(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	repoName := "demo"
	repoPath := filepath.Join(t.TempDir(), repoName)
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("![missing](missing.png)"), 0644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}

	g := &Generator{}
	metadata := &RepoMetadata{
		Languages: []LanguageStats{
			{Name: "Go", Lines: 10, Percentage: 100},
		},
	}

	images, snippet, language, err := g.collectAssets(repoName, repoPath, repoPath, metadata)
	if err != nil {
		t.Fatalf("collectAssets() error = %v", err)
	}
	if len(images) != 0 {
		t.Fatalf("collectAssets() images = %#v, want no images", images)
	}
	if snippet != "" || language != "" {
		t.Fatalf("collectAssets() snippet/language = %q/%q, want empty after extraction error", snippet, language)
	}
}

func TestCollectAssets_UsesConfiguredShowcaseOutputDir(t *testing.T) {
	repoName := "demo"
	repoPath := filepath.Join(t.TempDir(), repoName)
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("![screenshot](shot.png)"), 0644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "shot.png"), []byte("png"), 0644); err != nil {
		t.Fatalf("write shot.png: %v", err)
	}

	customOutputDir := filepath.Join(t.TempDir(), "custom-showcase-output")
	g := &Generator{
		config: &config.Config{
			ShowcaseOutputDir: customOutputDir,
		},
	}

	images, snippet, language, err := g.collectAssets(repoName, repoPath, repoPath, nil)
	if err != nil {
		t.Fatalf("collectAssets() error = %v", err)
	}
	if len(images) != 1 || images[0] != filepath.Join("showcase", repoName, "image-1.png") {
		t.Fatalf("collectAssets() images = %#v", images)
	}
	if snippet != "" || language != "" {
		t.Fatalf("collectAssets() snippet/language = %q/%q, want empty", snippet, language)
	}

	copiedPath := filepath.Join(customOutputDir, images[0])
	if _, err := os.Stat(copiedPath); err != nil {
		t.Fatalf("expected copied image at %s: %v", copiedPath, err)
	}
}

// AI-tool selection (Chain/AvailableChain/FirstAvailable) is now generic
// logic that lives in and is tested by internal/aitool; this package only
// consumes it (see generateProjectSummary/resolveSummary), so it no longer
// needs its own copy of these selection tests.

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
