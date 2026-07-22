package cli

import (
	"errors"
	"testing"

	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/release"
)

type fakeReleaseNotesGenerator struct {
	aiNotes       string
	aiErr         error
	standardNotes string
	aiCalls       int
	standardCalls int
}

func (f *fakeReleaseNotesGenerator) GenerateAIReleaseNotes(_ string, _ string, _ string, _ []string, _ []string) (string, error) {
	f.aiCalls++
	if f.aiErr != nil {
		return "", f.aiErr
	}
	return f.aiNotes, nil
}

func (f *fakeReleaseNotesGenerator) GenerateReleaseNotes(_ string, _ string, _ []string) string {
	f.standardCalls++
	return f.standardNotes
}

func TestResolveReleaseNotes_CreateWithoutAIUsesStandardNotes(t *testing.T) {
	gen := &fakeReleaseNotesGenerator{standardNotes: "standard notes"}
	flags := &Flags{AIReleaseNotes: false}
	cache := map[string]string{}
	failed := []string{}
	saveCalls := 0

	notes, ok := resolveReleaseNotes(
		gen,
		flags,
		"/tmp/repo",
		"demo",
		"v1.0.0",
		[]string{"v1.0.0"},
		nil,
		"/tmp/cache.json",
		cache,
		&failed,
		"owner/demo:v1.0.0",
		releaseNotesModeCreate,
		func(_ string, _ map[string]string) error {
			saveCalls++
			return nil
		},
	)

	if !ok {
		t.Fatalf("expected ok=true")
	}
	if notes != "standard notes" {
		t.Fatalf("expected standard notes, got %q", notes)
	}
	if gen.standardCalls != 1 {
		t.Fatalf("expected standard generator to be called once, got %d", gen.standardCalls)
	}
	if gen.aiCalls != 0 {
		t.Fatalf("expected AI generator to not be called, got %d", gen.aiCalls)
	}
	if saveCalls != 0 {
		t.Fatalf("expected cache save to not be called, got %d", saveCalls)
	}
}

func TestResolveReleaseNotes_CreateWithForceStillUsesCachedAINotes(t *testing.T) {
	gen := &fakeReleaseNotesGenerator{aiNotes: "new ai notes"}
	flags := &Flags{AIReleaseNotes: true, Force: true}
	cache := map[string]string{"demo:v1.0.0": "cached ai notes"}
	failed := []string{}

	notes, ok := resolveReleaseNotes(
		gen,
		flags,
		"/tmp/repo",
		"demo",
		"v1.0.0",
		[]string{"v1.0.0"},
		nil,
		"/tmp/cache.json",
		cache,
		&failed,
		"owner/demo:v1.0.0",
		releaseNotesModeCreate,
		func(_ string, _ map[string]string) error { return nil },
	)

	if !ok {
		t.Fatalf("expected ok=true")
	}
	if notes != "cached ai notes" {
		t.Fatalf("expected cached notes, got %q", notes)
	}
	if gen.aiCalls != 0 {
		t.Fatalf("expected AI generator to not be called when cached, got %d", gen.aiCalls)
	}
}

func TestResolveReleaseNotes_CreateAIFailureFallsBackAndClearsCache(t *testing.T) {
	gen := &fakeReleaseNotesGenerator{
		aiErr:         errors.New("ai unavailable"),
		standardNotes: "fallback notes",
	}
	flags := &Flags{AIReleaseNotes: true, Force: true}
	cache := map[string]string{}
	failed := []string{}
	saveCalls := 0

	notes, ok := resolveReleaseNotes(
		gen,
		flags,
		"/tmp/repo",
		"demo",
		"v1.0.0",
		[]string{"v1.0.0"},
		nil,
		"/tmp/cache.json",
		cache,
		&failed,
		"owner/demo:v1.0.0",
		releaseNotesModeCreate,
		func(_ string, _ map[string]string) error {
			saveCalls++
			return nil
		},
	)

	if !ok {
		t.Fatalf("expected ok=true on create fallback")
	}
	if notes != "fallback notes" {
		t.Fatalf("expected fallback notes, got %q", notes)
	}
	if _, exists := cache["demo:v1.0.0"]; exists {
		t.Fatalf("expected cache entry to be removed after AI failure")
	}
	if len(failed) != 1 || failed[0] != "owner/demo:v1.0.0" {
		t.Fatalf("unexpected failed list: %#v", failed)
	}
	if saveCalls != 1 {
		t.Fatalf("expected cache save once after clearing entry, got %d", saveCalls)
	}
}

