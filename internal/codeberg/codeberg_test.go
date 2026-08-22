package codeberg

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/snonux/gitsyncer/internal/forge"
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

func TestNewClient_TrimsTokenFromConfig(t *testing.T) {
	t.Setenv("CODEBERG_TOKEN", "")
	t.Setenv("HOME", t.TempDir())

	client := NewClient("config-token\n", "example-org")
	if client.token != "config-token" {
		t.Fatalf("expected trimmed config token, got %q", client.token)
	}
}

func TestNewClient_TrimsTokenFromEnv(t *testing.T) {
	t.Setenv("CODEBERG_TOKEN", "env-token\n")
	t.Setenv("HOME", t.TempDir())

	client := NewClient("", "example-org")
	if client.token != "env-token" {
		t.Fatalf("expected trimmed env token, got %q", client.token)
	}
}

func TestNewClient_TrimsTokenFromFile(t *testing.T) {
	t.Setenv("CODEBERG_TOKEN", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	tokenFile := filepath.Join(home, ".gitsyncer_codeberg_token")
	if err := os.WriteFile(tokenFile, []byte("file-token\n"), 0600); err != nil {
		t.Fatalf("failed to write token file: %v", err)
	}

	client := NewClient("", "example-org")
	if client.token != "file-token" {
		t.Fatalf("expected trimmed file token, got %q", client.token)
	}
}

// TestNewClient_AcceptsOwnerOnlyTokenFileModes exercises Client.loadToken's
// file fallback, which now reads through forge.ReadProtectedTokenFile
// instead of os.ReadFile. Owner-only modes must still work.
func TestNewClient_AcceptsOwnerOnlyTokenFileModes(t *testing.T) {
	for _, mode := range []os.FileMode{0600, 0400} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Setenv("CODEBERG_TOKEN", "")
			home := t.TempDir()
			t.Setenv("HOME", home)

			tokenFile := filepath.Join(home, ".gitsyncer_codeberg_token")
			if err := os.WriteFile(tokenFile, []byte("file-token"), mode); err != nil {
				t.Fatalf("write token file: %v", err)
			}
			if err := os.Chmod(tokenFile, mode); err != nil {
				t.Fatalf("set token file mode: %v", err)
			}

			client := NewClient("", "example-org")
			if client.token != "file-token" {
				t.Fatalf("loaded token = %q, want file token", client.token)
			}
		})
	}
}

