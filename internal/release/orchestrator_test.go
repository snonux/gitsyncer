package release

import (
	"errors"
	"testing"

	"codeberg.org/snonux/gitsyncer/internal/config"
)

// alwaysConfirm and neverConfirm are ConfirmFunc test doubles standing in
// for internal/cli's interactive promptConfirmation.
func alwaysConfirm(string) bool { return true }

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

// fakeReleaseClient is a forge.ReleaseClient double used to drive the release
// pipeline without touching any real HTTP. It also implements
// forge.ReleasesEnabler so the ensure-releases path can be exercised.
type fakeReleaseClient struct {
	releases    []string
	releasesErr error

	created    []string
	createErrs map[string]error // per-tag errors; nil error or missing tag -> success

	updated    []releaseUpdate
	updateErrs map[string]error // per-tag errors; nil error or missing tag -> success

	ensureErr   error
	ensureCalls int
}

func (f *fakeReleaseClient) GetReleases(_, _ string) ([]string, error) {
	return f.releases, f.releasesErr
}

func (f *fakeReleaseClient) CreateRelease(_, _, tag, _ string) error {
	f.created = append(f.created, tag)
	if f.createErrs != nil {
		if err, ok := f.createErrs[tag]; ok {
			return err
		}
	}
	return nil
}

func (f *fakeReleaseClient) UpdateRelease(_, _, tag, notes string) error {
	f.updated = append(f.updated, releaseUpdate{tag: tag, notes: notes})
	if f.updateErrs != nil {
		if err, ok := f.updateErrs[tag]; ok {
			return err
		}
	}
	return nil
}

func (f *fakeReleaseClient) EnsureReleasesEnabled(_, _ string) error {
	f.ensureCalls++
	return f.ensureErr
}

// releaseUpdate records the tag and notes passed to UpdateRelease so tests can
// assert on the notes content.
type releaseUpdate struct {
	tag   string
	notes string
}

