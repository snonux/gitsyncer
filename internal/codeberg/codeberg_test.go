package codeberg

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

func TestGiteaClient_EnsurePublicRepoCreatesUninitializedRepository(t *testing.T) {
	t.Parallel()

	var createPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token secret" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repos/snonux/demo":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/user/repos":
			if err := json.NewDecoder(r.Body).Decode(&createPayload); err != nil {
				t.Errorf("decode payload: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"name":"demo","full_name":"snonux/demo","private":false}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewGiteaClient(server.URL+"/api/v1/", "secret", "snonux", "Forgejo")
	if err := client.EnsurePublicRepo("demo", "Demo repository"); err != nil {
		t.Fatalf("EnsurePublicRepo() error = %v", err)
	}
	if createPayload["name"] != "demo" || createPayload["private"] != false || createPayload["auto_init"] != false {
		t.Fatalf("create payload = %#v", createPayload)
	}
}

func TestGiteaClient_EnsurePublicRepoRejectsUnsafeExistingRepositories(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		repo Repository
		want string
	}{
		{name: "wrong owner", repo: Repository{Name: "demo", FullName: "other/demo"}, want: "collision"},
		{name: "private", repo: Repository{Name: "demo", FullName: "snonux/demo", Private: true}, want: "unexpectedly private"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(tt.repo)
			}))
			defer server.Close()
			err := NewGiteaClient(server.URL, "secret", "snonux", "Forgejo").EnsurePublicRepo("demo", "")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("EnsurePublicRepo() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestGiteaClient_UpdatesDescriptionAtConfiguredAPIBase(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/custom/api/repos/snonux/demo" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewGiteaClient(server.URL+"/custom/api", "secret", "snonux", "Forgejo")
	if err := client.UpdateRepoDescription("demo", "new description"); err != nil {
		t.Fatalf("UpdateRepoDescription() error = %v", err)
	}
}

func TestGiteaClient_EnsurePublicRepoRequiresToken(t *testing.T) {
	t.Parallel()

	err := NewGiteaClient("https://forgejo.example/api/v1", "", "snonux", "Forgejo").EnsurePublicRepo("demo", "")
	if err == nil || !strings.Contains(err.Error(), "token required") {
		t.Fatalf("EnsurePublicRepo() error = %v, want clear missing-token error", err)
	}
}

func TestGiteaClient_EnsurePublicRepoReportsAPIFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	err := NewGiteaClient(server.URL, "secret", "snonux", "Forgejo").EnsurePublicRepo("demo", "")
	if err == nil || !strings.Contains(err.Error(), "status 503") {
		t.Fatalf("EnsurePublicRepo() error = %v, want API status context", err)
	}
}
