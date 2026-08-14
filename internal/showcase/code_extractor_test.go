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

func TestResolveLanguageExtensions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		languages   []LanguageStats
		wantLang    string
		wantExtsLen int
		wantErr     bool
	}{
		{
			name:        "primary language known",
			languages:   []LanguageStats{{Name: "Go", Percentage: 100}},
			wantLang:    "Go",
			wantExtsLen: 1,
		},
		{
			name: "falls back to next known language",
			languages: []LanguageStats{
				{Name: "TotallyUnknownLang", Percentage: 60},
				{Name: "Python", Percentage: 40},
			},
			wantLang:    "Python",
			wantExtsLen: 1,
		},
		{
			name:      "no known languages",
			languages: []LanguageStats{{Name: "TotallyUnknownLang", Percentage: 100}},
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotLang, gotExts, err := resolveLanguageExtensions(tc.languages)
			if (err != nil) != tc.wantErr {
				t.Fatalf("resolveLanguageExtensions() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if gotLang != tc.wantLang {
				t.Fatalf("resolveLanguageExtensions() lang = %q, want %q", gotLang, tc.wantLang)
			}
			if len(gotExts) != tc.wantExtsLen {
				t.Fatalf("resolveLanguageExtensions() extensions = %v, want length %d", gotExts, tc.wantExtsLen)
			}
		})
	}
}

// TestFindCodeFiles_SkipsVendorTestsAndOversizedFiles exercises findCodeFiles
// directly (rather than through extractCodeSnippet) to confirm it applies
// the directory-skip, size, and test/generated filters on its own.
func TestFindCodeFiles_SkipsVendorTestsAndOversizedFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	writeTestFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")
	writeTestFile(t, filepath.Join(dir, "main_test.go"), "package main\n\nfunc TestMain(t *testing.T) {}\n")
	writeTestFile(t, filepath.Join(dir, "vendor", "ignored.go"), "package vendor\n\nfunc Ignored() {}\n")

	huge := "package huge\n\n// " + strings.Repeat("x", 2*1024*1024) + "\n"
	writeTestFile(t, filepath.Join(dir, "huge.go"), huge)

	files, err := findCodeFiles(dir, "Go", langExtensions["Go"])
	if err != nil {
		t.Fatalf("findCodeFiles() error = %v", err)
	}

	if len(files) != 1 || filepath.Base(files[0]) != "main.go" {
		t.Fatalf("findCodeFiles() = %v, want only main.go", files)
	}
}

func TestPickSnippet_ReturnsErrorForEmptyOrUnreadableCandidates(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist.go")

	if _, _, err := pickSnippet([]string{missing}); err == nil {
		t.Fatal("pickSnippet() error = nil, want error for unreadable candidate")
	}
}

func TestPickSnippet_ExtractsFromCandidateFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	writeTestFile(t, path, "package main\n\nfunc main() {\n\tprintln(\"hi\")\n}\n")

	snippet, selected, err := pickSnippet([]string{path})
	if err != nil {
		t.Fatalf("pickSnippet() error = %v", err)
	}
	if selected != path {
		t.Fatalf("pickSnippet() selected = %q, want %q", selected, path)
	}
	if snippet == "" {
		t.Fatal("pickSnippet() returned empty snippet")
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
