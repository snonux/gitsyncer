package codeberg

import "testing"

func TestNewClient_UsesTokenOrgOrderAndReturnsPointer(t *testing.T) {
	t.Setenv("CODEBERG_TOKEN", "")
	t.Setenv("HOME", t.TempDir())

	const (
		token = "config-token"
		org   = "example-org"
	)

	client := NewClient(token, org)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.token != token {
		t.Fatalf("expected token %q, got %q", token, client.token)
	}
	if client.org != org {
		t.Fatalf("expected org %q, got %q", org, client.org)
	}
}

func TestNewClient_LoadsTokenFromEnvWhenConfigTokenMissing(t *testing.T) {
	t.Setenv("CODEBERG_TOKEN", "env-token")
	t.Setenv("HOME", t.TempDir())

	client := NewClient("", "example-org")
	if !client.HasToken() {
		t.Fatal("expected token from environment")
	}
	if client.token != "env-token" {
		t.Fatalf("expected env token, got %q", client.token)
	}
}

func TestNewClient_HasNoTokenWhenNoSourcesAvailable(t *testing.T) {
	t.Setenv("CODEBERG_TOKEN", "")
	t.Setenv("HOME", t.TempDir())

	client := NewClient("", "example-org")
	if client.HasToken() {
		t.Fatal("expected no token when config, env, and file are empty")
	}
}