func TestResolveReleaseNotes_UpdateAIFailureSkipsUpdate(t *testing.T) {
	gen := &fakeReleaseNotesGenerator{
		aiErr:         errors.New("ai unavailable"),
		standardNotes: "unused fallback",
	}
	flags := &Flags{AIReleaseNotes: true, Force: true}
	cache := map[string]string{}
	failed := []string{}
	saveCalls := 0

	notes, ok := resolveReleaseNotes(
		gen,
		flags,
		"/tmp/repo",
		"demo",
		"v1.0.0",
		[]string{"v1.0.0"},
		nil,
		"/tmp/cache.json",
		cache,
		&failed,
		"owner/demo:v1.0.0",
		releaseNotesModeUpdate,
		func(_ string, _ map[string]string) error {
			saveCalls++
			return nil
		},
	)

	if ok {
		t.Fatalf("expected ok=false for update on AI failure")
	}
	if notes != "" {
		t.Fatalf("expected empty notes on update failure, got %q", notes)
	}
	if gen.standardCalls != 0 {
		t.Fatalf("did not expect standard fallback generation for update mode")
	}
	if _, exists := cache["demo:v1.0.0"]; exists {
		t.Fatalf("expected cache entry to be removed after AI failure")
	}
	if len(failed) != 1 || failed[0] != "owner/demo:v1.0.0" {
		t.Fatalf("unexpected failed list: %#v", failed)
	}
	if saveCalls != 1 {
		t.Fatalf("expected cache save once after clearing entry, got %d", saveCalls)
	}
}

func TestResolveReleaseNotes_CreateAISuccessContinuesOnCacheSaveError(t *testing.T) {
	gen := &fakeReleaseNotesGenerator{aiNotes: "generated ai notes"}
	flags := &Flags{AIReleaseNotes: true}
	cache := map[string]string{}
	failed := []string{}

	notes, ok := resolveReleaseNotes(
		gen,
		flags,
		"/tmp/repo",
		"demo",
		"v1.0.0",
		[]string{"v1.0.0"},
		nil,
		"/tmp/cache.json",
		cache,
		&failed,
		"owner/demo:v1.0.0",
		releaseNotesModeCreate,
		func(_ string, _ map[string]string) error {
			return errors.New("disk full")
		},
	)

	if !ok {
		t.Fatalf("expected ok=true on AI success")
	}
	if notes != "generated ai notes" {
		t.Fatalf("unexpected notes: %q", notes)
	}
	if cache["demo:v1.0.0"] != "generated ai notes" {
		t.Fatalf("expected successful AI notes to remain in cache")
	}
	if len(failed) != 0 {
		t.Fatalf("unexpected failed list: %#v", failed)
	}
}

func TestGetMissingReleasesForTarget_FiltersConfiguredSkips(t *testing.T) {
	cfg := &config.Config{
		SkipReleases: map[string][]string{
			"demo": {"v1.0.0"},
		},
	}
	releaseManager := release.NewManager("")
	target := releaseTarget{
		name:  "GitHub",
		owner: "owner",
		getReleases: func(_ string, _ string) ([]string, error) {
			return []string{"v0.9.0"}, nil
		},
	}

	missing := getMissingReleasesForTarget(cfg, releaseManager, target, "demo", []string{"v0.9.0", "v1.0.0", "v1.1.0"})

	if len(missing) != 1 || missing[0] != "v1.1.0" {
		t.Fatalf("expected only non-skipped missing release v1.1.0, got %#v", missing)
	}
}

func TestGetMissingReleasesForTarget_GetReleasesErrorReturnsNil(t *testing.T) {
	cfg := &config.Config{}
	releaseManager := release.NewManager("")
	target := releaseTarget{
		name:  "GitHub",
		owner: "owner",
		getReleases: func(_ string, _ string) ([]string, error) {
			return nil, errors.New("upstream unavailable")
		},
	}

	missing := getMissingReleasesForTarget(cfg, releaseManager, target, "demo", []string{"v1.0.0"})

	if missing != nil {
		t.Fatalf("expected nil missing releases on getReleases error, got %#v", missing)
	}
}

