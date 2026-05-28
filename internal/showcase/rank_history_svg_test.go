package showcase

import (
	"strings"
	"testing"
)

// makeTestSummaries builds a small slice of ProjectSummary with rank history
// suitable for unit-testing the SVG generator without touching the filesystem.
func makeTestSummaries() []ProjectSummary {
	return []ProjectSummary{
		{
			Name: "alpha",
			RankHistory: []RepoRankHistory{
				{Spot: 1, Anchor: "now", SnapshotDate: "2026-05-27"},
				{Spot: 2, Anchor: "1w", SnapshotDate: "2026-05-20"},
				{Spot: 2, Anchor: "2w", SnapshotDate: "2026-05-13"},
				{Spot: 3, Anchor: "3w", SnapshotDate: "2026-05-06"},
				{Spot: 4, Anchor: "4w", SnapshotDate: "2026-04-29"}, // oldest data point
			},
		},
		{
			Name: "beta",
			RankHistory: []RepoRankHistory{
				{Spot: 2, Anchor: "now", SnapshotDate: "2026-05-27"},
				{Spot: 1, Anchor: "1w", SnapshotDate: "2026-05-20"},
				{Spot: 1, Anchor: "2w", SnapshotDate: "2026-05-13"},
				{Spot: 0, Anchor: "3w"},
				{Spot: 0, Anchor: "4w"},
			},
		},
		{
			// Project with no rank data at all should be skipped.
			Name:        "ghost",
			RankHistory: []RepoRankHistory{{Spot: 0}, {Spot: 0}, {Spot: 0}, {Spot: 0}, {Spot: 0}},
		},
	}
}

func TestGenerateRankHistorySVG_IsValidSVG(t *testing.T) {
	t.Parallel()

	svg := GenerateRankHistorySVG(makeTestSummaries())

	if !strings.HasPrefix(svg, "<svg ") {
		t.Fatalf("expected SVG to start with '<svg ', got prefix: %.40s", svg)
	}
	if !strings.HasSuffix(svg, "</svg>") {
		t.Fatalf("expected SVG to end with '</svg>', got suffix: %.40s", svg[len(svg)-40:])
	}
}

func TestGenerateRankHistorySVG_ContainsProjectNames(t *testing.T) {
	t.Parallel()

	svg := GenerateRankHistorySVG(makeTestSummaries())

	if !strings.Contains(svg, `"name":"alpha"`) {
		t.Error("SVG JSON data missing project 'alpha'")
	}
	if !strings.Contains(svg, `"name":"beta"`) {
		t.Error("SVG JSON data missing project 'beta'")
	}
	// 'ghost' has no valid data points and must be excluded.
	if strings.Contains(svg, `"name":"ghost"`) {
		t.Error("SVG JSON data should not contain 'ghost' (no rank data)")
	}
}

func TestGenerateRankHistorySVG_ContainsInteractiveJS(t *testing.T) {
	t.Parallel()

	svg := GenerateRankHistorySVG(makeTestSummaries())

	// Key JS functions must be present for interactivity.
	for _, fn := range []string{"onEnter", "onLeave", "moveTT"} {
		if !strings.Contains(svg, fn) {
			t.Errorf("SVG JavaScript missing function %q", fn)
		}
	}
	// Tooltip anchor elements must exist.
	for _, id := range []string{"tt", "ttbg", "tttl", "ttbd"} {
		if !strings.Contains(svg, `id="`+id+`"`) {
			t.Errorf("SVG missing element id=%q", id)
		}
	}
}

func TestGenerateRankHistorySVG_EmptySummaries(t *testing.T) {
	t.Parallel()

	svg := GenerateRankHistorySVG([]ProjectSummary{})

	// Should still produce a valid (empty-data) SVG with the subtitle showing 0 projects.
	if !strings.Contains(svg, "<svg ") {
		t.Fatal("expected valid SVG even with empty input")
	}
	if !strings.Contains(svg, "0 projects tracked") {
		t.Error("expected subtitle to mention 0 projects")
	}
}

func TestGenerateRankHistorySVG_TimeAxisLabels(t *testing.T) {
	t.Parallel()

	svg := GenerateRankHistorySVG(makeTestSummaries())

	// All five time-axis labels must appear.
	for _, label := range []string{"now", "1w ago", "2w ago", "3w ago", "4w ago"} {
		if !strings.Contains(svg, label) {
			t.Errorf("SVG missing time-axis label %q", label)
		}
	}
}

