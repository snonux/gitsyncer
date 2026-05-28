package forge

import (
	"errors"
	"strings"
	"testing"
)

func TestCheckRepoExists(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		exists, err := CheckRepoExists("repo", func(string) (bool, error) {
			return true, nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !exists {
			t.Fatal("expected repo to exist")
		}
	})

	t.Run("missing", func(t *testing.T) {
		exists, err := CheckRepoExists("repo", func(string) (bool, error) {
			return false, nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if exists {
			t.Fatal("expected repo to be missing")
		}
	})

	t.Run("wraps error", func(t *testing.T) {
		original := errors.New("boom")
		_, err := CheckRepoExists("repo", func(string) (bool, error) {
			return false, original
		})
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "failed to check if repo exists") {
			t.Fatalf("expected wrapped message, got %q", err.Error())
		}
		if !errors.Is(err, original) {
			t.Fatal("expected wrapped original error")
		}
	})
}

func TestEnsureRepoExists(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		err := EnsureRepoExists("org", "repo", func(string) (bool, error) {
			return true, nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		err := EnsureRepoExists("org", "repo", func(string) (bool, error) {
			return false, nil
		})
		if err == nil {
			t.Fatal("expected an error")
		}
		if got, want := err.Error(), "repository org/repo does not exist"; got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})
}

func TestDeleteStatusError(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		errMsg string
	}{
		{name: "deleted", status: 204, body: ""},
		{name: "already gone", status: 404, body: ""},
		{name: "forbidden", status: 403, body: "nope", errMsg: "permission denied (403): nope"},
		{name: "unauthorized", status: 401, body: "bad creds", errMsg: "authentication failed (401): bad creds"},
		{name: "unexpected", status: 500, body: "oops", errMsg: "failed to delete repository: status 500: oops"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := DeleteStatusError(tc.status, tc.body)
			if tc.errMsg == "" {
				if err != nil {
					t.Fatalf("expected nil, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected an error")
			}
			if got := err.Error(); got != tc.errMsg {
				t.Fatalf("expected %q, got %q", tc.errMsg, got)
			}
		})
	}
}
