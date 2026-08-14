package release

import "testing"

// TestCategorizeCommits verifies that commit subjects are bucketed into
// features/fixes/other based on their Conventional Commit type prefix,
// covering both unscoped ("feat: ...") and scoped ("feat(scope): ...")
// forms. This guards against regressions like the one where scoped
// commits (e.g. "feat(sync): ...", "fix(config): ...") fell through to
// "Other" because the scope in parentheses broke a plain HasPrefix match.
func TestCategorizeCommits(t *testing.T) {
	commits := []string{
		"feat: add sync command",
		"feat(sync): add scoped sync support",
		"feature: add legacy alias support",
		"feature(cli): add legacy scoped alias support",
		"fix: correct config path",
		"fix(config): correct scoped config path",
		"bugfix: handle nil pointer",
		"bugfix(release): handle scoped nil pointer",
		"Feat(API): uppercase type is still recognized",
		"chore: bump dependencies",
		"docs: update README",
		"random commit message with no prefix",
	}

	features, fixes, other := categorizeCommits(commits)

	wantFeatures := []string{
		"feat: add sync command",
		"feat(sync): add scoped sync support",
		"feature: add legacy alias support",
		"feature(cli): add legacy scoped alias support",
		"Feat(API): uppercase type is still recognized",
	}
	wantFixes := []string{
		"fix: correct config path",
		"fix(config): correct scoped config path",
		"bugfix: handle nil pointer",
		"bugfix(release): handle scoped nil pointer",
	}
	wantOther := []string{
		"chore: bump dependencies",
		"docs: update README",
		"random commit message with no prefix",
	}

	assertStringSliceEqual(t, "features", features, wantFeatures)
	assertStringSliceEqual(t, "fixes", fixes, wantFixes)
	assertStringSliceEqual(t, "other", other, wantOther)
}

// TestCategorizeCommits_ScopeDoesNotLeakIntoOtherTypes ensures a scope on
// one type doesn't accidentally satisfy another type's match (e.g. a scope
// containing the word "fix" inside a feat commit must still be a feature).
func TestCategorizeCommits_ScopeDoesNotLeakIntoOtherTypes(t *testing.T) {
	commits := []string{
		"feat(fix-parser): add new parser",
		"fix(feature-flag): correct flag default",
	}

	features, fixes, other := categorizeCommits(commits)

	assertStringSliceEqual(t, "features", features, []string{"feat(fix-parser): add new parser"})
	assertStringSliceEqual(t, "fixes", fixes, []string{"fix(feature-flag): correct flag default"})
	assertStringSliceEqual(t, "other", other, nil)
}

func assertStringSliceEqual(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d entries %v, want %d entries %v", label, len(got), got, len(want), want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s[%d]: got %q, want %q", label, i, got[i], want[i])
		}
	}
}
