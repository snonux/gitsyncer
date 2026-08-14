package sync

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"codeberg.org/snonux/gitsyncer/internal/config"
)

// captureStdout runs fn while redirecting os.Stdout, returning the captured
// output. Used below to assert on the order remotes are processed in,
// without depending on any particular git behavior.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return buf.String()
}

// TestSortedRemoteNames_IsDeterministicAcrossManyRuns guards the shared
// helper directly: Go deliberately randomizes map iteration order (see
// "100 Go Mistakes" #33), so a naive `for k := range m` would produce a
// different order on every call even for the exact same map. Calling the
// helper many times over the same unsorted map and requiring the exact same
// sorted slice every time would have caught a regression back to plain map
// ranging, which is very likely to vary across the 50 runs below.
func TestSortedRemoteNames_IsDeterministicAcrossManyRuns(t *testing.T) {
	remotes := map[string]bool{
		"zulu": true, "alpha": true, "mike": true, "bravo": true,
		"echo": true, "yankee": true, "delta": true, "sierra": true,
	}
	want := []string{"alpha", "bravo", "delta", "echo", "mike", "sierra", "yankee", "zulu"}

	for i := 0; i < 50; i++ {
		got := sortedRemoteNames(remotes)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d: sortedRemoteNames() = %v, want %v", i, got, want)
		}
	}
}

// TestMergeFromRemotes_AttemptsRemotesInSortedOrder checks that
// mergeFromRemotes walks remotesWithBranch in sorted order rather than
// Go's randomized map order. It merges against a repo path that does not
// exist, so the very first merge attempt fails immediately; the error
// message (which mergeBranch annotates with the remote name it was merging)
// must always name the alphabetically-first remote ("alpha"), never one of
// the others, across many repeated runs.
func TestMergeFromRemotes_AttemptsRemotesInSortedOrder(t *testing.T) {
	remotesWithBranch := map[string]bool{"zulu": true, "alpha": true, "mike": true}
	missingRepoPath := filepath.Join(t.TempDir(), "does-not-exist")

	for i := 0; i < 20; i++ {
		err := mergeFromRemotes(missingRepoPath, "main", remotesWithBranch)
		if err == nil {
			t.Fatalf("run %d: expected an error merging against a nonexistent repo path", i)
		}
		if !strings.Contains(err.Error(), "alpha/main") {
			t.Fatalf("run %d: expected the alphabetically-first remote (alpha) to be attempted first, got: %v", i, err)
		}
	}
}

// TestPushToAllRemotes_ProcessesRemotesInSortedOrder exercises the real
// (dry-run) push path and asserts the "Creating branch on X" progress
// messages appear in sorted order on every run. Without sortedRemoteNames,
// this would flake across the 20 runs below because Go randomizes map
// iteration order per range, not just per map.
func TestPushToAllRemotes_ProcessesRemotesInSortedOrder(t *testing.T) {
	names := []string{"zulu", "alpha", "mike", "bravo", "echo"}
	remotes := make(map[string]*config.Organization, len(names))
	for _, name := range names {
		remotes[name] = &config.Organization{Host: "host-" + name}
	}

	want := append([]string(nil), names...)
	sort.Strings(want)

	orderRe := regexp.MustCompile(`Creating branch on (\S+) `)

	for i := 0; i < 20; i++ {
		syncer := New(&config.Config{}, t.TempDir())
		syncer.SetDryRun(true)

		output := captureStdout(t, func() {
			if err := syncer.pushToAllRemotes("/path/that/does/not/exist", "main", remotes, nil); err != nil {
				t.Fatalf("pushToAllRemotes() error = %v", err)
			}
		})

		var got []string
		for _, m := range orderRe.FindAllStringSubmatch(output, -1) {
			got = append(got, m[1])
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d: push order = %v, want %v", i, got, want)
		}
	}
}

// TestFetchAll_ProcessesRemotesInSortedOrder exercises fetchAll against a
// real git repository with several remotes configured as active backup
// locations, which makes fetchAll print "Skipping fetch from backup
// location X" and move on without ever invoking git fetch. That lets the
// test observe fetch order deterministically and cheaply across many runs,
// which would have caught fetchAll ranging over the remotes map directly.
func TestFetchAll_ProcessesRemotesInSortedOrder(t *testing.T) {
	workDir := t.TempDir()
	repoPath := filepath.Join(workDir, "demo")
	if output, err := exec.Command("git", "init", repoPath).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}

	names := []string{"zulu", "alpha", "mike", "bravo", "echo"}
	orgs := make([]config.Organization, 0, len(names))
	for _, name := range names {
		addOutput, err := exec.Command("git", "-C", repoPath, "remote", "add", name, "file:///nonexistent/"+name+".git").CombinedOutput()
		if err != nil {
			t.Fatalf("git remote add %s: %v\n%s", name, err, addOutput)
		}
		// Host is "git@"+name so getRemoteName(org) derives back to name
		// exactly (no dots/colons/slashes to rewrite).
		orgs = append(orgs, config.Organization{Host: "git@" + name, BackupLocation: true})
	}

	want := append([]string(nil), names...)
	sort.Strings(want)

	orderRe := regexp.MustCompile(`Skipping fetch from backup location (\S+)`)

	for i := 0; i < 20; i++ {
		syncer := &Syncer{
			config:   &config.Config{Organizations: orgs},
			workDir:  workDir,
			repoName: "demo",
		}
		syncer.SetBackupEnabled(true)

		output := captureStdout(t, func() {
			if err := syncer.fetchAll(); err != nil {
				t.Fatalf("fetchAll() error = %v", err)
			}
		})

		var got []string
		for _, m := range orderRe.FindAllStringSubmatch(output, -1) {
			got = append(got, m[1])
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d: fetch order = %v, want %v", i, got, want)
		}
	}
}
