package cli

import (
	"errors"
	"testing"
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

func TestResolveReleaseNotes_CreateUsesCachedAINotes(t *testing.T) {
	gen := &fakeReleaseNotesGenerator{aiNotes: "new ai notes"}
	flags := &Flags{AIReleaseNotes: true}
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
	cache := map[string]string{"demo:v1.0.0": "stale"}
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
	cache := map[string]string{"demo:v1.0.0": "stale"}
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