// TestBuildSVGPath_SkipsZeroSpots verifies that missing data produces M commands
// (pen-up) rather than L commands (pen-down), keeping lines broken at gaps.
func TestBuildSVGPath_SkipsZeroSpots(t *testing.T) {
	t.Parallel()

	points := []svgTimePoint{
		{Spot: 3, Label: "4w"},
		{Spot: 0, Label: "3w"}, // gap
		{Spot: 2, Label: "2w"},
		{Spot: 1, Label: "1w"},
		{Spot: 1, Label: "now"},
	}

	xPos := func(i int) float64 { return float64(i) * 100 }
	yPos := func(rank int) float64 {
		if rank <= 0 {
			return -999
		}
		return float64(rank) * 10
	}

	path := buildSVGPath(points, xPos, yPos)

	// Should start at index 0 (rank=3) with M.
	if !strings.HasPrefix(path, "M0.0 ") {
		t.Fatalf("path should start with M0.0, got: %s", path)
	}
	// After the gap the next segment must begin with M (not L).
	if !strings.Contains(path, "M200.0") {
		t.Errorf("path should re-open with M after gap at index 2, got: %s", path)
	}
	// Consecutive valid points should be joined with L.
	if !strings.Contains(path, "L300.0") {
		t.Errorf("path should continue with L at index 3, got: %s", path)
	}
}

// TestProjectColor_GoldenRatioDistribution verifies that successive colors are
// distinct (no two consecutive projects share the same hex string within 10).
func TestProjectColor_GoldenRatioDistribution(t *testing.T) {
	t.Parallel()

	seen := make(map[string]int, 30)
	for i := 0; i < 30; i++ {
		c := projectColor(i)
		if !strings.HasPrefix(c, "#") || len(c) != 7 {
			t.Errorf("projectColor(%d) = %q: expected #rrggbb format", i, c)
		}
		if prev, ok := seen[c]; ok {
			t.Errorf("projectColor(%d) == projectColor(%d) == %q: colors should be distinct", i, prev, c)
		}
		seen[c] = i
	}
}

// TestHslToRGBHex_KnownValues checks a few well-known HSL → RGB conversions.
func TestHslToRGBHex_KnownValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		h, s, l float64
		want    string
	}{
		{0, 0, 0, "#000000"},   // black
		{0, 0, 1, "#ffffff"},   // white
		{0, 1, 0.5, "#ff0000"}, // pure red
	}

	for _, tc := range tests {
		got := hslToRGBHex(tc.h, tc.s, tc.l)
		if got != tc.want {
			t.Errorf("hslToRGBHex(%.0f, %.0f, %.1f) = %q, want %q", tc.h, tc.s, tc.l, got, tc.want)
		}
	}
}

// TestGenerateRankHistorySVG_PartialRankHistory ensures that summaries with
// fewer than rankHistoryPoints history entries do not panic and produce valid
// SVG. This guards against the off-by-one reversal bug where revIdx was
// computed against numPoints (constant 5) instead of len(s.RankHistory),
// causing an index out-of-bounds when the slice was shorter.
func TestGenerateRankHistorySVG_PartialRankHistory(t *testing.T) {
	t.Parallel()

	summaries := []ProjectSummary{
		{
			Name: "partial",
			// Only 3 history entries instead of the full 5.
			RankHistory: []RepoRankHistory{
				{Spot: 1, Anchor: "now", SnapshotDate: "2026-05-27"},
				{Spot: 2, Anchor: "1w", SnapshotDate: "2026-05-20"},
				{Spot: 3, Anchor: "2w", SnapshotDate: "2026-05-13"},
			},
		},
		{
			Name: "single-entry",
			// Only 1 history entry.
			RankHistory: []RepoRankHistory{
				{Spot: 5, Anchor: "now", SnapshotDate: "2026-05-27"},
			},
		},
	}

	// Must not panic despite len(RankHistory) < rankHistoryPoints.
	var svg string
	require := func(cond bool, msg string) {
		t.Helper()
		if !cond {
			t.Fatal(msg)
		}
	}
	require(func() (ok bool) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("GenerateRankHistorySVG panicked with partial history: %v", r)
				ok = false
			}
		}()
		svg = GenerateRankHistorySVG(summaries)
		return true
	}(), "GenerateRankHistorySVG must not panic with partial RankHistory")

	require(strings.Contains(svg, `"name":"partial"`), "SVG should include 'partial' project")
	require(strings.Contains(svg, `"name":"single-entry"`), "SVG should include 'single-entry' project")
}

