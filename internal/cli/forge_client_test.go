package cli

import (
	"testing"

	"codeberg.org/snonux/gitsyncer/internal/config"
)

func TestNewRepoClientForOrg(t *testing.T) {
	t.Parallel()

	t.Run("github", func(t *testing.T) {
		client, ok := newRepoClientForOrg(config.Organization{
			Host:        "git@github.com",
			Name:        "acme",
			GitHubToken: "token",
		})
		if !ok {
			t.Fatal("expected supported github client")
		}
		if !client.HasToken() {
			t.Fatal("expected github client token to be loaded")
		}
	})

	t.Run("codeberg", func(t *testing.T) {
		client, ok := newRepoClientForOrg(config.Organization{
			Host:          "git@codeberg.org",
			Name:          "acme",
			CodebergToken: "token",
		})
		if !ok {
			t.Fatal("expected supported codeberg client")
		}
		if !client.HasToken() {
			t.Fatal("expected codeberg client token to be loaded")
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		client, ok := newRepoClientForOrg(config.Organization{
			Host: "ssh://example.org",
			Name: "acme",
		})
		if ok {
			t.Fatal("expected unsupported host")
		}
		if client != nil {
			t.Fatal("expected nil client for unsupported host")
		}
	})

	t.Run("unsupported near miss hosts", func(t *testing.T) {
		t.Parallel()

		nearMissHosts := []string{
			"ssh://github.com",
			"git@github.company.com",
			"git@github.com:acme",
			"https://codeberg.org",
			"git@codeberg.org.example",
		}

		for _, host := range nearMissHosts {
			host := host
			t.Run(host, func(t *testing.T) {
				t.Parallel()

				client, ok := newRepoClientForOrg(config.Organization{
					Host: host,
					Name: "acme",
				})
				if ok {
					t.Fatalf("expected unsupported host %q", host)
				}
				if client != nil {
					t.Fatalf("expected nil client for unsupported host %q", host)
				}
			})
		}
	})
}