// TestNewClient_RejectsSymlinkTokenFile mirrors the same protection already
// covered for loadForgejoToken: a symlink at the well-known token path must
// not be followed.
func TestNewClient_RejectsSymlinkTokenFile(t *testing.T) {
	t.Setenv("CODEBERG_TOKEN", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	target := filepath.Join(home, "token-target")
	if err := os.WriteFile(target, []byte("file-token"), 0600); err != nil {
		t.Fatalf("write symlink target: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(home, ".gitsyncer_codeberg_token")); err != nil {
		t.Fatalf("create token symlink: %v", err)
	}

	client := NewClient("", "example-org")
	if client.HasToken() {
		t.Fatalf("loaded token = %q, want no token for symlinked token file", client.token)
	}
}

// TestNewClient_RejectsOverlyPermissiveTokenFile mirrors the same
// protection already covered for loadForgejoToken: a token file readable or
// writable by group/other must be rejected.
func TestNewClient_RejectsOverlyPermissiveTokenFile(t *testing.T) {
	t.Setenv("CODEBERG_TOKEN", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	tokenFile := filepath.Join(home, ".gitsyncer_codeberg_token")
	if err := os.WriteFile(tokenFile, []byte("file-token"), 0644); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	if err := os.Chmod(tokenFile, 0644); err != nil {
		t.Fatalf("set token file mode: %v", err)
	}

	client := NewClient("", "example-org")
	if client.HasToken() {
		t.Fatalf("loaded token = %q, want no token for group/other readable token file", client.token)
	}
}

// TestNewClient_RejectsNonRegularTokenFile mirrors the same protection
// already covered for loadForgejoToken: a non-regular file (e.g. a unix
// socket) at the token path must be rejected.
func TestNewClient_RejectsNonRegularTokenFile(t *testing.T) {
	t.Setenv("CODEBERG_TOKEN", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	listener, err := net.Listen("unix", filepath.Join(home, ".gitsyncer_codeberg_token"))
	if err != nil {
		t.Fatalf("create token socket: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	client := NewClient("", "example-org")
	if client.HasToken() {
		t.Fatalf("loaded token = %q, want no token for non-regular token file", client.token)
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

func TestNewForgejoClient_LoadsProtectedToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FORGEJO_TOKEN", "")
	if err := os.Unsetenv("FORGEJO_TOKEN"); err != nil {
		t.Fatalf("unset FORGEJO_TOKEN: %v", err)
	}
	tokenFile := filepath.Join(home, ".gitsyncer_forgejo_token")
	if err := os.WriteFile(tokenFile, []byte("  file-token\n"), 0600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	t.Run("token file is trimmed", func(t *testing.T) {
		client := NewForgejoClient("https://forgejo.example/api/v1", "owner", forge.OwnerTypeUser)
		if client.token != "file-token" {
			t.Fatalf("loaded token = %q, want trimmed file token", client.token)
		}
	})

	t.Run("environment takes precedence and is trimmed", func(t *testing.T) {
		t.Setenv("FORGEJO_TOKEN", "  env-token\n")
		client := NewForgejoClient("https://forgejo.example/api/v1", "owner", forge.OwnerTypeUser)
		if client.token != "env-token" {
			t.Fatalf("loaded token = %q, want trimmed environment token", client.token)
		}
	})

	t.Run("whitespace-only environment falls back to token file", func(t *testing.T) {
		t.Setenv("FORGEJO_TOKEN", " \t\n")
		client := NewForgejoClient("https://forgejo.example/api/v1", "owner", forge.OwnerTypeUser)
		if client.token != "file-token" {
			t.Fatalf("loaded token = %q, want token from file", client.token)
		}
	})
}

func TestNewForgejoClient_AcceptsOwnerOnlyTokenFileModes(t *testing.T) {
	for _, mode := range []os.FileMode{0600, 0400} {
		t.Run(mode.String(), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("FORGEJO_TOKEN", "")
			tokenFile := filepath.Join(home, ".gitsyncer_forgejo_token")
			if err := os.WriteFile(tokenFile, []byte("file-token"), mode); err != nil {
				t.Fatalf("write token file: %v", err)
			}
			if err := os.Chmod(tokenFile, mode); err != nil {
				t.Fatalf("set token file mode: %v", err)
			}

			client := NewForgejoClient("https://forgejo.example/api/v1", "owner", forge.OwnerTypeUser)
			if client.token != "file-token" {
				t.Fatalf("loaded token = %q, want file token", client.token)
			}
		})
	}
}

func TestNewForgejoClient_WhitespaceOnlyFileHasNoToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FORGEJO_TOKEN", " \t\n")
	tokenFile := filepath.Join(home, ".gitsyncer_forgejo_token")
	if err := os.WriteFile(tokenFile, []byte(" \t\n"), 0600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	client := NewForgejoClient("https://forgejo.example/api/v1", "owner", forge.OwnerTypeUser)
	if client.HasToken() {
		t.Fatalf("loaded token = %q, want no token", client.token)
	}
}

func TestNewForgejoClient_MissingOrUnreadableFileHasNoToken(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{name: "missing"},
		{name: "group or other permissions", setup: func(t *testing.T, home string) {
			tokenFile := filepath.Join(home, ".gitsyncer_forgejo_token")
			if err := os.WriteFile(tokenFile, []byte("file-token"), 0644); err != nil {
				t.Fatalf("write token file: %v", err)
			}
			if err := os.Chmod(tokenFile, 0644); err != nil {
				t.Fatalf("set token file mode: %v", err)
			}
		}},
		{name: "unreadable", setup: func(t *testing.T, home string) {
			if err := os.Mkdir(filepath.Join(home, ".gitsyncer_forgejo_token"), 0700); err != nil {
				t.Fatalf("create unreadable token path: %v", err)
			}
		}},
		{name: "symlink", setup: func(t *testing.T, home string) {
			target := filepath.Join(home, "token-target")
			if err := os.WriteFile(target, []byte("file-token"), 0600); err != nil {
				t.Fatalf("write symlink target: %v", err)
			}
			if err := os.Symlink(target, filepath.Join(home, ".gitsyncer_forgejo_token")); err != nil {
				t.Fatalf("create token symlink: %v", err)
			}
		}},
		{name: "unix socket", setup: func(t *testing.T, home string) {
			listener, err := net.Listen("unix", filepath.Join(home, ".gitsyncer_forgejo_token"))
			if err != nil {
				t.Fatalf("create token socket: %v", err)
			}
			t.Cleanup(func() { _ = listener.Close() })
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("FORGEJO_TOKEN", "")
			if err := os.Unsetenv("FORGEJO_TOKEN"); err != nil {
				t.Fatalf("unset FORGEJO_TOKEN: %v", err)
			}
			if tt.setup != nil {
				tt.setup(t, home)
			}
			client := NewForgejoClient("https://forgejo.example/api/v1", "owner", forge.OwnerTypeUser)
			if client.HasToken() {
				t.Fatal("HasToken() = true, want false")
			}
			err := client.EnsurePublicRepo("demo", "")
			if err == nil || strings.Contains(err.Error(), home) || strings.Contains(err.Error(), "file-token") {
				t.Fatalf("missing-token error = %q, want generic error without credential details", err)
			}
		})
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

	client := NewGiteaClient(server.URL+"/api/v1/", "secret", "snonux", "Forgejo", "")
	if err := client.EnsurePublicRepo("demo", "Demo repository"); err != nil {
		t.Fatalf("EnsurePublicRepo() error = %v", err)
	}
	if createPayload["name"] != "demo" || createPayload["private"] != false || createPayload["auto_init"] != false {
		t.Fatalf("create payload = %#v", createPayload)
	}
}

func TestGiteaClient_EnsurePublicRepoUsesOrganizationEndpoint(t *testing.T) {
	t.Parallel()

	var posted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token secret" {
			t.Errorf("Authorization = %q, want token secret", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repos/snonux/demo":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/orgs/snonux/repos":
			posted = true
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode payload: %v", err)
			}
			if payload["name"] != "demo" || payload["description"] != "Mirror of demo" ||
				payload["private"] != false || payload["auto_init"] != false {
				t.Errorf("create payload = %#v, want named, described, public, and uninitialized", payload)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"name":"demo","full_name":"snonux/demo","private":false}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewGiteaClient(server.URL+"/api/v1", "secret", "snonux", "Forgejo", forge.OwnerTypeOrganization)
	if err := client.EnsurePublicRepo("demo", "Mirror of demo"); err != nil {
		t.Fatalf("EnsurePublicRepo() error = %v", err)
	}
	if !posted {
		t.Fatal("organization repository endpoint was not called")
	}
}

func TestGiteaClient_EnsurePublicRepoReportsOrganizationCreateFailure(t *testing.T) {
	t.Parallel()

	var postRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repos/snonux/demo":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/orgs/snonux/repos":
			postRequests++
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"not allowed to create repository in organization"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewGiteaClient(server.URL+"/api/v1", "secret", "snonux", "Forgejo", forge.OwnerTypeOrganization)
	err := client.EnsurePublicRepo("demo", "Mirror of demo")
	if err == nil || !strings.Contains(err.Error(), "not allowed to create repository in organization") ||
		!strings.Contains(err.Error(), "status code 403") {
		t.Fatalf("EnsurePublicRepo() error = %v, want organization API message and status", err)
	}
	if postRequests != 1 {
		t.Fatalf("organization repository POST requests = %d, want 1", postRequests)
	}
}

func TestGiteaClient_EnsurePublicRepoRejectsInvalidOwnerType(t *testing.T) {
	t.Parallel()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	err := NewGiteaClient(server.URL, "secret", "snonux", "Forgejo", "team").EnsurePublicRepo("demo", "")
	if err == nil || !strings.Contains(err.Error(), "invalid Forgejo owner type") {
		t.Fatalf("EnsurePublicRepo() error = %v, want invalid owner type error", err)
	}
	if requests != 0 {
		t.Fatalf("invalid owner type issued %d HTTP requests, want zero", requests)
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
			err := NewGiteaClient(server.URL, "secret", "snonux", "Forgejo", forge.OwnerTypeUser).EnsurePublicRepo("demo", "")
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

	client := NewGiteaClient(server.URL+"/custom/api", "secret", "snonux", "Forgejo", forge.OwnerTypeUser)
	if err := client.UpdateRepoDescription("demo", "new description"); err != nil {
		t.Fatalf("UpdateRepoDescription() error = %v", err)
	}
}

func TestGiteaClient_EnsurePublicRepoRequiresToken(t *testing.T) {
	t.Parallel()

	err := NewGiteaClient("https://forgejo.example/api/v1", "", "snonux", "Forgejo", forge.OwnerTypeUser).EnsurePublicRepo("demo", "")
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

	err := NewGiteaClient(server.URL, "secret", "snonux", "Forgejo", forge.OwnerTypeUser).EnsurePublicRepo("demo", "")
	if err == nil || !strings.Contains(err.Error(), "status 503") {
		t.Fatalf("EnsurePublicRepo() error = %v, want API status context", err)
	}
}

// startPaginatedReposServer serves a two-page repo listing at
// /<pathPrefix>/<org>/repos, exercising the shared listReposPaginated
// pagination loop used by both ListPublicRepos (pathPrefix "orgs") and
// ListUserPublicRepos (pathPrefix "users"). Page 1 has exactly 50 repos (the
// hardcoded page size) so the loop fetches page 2; page 2 has fewer than 50
// so pagination stops there. Both pages mix in private, fork, archived, and
// empty repos that must be filtered out, alongside the repos that must be
// kept.
func startPaginatedReposServer(t *testing.T, pathPrefix, org string) *httptest.Server {
	t.Helper()

	page1 := make([]Repository, 0, 50)
	page1 = append(page1, Repository{Name: "keep-page1-first"})
	for i := 0; i < 45; i++ {
		page1 = append(page1, Repository{Name: "private-" + strconv.Itoa(i), Private: true})
	}
	page1 = append(page1,
		Repository{Name: "fork-excluded", Fork: true},
		Repository{Name: "archived-excluded", Archived: true},
		Repository{Name: "empty-excluded", Empty: true},
		Repository{Name: "keep-page1-last"},
	)
	if len(page1) != 50 {
		t.Fatalf("test setup: page1 has %d repos, want 50", len(page1))
	}

	page2 := []Repository{
		{Name: "keep-page2"},
		{Name: "private-page2", Private: true},
	}

	wantPath := fmt.Sprintf("/%s/%s/repos", pathPrefix, org)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath {
			t.Errorf("request path = %q, want %q", r.URL.Path, wantPath)
		}
		if got := r.URL.Query().Get("limit"); got != "50" {
			t.Errorf("limit query = %q, want 50", got)
		}
		switch r.URL.Query().Get("page") {
		case "1":
			_ = json.NewEncoder(w).Encode(page1)
		case "2":
			_ = json.NewEncoder(w).Encode(page2)
		default:
			t.Errorf("unexpected page query = %q", r.URL.Query().Get("page"))
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
}

// assertKeptRepoNames checks that only the non-private, non-fork,
// non-archived, non-empty repos survived filtering, in page order.
func assertKeptRepoNames(t *testing.T, repos []Repository) {
	t.Helper()
	want := []string{"keep-page1-first", "keep-page1-last", "keep-page2"}
	if len(repos) != len(want) {
		t.Fatalf("got %d repos, want %d: %#v", len(repos), len(want), repos)
	}
	for i, name := range want {
		if repos[i].Name != name {
			t.Errorf("repos[%d].Name = %q, want %q", i, repos[i].Name, name)
		}
	}
}

func TestListPublicRepos_PaginatesFiltersAndUsesOrgEndpoint(t *testing.T) {
	t.Parallel()

	const org = "snonux"
	server := startPaginatedReposServer(t, "orgs", org)
	defer server.Close()

	client := NewGiteaClient(server.URL, "secret", org, "Forgejo", forge.OwnerTypeOrganization)
	repos, err := client.ListPublicRepos()
	if err != nil {
		t.Fatalf("ListPublicRepos() error = %v", err)
	}
	assertKeptRepoNames(t, repos)
}

func TestListUserPublicRepos_PaginatesFiltersAndUsesUserEndpoint(t *testing.T) {
	t.Parallel()

	const org = "snonux"
	server := startPaginatedReposServer(t, "users", org)
	defer server.Close()

	client := NewGiteaClient(server.URL, "secret", org, "Forgejo", forge.OwnerTypeUser)
	repos, err := client.ListUserPublicRepos()
	if err != nil {
		t.Fatalf("ListUserPublicRepos() error = %v", err)
	}
	assertKeptRepoNames(t, repos)
}
