package forgejo

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/snonux/gitsyncer/internal/forge"
)

// TestCreateRelease_RefusesWhenTagMissing guards against Forgejo creating a
// missing tag at the default branch HEAD: when the tag lookup answers 404,
// no release may be posted.
func TestCreateRelease_RefusesWhenTagMissing(t *testing.T) {
	t.Parallel()

	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/snonux/demo/tags/v1.0.0":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost:
			posts.Add(1)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewGiteaClient(server.URL, "secret", "snonux", "Forgejo", forge.OwnerTypeUser)
	err := client.CreateRelease("snonux", "demo", "v1.0.0", "notes")
	if err == nil || !strings.Contains(err.Error(), "does not exist on Forgejo") {
		t.Fatalf("CreateRelease() error = %v, want missing-tag error", err)
	}
	if got := posts.Load(); got != 0 {
		t.Fatalf("release POSTs = %d, want 0", got)
	}
}

// TestCreateRelease_CreatesWhenTagExists checks the normal path still posts
// the release once the tag is present on Forgejo.
func TestCreateRelease_CreatesWhenTagExists(t *testing.T) {
	t.Parallel()

	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/snonux/demo/tags/v1.0.0":
			_, _ = w.Write([]byte(`{"name":"v1.0.0"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/snonux/demo/releases":
			posts.Add(1)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewGiteaClient(server.URL, "secret", "snonux", "Forgejo", forge.OwnerTypeUser)
	if err := client.CreateRelease("snonux", "demo", "v1.0.0", "notes"); err != nil {
		t.Fatalf("CreateRelease() error = %v", err)
	}
	if got := posts.Load(); got != 1 {
		t.Fatalf("release POSTs = %d, want 1", got)
	}
}
