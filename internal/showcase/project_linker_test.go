package showcase

import (
	"testing"

	"github.com/snonux/gitsyncer/internal/config"
)

func TestProjectLinkerBuildLink(t *testing.T) {
	t.Parallel()

	linker := NewProjectLinker(&config.Config{Organizations: []config.Organization{{
		Host: "git@github.com",
		Name: "snonux",
	}}})
	if got, want := linker.BuildLink("gitsyncer"), "https://github.com/snonux/gitsyncer"; got != want {
		t.Fatalf("BuildLink() = %q, want %q", got, want)
	}
}

func TestProjectLinkerBuildLinkWithoutGitHub(t *testing.T) {
	t.Parallel()

	if got := NewProjectLinker(nil).BuildLink("gitsyncer"); got != "" {
		t.Fatalf("BuildLink() = %q, want empty", got)
	}
}