func TestResolveReleaseNotes_CreateWithoutAIUsesStandardNotes(t *testing.T) {
	gen := &fakeReleaseNotesGenerator{standardNotes: "standard notes"}
	opts := Options{AIReleaseNotes: false}
	cache := map[string]string{}
	failed := []string{}
	saveCalls := 0

	notes, ok := resolveReleaseNotes(
		gen,
		opts,
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
	opts := Options{AIReleaseNotes: true}
	cache := map[string]string{"demo:v1.0.0": "cached ai notes"}
	failed := []string{}

	notes, ok := resolveReleaseNotes(
		gen,
		opts,
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
	opts := Options{AIReleaseNotes: true}
	cache := map[string]string{}
	failed := []string{}
	saveCalls := 0

	notes, ok := resolveReleaseNotes(
		gen,
		opts,
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
	opts := Options{AIReleaseNotes: true}
	cache := map[string]string{}
	failed := []string{}
	saveCalls := 0

	notes, ok := resolveReleaseNotes(
		gen,
		opts,
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
	opts := Options{AIReleaseNotes: true}
	cache := map[string]string{}
	failed := []string{}

	notes, ok := resolveReleaseNotes(
		gen,
		opts,
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

func TestTargetApplicable_CodebergGatedByAllowlist(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{Repositories: []string{"cpuinfo"}, SyncCodeberg: true}
	gh := Target{Name: "GitHub"}
	cb := Target{Name: "Codeberg", SyncRepoRequired: true}

	// GitHub target runs for any repo.
	if !TargetApplicable(gh, cfg, "anything") {
		t.Fatal("GitHub target should be applicable to any repo")
	}

	// Codeberg target only runs for allowlisted repos.
	if !TargetApplicable(cb, cfg, "cpuinfo") {
		t.Fatal("Codeberg target should be applicable to allowlisted repo")
	}
	if TargetApplicable(cb, cfg, "hypr") {
		t.Fatal("Codeberg target should NOT be applicable to non-allowlisted repo")
	}

	// Discovery mode (empty allowlist): Codeberg runs for any repo.
	discovery := &config.Config{SyncCodeberg: true}
	if !TargetApplicable(cb, discovery, "anything") {
		t.Fatal("Codeberg target should be applicable to any repo in discovery mode")
	}
}

func TestGetMissingReleasesForTarget_FiltersConfiguredSkips(t *testing.T) {
	cfg := &config.Config{
		SkipReleases: map[string][]string{
			"demo": {"v1.0.0"},
		},
	}
	inspector := NewGitInspector()
	target := Target{
		Name:  "GitHub",
		Owner: "owner",
		Client: &fakeReleaseClient{
			releases: []string{"v0.9.0"},
		},
	}

	missing := getMissingReleasesForTarget(cfg, inspector, target, "demo", []string{"v0.9.0", "v1.0.0", "v1.1.0"})

	if len(missing) != 1 || missing[0] != "v1.1.0" {
		t.Fatalf("expected only non-skipped missing release v1.1.0, got %#v", missing)
	}
}

func TestGetMissingReleasesForTarget_GetReleasesErrorReturnsNil(t *testing.T) {
	cfg := &config.Config{}
	inspector := NewGitInspector()
	target := Target{
		Name:  "GitHub",
		Owner: "owner",
		Client: &fakeReleaseClient{
			releasesErr: errors.New("upstream unavailable"),
		},
	}

	missing := getMissingReleasesForTarget(cfg, inspector, target, "demo", []string{"v1.0.0"})

	if missing != nil {
		t.Fatalf("expected nil missing releases on getReleases error, got %#v", missing)
	}
}

func TestProcessCreateReleasesForTarget_CreateErrorDoesNotStopOtherTags(t *testing.T) {
	cfg := &config.Config{}
	opts := Options{AutoCreateReleases: true}
	inspector := NewGitInspector()
	notes := &fakeReleaseNotesGenerator{}

	client := &fakeReleaseClient{
		createErrs: map[string]error{"v1.0.0": errors.New("create failed")},
	}
	target := Target{
		Name:   "GitHub",
		Owner:  "owner",
		Client: client,
	}

	processCreateReleasesForTarget(
		cfg,
		opts,
		alwaysConfirm,
		inspector,
		notes,
		target,
		"demo",
		"/definitely/not/a/repo",
		[]string{"v1.0.0", "v1.1.0"},
		[]string{"v1.0.0", "v1.1.0"},
		"/tmp/cache.json",
		map[string]string{},
		&[]string{},
	)

	if len(client.created) != 2 || client.created[0] != "v1.0.0" || client.created[1] != "v1.1.0" {
		t.Fatalf("expected both releases to be attempted despite first failure, got %#v", client.created)
	}
}

func TestProcessCreateReleasesForTarget_HonorsConfiguredSkip(t *testing.T) {
	cfg := &config.Config{
		SkipReleases: map[string][]string{
			"demo": {"v1.0.0"},
		},
	}
	opts := Options{AutoCreateReleases: true}
	inspector := NewGitInspector()
	notes := &fakeReleaseNotesGenerator{}
	client := &fakeReleaseClient{}
	target := Target{
		Name:   "Codeberg",
		Owner:  "owner",
		Client: client,
	}

	processCreateReleasesForTarget(
		cfg,
		opts,
		alwaysConfirm,
		inspector,
		notes,
		target,
		"demo",
		"/definitely/not/a/repo",
		[]string{"v1.0.0", "v1.1.0"},
		[]string{"v1.0.0", "v1.1.0"},
		"/tmp/cache.json",
		map[string]string{},
		&[]string{},
	)

	if len(client.created) != 1 || client.created[0] != "v1.1.0" {
		t.Fatalf("expected only non-skipped release creation attempt, got %#v", client.created)
	}
}

func TestProcessUpdateReleasesForTarget_UsesCachedAIAndSkipsNonVersionTags(t *testing.T) {
	opts := Options{
		AIReleaseNotes:     true,
		AutoCreateReleases: true,
	}
	inspector := NewGitInspector()
	notes := &fakeReleaseNotesGenerator{}
	client := &fakeReleaseClient{
		releases: []string{"latest", "1-beta", "v1.0.0"},
	}
	target := Target{
		Name:   "GitHub",
		Owner:  "owner",
		Client: client,
	}

	processUpdateReleasesForTarget(
		opts,
		alwaysConfirm,
		inspector,
		notes,
		target,
		"demo",
		"/definitely/not/a/repo",
		[]string{"v1.0.0"},
		"/tmp/cache.json",
		map[string]string{"demo:v1.0.0": "cached ai notes"},
		&[]string{},
	)

	if len(client.updated) != 1 || client.updated[0].tag != "v1.0.0" {
		t.Fatalf("expected exactly one version tag update, got %#v", client.updated)
	}
	if client.updated[0].notes != "cached ai notes" {
		t.Fatalf("expected cached AI notes, got %q", client.updated[0].notes)
	}
}

func TestProcessUpdateReleasesForTarget_GetReleasesErrorSkipsUpdates(t *testing.T) {
	opts := Options{
		AIReleaseNotes:     true,
		AutoCreateReleases: true,
	}
	inspector := NewGitInspector()
	notes := &fakeReleaseNotesGenerator{}
	client := &fakeReleaseClient{
		releasesErr: errors.New("api error"),
	}
	target := Target{
		Name:   "Codeberg",
		Owner:  "owner",
		Client: client,
	}

	processUpdateReleasesForTarget(
		opts,
		alwaysConfirm,
		inspector,
		notes,
		target,
		"demo",
		"/definitely/not/a/repo",
		[]string{"v1.0.0"},
		"/tmp/cache.json",
		map[string]string{"demo:v1.0.0": "cached ai notes"},
		&[]string{},
	)

	if len(client.updated) != 0 {
		t.Fatalf("expected no update attempts when getReleases fails, got %#v", client.updated)
	}
}

func TestProcessReleaseTargets_DryRunDispatchesNoMutations(t *testing.T) {
	opts := Options{DryRun: true, AIReleaseNotes: true, AutoCreateReleases: true}
	inspector := NewGitInspector()
	notes := &fakeReleaseNotesGenerator{}
	client := &fakeReleaseClient{
		releases: []string{"v1.0.0"},
	}
	target := Target{
		Name:   "GitHub",
		Owner:  "owner",
		Client: client,
	}

	processCreateReleasesForTarget(&config.Config{}, opts, alwaysConfirm, inspector, notes, target, "demo", "/not/a/repo", []string{"v1.0.0"}, []string{"v1.0.0"}, "/tmp/cache", map[string]string{}, &[]string{})
	processUpdateReleasesForTarget(opts, alwaysConfirm, inspector, notes, target, "demo", "/not/a/repo", []string{"v1.0.0"}, "/tmp/cache", map[string]string{}, &[]string{})

	if len(client.created) != 0 || len(client.updated) != 0 || client.ensureCalls != 0 {
		t.Fatalf("dry run dispatched release mutations: create=%d update=%d ensure=%d", len(client.created), len(client.updated), client.ensureCalls)
	}
}
