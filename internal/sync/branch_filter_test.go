package sync

import (
	"reflect"
	"testing"
)

func TestFilterAllowedBranches(t *testing.T) {
	t.Parallel()

	got := FilterAllowedBranches(
		[]string{"feature/new", "main", "develop", "release"},
		[]string{"main", "develop"},
	)
	want := []string{"main", "develop"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FilterAllowedBranches() = %v, want %v", got, want)
	}
}

func TestFilterAllowedBranches_EmptyAllowlistFiltersAll(t *testing.T) {
	t.Parallel()

	got := FilterAllowedBranches([]string{"main", "topic"}, nil)
	if len(got) != 0 {
		t.Fatalf("FilterAllowedBranches() = %v, want no branches", got)
	}
}
