package version

import "testing"

func TestIsVersionTag(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		tag  string
		want bool
	}{
		{name: "single digit", tag: "1", want: true},
		{name: "single digit with v prefix", tag: "v1", want: true},
		{name: "major minor", tag: "1.2", want: true},
		{name: "major minor with v prefix", tag: "v1.2", want: true},
		{name: "major minor patch", tag: "1.2.3", want: true},
		{name: "major minor patch with v prefix", tag: "v1.2.3", want: true},
		{name: "empty", tag: "", want: false},
		{name: "letter prefix", tag: "release-1.2.3", want: false},
		{name: "too many components", tag: "1.2.3.4", want: false},
		{name: "trailing dot", tag: "1.", want: false},
		{name: "prerelease style", tag: "1-beta", want: false},
		{name: "prerelease style with v prefix", tag: "v1-beta", want: false},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := IsVersionTag(tc.tag)
			if got != tc.want {
				t.Fatalf("IsVersionTag(%q) = %v, want %v", tc.tag, got, tc.want)
			}
		})
	}
}
