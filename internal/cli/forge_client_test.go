package cli

import (
	"testing"

	"codeberg.org/snonux/gitsyncer/internal/config"
)

func TestForgeClientResolver_ClientFor(t *testing.T) {
	resolver := ForgeClientResolver{}

	t.Run("github", func(t *testing.T) {
		org := config.Organization{
			Host:        "git@github.com",
			Name:        "acme",
			GitHubToken: "token",
		}
		client, ok := resolver.ClientFor(&org)
		if !ok {
			t.Fatal("expected supported github client")
		}
		if !client.HasToken() {
			t.Fatal("expected github client token to be loaded")
		}
	})

	t.Run("codeberg", func(t *testing.T) {
		org := config.Organization{
			Host:          "git@codeberg.org",
			Name:          "acme",
			CodebergToken: "token",
		}
		client, ok := resolver.ClientFor(&org)
		if !ok {
			t.Fatal("expected supported codeberg client")
		}
		if !client.HasToken() {
			t.Fatal("expected codeberg client token to be loaded")
		}
	})

	t.Run("forgejo backup", func(t *testing.T) {
		t.Setenv("FORGEJO_TOKEN", "token")
		org := config.Organization{
			Host:           "ssh://git@code.f3s.buetow.org:2022",
			ForgejoAPIBase: "https://code.f3s.buetow.org/api/v1",
			ForgejoOwner:   "snonux",
			BackupLocation: true,
		}
		client, ok := resolver.ClientFor(&org)
		if !ok || !client.HasToken() {
			t.Fatal("expected supported Forgejo client with environment token")
		}
	})

	t.Run("github host variants", func(t *testing.T) {
		t.Parallel()
		resolver := ForgeClientResolver{}

		variantHosts := []string{
			"ssh://github.com",
			"git@github.company.com",
			"git@github.com:acme",
			"https://github.com",
		}

		for _, host := range variantHosts {
			host := host
			t.Run(host, func(t *testing.T) {
				t.Parallel()

				org := config.Organization{
					Host:        host,
					Name:        "acme",
					GitHubToken: "token",
				}
				client, ok := resolver.ClientFor(&org)
				if !ok {
					t.Fatalf("expected supported github host variant %q", host)
				}
				if !client.HasToken() {
					t.Fatalf("expected github client token for host variant %q", host)
				}
			})
		}
	})

	t.Run("codeberg host variants", func(t *testing.T) {
		t.Parallel()
		resolver := ForgeClientResolver{}

		variantHosts := []string{
			"https://codeberg.org",
			"ssh://codeberg.org",
			"git@codeberg.org:acme",
			"git@codeberg.org.example",
		}

		for _, host := range variantHosts {
			host := host
			t.Run(host, func(t *testing.T) {
				t.Parallel()

				org := config.Organization{
					Host:          host,
					Name:          "acme",
					CodebergToken: "token",
				}
				client, ok := resolver.ClientFor(&org)
				if !ok {
					t.Fatalf("expected supported codeberg host variant %q", host)
				}
				if !client.HasToken() {
					t.Fatalf("expected codeberg client token for host variant %q", host)
				}
			})
		}
	})

	t.Run("unsupported hosts", func(t *testing.T) {
		t.Parallel()
		resolver := ForgeClientResolver{}

		unsupportedHosts := []string{
			"ssh://example.org",
			"git@gitlab.com",
			"file:///srv/git",
		}

		for _, host := range unsupportedHosts {
			host := host
			t.Run(host, func(t *testing.T) {
				t.Parallel()

				org := config.Organization{
					Host: host,
					Name: "acme",
				}
				client, ok := resolver.ClientFor(&org)
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
