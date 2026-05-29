package showcase

import "testing"

func TestIsCommentLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		trimmed string
		want    bool
	}{
		{name: "slash comment", trimmed: "// comment", want: true},
		{name: "hash comment", trimmed: "# comment", want: true},
		{name: "include directive", trimmed: "#include <stdio.h>", want: false},
		{name: "define directive", trimmed: "#define FOO 1", want: false},
		{name: "html comment", trimmed: "<!-- comment -->", want: true},
		{name: "block comment inner line", trimmed: "* comment", want: true},
		{name: "single asterisk", trimmed: "*", want: false},
		{name: "code line", trimmed: "fmt.Println(\"ok\")", want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := isCommentLine(tc.trimmed)
			if got != tc.want {
				t.Fatalf("isCommentLine(%q) = %v, want %v", tc.trimmed, got, tc.want)
			}
		})
	}
}

func TestStripComments_PreservesPreprocessorDirectives(t *testing.T) {
	t.Parallel()

	code := `#include <stdio.h>
#define VALUE 1
# this is a comment
int main() {
    return VALUE; // trailing comment
}`

	got := stripComments(code)
	want := `#include <stdio.h>
#define VALUE 1
int main() {
    return VALUE;
}`

	if got != want {
		t.Fatalf("stripComments() = %q, want %q", got, want)
	}
}
