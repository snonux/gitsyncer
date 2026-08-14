package showcase

import (
	"path/filepath"
	"strings"
	"testing"
)

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

// TestExtractCodeSnippet_SkipsVendorDirAndOversizedFiles exercises the
// filepath.WalkDir-based walk in extractCodeSnippet: files under vendor/
// must be skipped via the directory-skip rule, and files over 1MB must be
// skipped via the per-file size check (which now requires an explicit
// d.Info() call since WalkDir's fs.DirEntry doesn't carry size).
func TestExtractCodeSnippet_SkipsVendorDirAndOversizedFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	writeTestFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {\n\tprintln(\"hi\")\n}\n")
	writeTestFile(t, filepath.Join(dir, "vendor", "ignored.go"), "package vendor\n\nfunc Ignored() {}\n")

	huge := "package huge\n\n// " + strings.Repeat("x", 2*1024*1024) + "\n"
	writeTestFile(t, filepath.Join(dir, "huge.go"), huge)

	_, desc, err := extractCodeSnippet(dir, []LanguageStats{{Name: "Go", Percentage: 100}})
	if err != nil {
		t.Fatalf("extractCodeSnippet() error = %v", err)
	}

	if !strings.Contains(desc, "main.go") {
		t.Fatalf("extractCodeSnippet() description = %q, want it to reference main.go", desc)
	}
	if strings.Contains(desc, "vendor") || strings.Contains(desc, "huge.go") {
		t.Fatalf("extractCodeSnippet() description = %q, vendor/oversized files must be skipped", desc)
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
