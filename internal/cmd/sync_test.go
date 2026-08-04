package cmd

import (
	"testing"

	"codeberg.org/snonux/gitsyncer/internal/cli"
)

func TestShouldRunPostSyncReleases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		exitCode         int
		flags            *cli.Flags
		releasesDisabled bool
		want             bool
	}{
		{name: "successful sync", flags: &cli.Flags{}, want: true},
		{name: "dry run", flags: &cli.Flags{DryRun: true}},
		{name: "sync failure", exitCode: 1, flags: &cli.Flags{}},
		{name: "releases disabled", flags: &cli.Flags{}, releasesDisabled: true},
		{name: "nil flags"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRunPostSyncReleases(tt.exitCode, tt.flags, tt.releasesDisabled); got != tt.want {
				t.Fatalf("shouldRunPostSyncReleases() = %t, want %t", got, tt.want)
			}
		})
	}
}