// TestXLabelStep_ReturnsSensibleSteps verifies the X-axis label density
// function thins out labels as the number of display columns grows.
func TestXLabelStep_ReturnsSensibleSteps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		displayPoints int
		wantMax       int // step must be ≤ wantMax
	}{
		{5, 1},
		{10, 2},
		{20, 4},
		{32, 8},
	}

	for _, tc := range tests {
		got := xLabelStep(tc.displayPoints)
		if got > tc.wantMax {
			t.Errorf("xLabelStep(%d) = %d, want ≤ %d", tc.displayPoints, got, tc.wantMax)
		}
		if got <= 0 {
			t.Errorf("xLabelStep(%d) = %d, want > 0", tc.displayPoints, got)
		}
	}
}

// TestGridStep_ReturnsSensibleSteps ensures the Y-axis step function keeps
// the graph legible across various project counts.
func TestGridStep_ReturnsSensibleSteps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		maxRank int
		wantMax int // step must be ≤ wantMax
	}{
		{5, 1},
		{10, 2},
		{20, 3},
		{40, 5},
	}

	for _, tc := range tests {
		got := gridStep(tc.maxRank)
		if got > tc.wantMax {
			t.Errorf("gridStep(%d) = %d, want ≤ %d", tc.maxRank, got, tc.wantMax)
		}
		if got <= 0 {
			t.Errorf("gridStep(%d) = %d, want > 0", tc.maxRank, got)
		}
	}
}

func TestBuildProjectData_MapsHistoryAndMetadata(t *testing.T) {
	t.Parallel()

	summaries := []ProjectSummary{
		{
			Name: "alpha",
			Metadata: &RepoMetadata{
				Score: 42.5,
			},
			RankHistory: []RepoRankHistory{
				{Spot: 1, Anchor: "now", SnapshotDate: "2026-05-27"},
				{Spot: 3, Anchor: "1w", SnapshotDate: "2026-05-20"},
			},
		},
		{
			Name: "dormant",
			Metadata: &RepoMetadata{
				AvgCommitAge: 1200,
				Score:        1.5,
			},
			RankHistory: []RepoRankHistory{
				{Spot: 4, Anchor: "now", SnapshotDate: "2026-05-27"},
			},
		},
		{
			Name:        "ghost",
			RankHistory: []RepoRankHistory{{Spot: 0}, {Spot: 0}},
		},
	}

	projects, maxRank := buildProjectData(summaries, 5)

	if maxRank != 4 {
		t.Fatalf("maxRank = %d, want 4", maxRank)
	}
	if len(projects) != 2 {
		t.Fatalf("len(projects) = %d, want 2 (ghost should be skipped)", len(projects))
	}

	alpha := projects[0]
	if alpha.Name != "alpha" {
		t.Fatalf("first project name = %q, want alpha", alpha.Name)
	}
	if alpha.Score != 42.5 {
		t.Fatalf("alpha score = %.1f, want 42.5", alpha.Score)
	}
	if alpha.Inactive {
		t.Fatal("alpha should be active")
	}
	if alpha.Points[4].Label != "now" || alpha.Points[4].Spot != 1 {
		t.Fatalf("alpha now point = %+v, want label=now spot=1", alpha.Points[4])
	}
	if alpha.Points[3].Label != "1w" || alpha.Points[3].Spot != 3 {
		t.Fatalf("alpha 1w point = %+v, want label=1w spot=3", alpha.Points[3])
	}

	dormant := projects[1]
	if !dormant.Inactive {
		t.Fatal("dormant should be inactive")
	}
}

func TestTrimLeadingEmptyColumns_EdgeCases(t *testing.T) {
	t.Parallel()

	noProjects := []svgProjectData{}
	if got := trimLeadingEmptyColumns(noProjects, 5); got != 1 {
		t.Fatalf("displayPoints for empty projects = %d, want 1", got)
	}

	projects := []svgProjectData{
		{
			Name: "alpha",
			Points: []svgTimePoint{
				{Spot: 0}, {Spot: 0}, {Spot: 0}, {Spot: 2}, {Spot: 1},
			},
		},
	}

	displayPoints := trimLeadingEmptyColumns(projects, 5)
	if displayPoints != 2 {
		t.Fatalf("displayPoints = %d, want 2", displayPoints)
	}
	if len(projects[0].Points) != 2 {
		t.Fatalf("trimmed point count = %d, want 2", len(projects[0].Points))
	}
	if projects[0].Points[0].Spot != 2 || projects[0].Points[1].Spot != 1 {
		t.Fatalf("trimmed points = %+v, want spots [2 1]", projects[0].Points)
	}
}

