package showcase

import (
	"reflect"
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