func TestProcessCreateReleasesForTarget_CreateErrorDoesNotStopOtherTags(t *testing.T) {
	cfg := &config.Config{}
	flags := &Flags{AutoCreateReleases: true}
	releaseManager := release.NewManager("")

	created := make([]string, 0, 2)
	target := releaseTarget{
		name:  "GitHub",
		owner: "owner",
		createRelease: func(_ string, _ string, tag string, _ string) error {
			created = append(created, tag)
			if tag == "v1.0.0" {
				return errors.New("create failed")
			}
			return nil
		},
	}

	processCreateReleasesForTarget(
		cfg,
		flags,
		releaseManager,
		target,
		"demo",
		"/definitely/not/a/repo",
		[]string{"v1.0.0", "v1.1.0"},
		[]string{"v1.0.0", "v1.1.0"},
		"/tmp/cache.json",
		map[string]string{},
		&[]string{},
	)

	if len(created) != 2 || created[0] != "v1.0.0" || created[1] != "v1.1.0" {
		t.Fatalf("expected both releases to be attempted despite first failure, got %#v", created)
	}
}

func TestProcessCreateReleasesForTarget_HonorsConfiguredSkip(t *testing.T) {
	cfg := &config.Config{
		SkipReleases: map[string][]string{
			"demo": {"v1.0.0"},
		},
	}
	flags := &Flags{AutoCreateReleases: true}
	releaseManager := release.NewManager("")
	created := make([]string, 0, 1)
	target := releaseTarget{
		name:  "Codeberg",
		owner: "owner",
		createRelease: func(_ string, _ string, tag string, _ string) error {
			created = append(created, tag)
			return nil
		},
	}

	processCreateReleasesForTarget(
		cfg,
		flags,
		releaseManager,
		target,
		"demo",
		"/definitely/not/a/repo",
		[]string{"v1.0.0", "v1.1.0"},
		[]string{"v1.0.0", "v1.1.0"},
		"/tmp/cache.json",
		map[string]string{},
		&[]string{},
	)

	if len(created) != 1 || created[0] != "v1.1.0" {
		t.Fatalf("expected only non-skipped release creation attempt, got %#v", created)
	}
}

func TestProcessUpdateReleasesForTarget_UsesCachedAIAndSkipsNonVersionTags(t *testing.T) {
	flags := &Flags{
		AIReleaseNotes:     true,
		AutoCreateReleases: true,
	}
	releaseManager := release.NewManager("")
	updated := make([]string, 0, 1)
	target := releaseTarget{
		name:  "GitHub",
		owner: "owner",
		getReleases: func(_ string, _ string) ([]string, error) {
			return []string{"latest", "1-beta", "v1.0.0"}, nil
		},
		updateRelease: func(_ string, _ string, tag string, notes string) error {
			if notes != "cached ai notes" {
				t.Fatalf("expected cached AI notes, got %q", notes)
			}
			updated = append(updated, tag)
			return nil
		},
	}

	processUpdateReleasesForTarget(
		flags,
		releaseManager,
		target,
		"demo",
		"/definitely/not/a/repo",
		[]string{"v1.0.0"},
		"/tmp/cache.json",
		map[string]string{"demo:v1.0.0": "cached ai notes"},
		&[]string{},
	)

	if len(updated) != 1 || updated[0] != "v1.0.0" {
		t.Fatalf("expected exactly one version tag update, got %#v", updated)
	}
}

func TestProcessUpdateReleasesForTarget_GetReleasesErrorSkipsUpdates(t *testing.T) {
	flags := &Flags{
		AIReleaseNotes:     true,
		AutoCreateReleases: true,
	}
	releaseManager := release.NewManager("")
	updateCalls := 0
	target := releaseTarget{
		name:  "Codeberg",
		owner: "owner",
		getReleases: func(_ string, _ string) ([]string, error) {
			return nil, errors.New("api error")
		},
		updateRelease: func(_ string, _ string, _ string, _ string) error {
			updateCalls++
			return nil
		},
	}

	processUpdateReleasesForTarget(
		flags,
		releaseManager,
		target,
		"demo",
		"/definitely/not/a/repo",
		[]string{"v1.0.0"},
		"/tmp/cache.json",
		map[string]string{"demo:v1.0.0": "cached ai notes"},
		&[]string{},
	)

	if updateCalls != 0 {
		t.Fatalf("expected no update attempts when getReleases fails, got %d", updateCalls)
	}
}
