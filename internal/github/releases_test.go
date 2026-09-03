package github

import (
	"net/http"
	"testing"
)

func TestReleaseAlreadyExists(t *testing.T) {
	t.Parallel()

	existing := []byte(`{"message":"Validation Failed","errors":[{"resource":"Release","code":"already_exists","field":"tag_name"}]}`)
	if !releaseAlreadyExists(http.StatusUnprocessableEntity, existing) {
		t.Fatal("expected GitHub 422 already_exists on tag_name to match")
	}
	if releaseAlreadyExists(http.StatusCreated, existing) {
		t.Fatal("did not expect a 201 to be treated as already exists")
	}
	if releaseAlreadyExists(http.StatusUnprocessableEntity, []byte(`{"message":"Validation Failed"}`)) {
		t.Fatal("did not expect a 422 without already_exists to match")
	}
}