func TestBuildXLabels_SinglePoint(t *testing.T) {
	t.Parallel()

	labels := buildXLabels(1)
	if len(labels) != 1 || labels[0] != "now" {
		t.Fatalf("buildXLabels(1) = %#v, want [\"now\"]", labels)
	}
}

func TestRenderAxes_LabelThinningBoundaryTransitions(t *testing.T) {
	t.Parallel()

	xPos := func(i int) float64 { return float64(100 + i*10) }
	yPos := func(rank int) float64 { return float64(200 - rank*5) }

	tests := []struct {
		displayPoints int
		wantLabels    int
	}{
		// xLabelStep transitions: 1 -> 2 at 7, 2 -> 4 at 13, 4 -> 8 at 25.
		{displayPoints: 6, wantLabels: 6},
		{displayPoints: 7, wantLabels: 4},
		{displayPoints: 12, wantLabels: 7},
		{displayPoints: 13, wantLabels: 4},
		{displayPoints: 24, wantLabels: 7},
		{displayPoints: 25, wantLabels: 4},
	}

	for _, tc := range tests {
		_, xAxis := renderAxes(5, tc.displayPoints, 100, 60, xPos, yPos, buildXLabels(tc.displayPoints))

		if got := strings.Count(xAxis, `<line class="gl"`); got != tc.displayPoints {
			t.Errorf("displayPoints=%d: x-axis grid line count = %d, want %d", tc.displayPoints, got, tc.displayPoints)
		}
		if got := strings.Count(xAxis, `<text class="al"`); got != tc.wantLabels {
			t.Errorf("displayPoints=%d: x-axis label count = %d, want %d", tc.displayPoints, got, tc.wantLabels)
		}
		if got := strings.Count(xAxis, ">now</text>"); got != 1 {
			t.Errorf("displayPoints=%d: expected exactly one 'now' label, got %d", tc.displayPoints, got)
		}
	}
}

func TestRenderAxes_SinglePointAxisRendering(t *testing.T) {
	t.Parallel()

	xPos := func(int) float64 { return 123.4 }
	yPos := func(rank int) float64 { return float64(rank * 10) }

	grid, xAxis := renderAxes(3, 1, 100, 80, xPos, yPos, []string{"now"})
	if grid == "" {
		t.Fatal("expected non-empty Y-axis/grid SVG for single-point chart")
	}
	if got := strings.Count(xAxis, `<line class="gl"`); got != 1 {
		t.Fatalf("single-point x-axis should render exactly 1 vertical grid line, got %d", got)
	}
	if got := strings.Count(xAxis, `<text class="al"`); got != 1 {
		t.Fatalf("single-point x-axis should render exactly 1 label, got %d", got)
	}
	if !strings.Contains(xAxis, `x1="123.4"`) {
		t.Fatalf("single-point x-axis should use provided x position, got: %s", xAxis)
	}
	if !strings.Contains(xAxis, ">now</text>") {
		t.Fatalf("single-point x-axis should render 'now' label, got: %s", xAxis)
	}
}

func TestRenderLines_EmptyAndAllInvalid(t *testing.T) {
	t.Parallel()

	xPos := func(i int) float64 { return float64(i * 10) }

	if got := renderLines(nil, xPos, func(rank int) float64 { return float64(rank * 5) }); got != "" {
		t.Fatalf("renderLines(nil, ...) = %q, want empty string", got)
	}

	projects := []svgProjectData{
		{
			Name:  "zeroes",
			Color: "#111111",
			Points: []svgTimePoint{
				{Spot: 0},
				{Spot: 0},
			},
		},
		{
			Name:  "offchart",
			Color: "#222222",
			Points: []svgTimePoint{
				{Spot: 1},
				{Spot: 2},
			},
		},
	}
	if got := renderLines(projects, xPos, func(int) float64 { return -1 }); got != "" {
		t.Fatalf("renderLines(all-invalid, ...) = %q, want empty string", got)
	}
}
