package showcase

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// maxScannableFileSize caps the size of files considered for language
// detection; anything larger is skipped to avoid scanning huge binaries
// or generated artifacts line-by-line.
const maxScannableFileSize = 10 * 1024 * 1024 // 10MB

// languageExtensions maps common file extensions to their programming
// language name. Declared once at package scope so it isn't rebuilt on
// every detectLanguages call.
var languageExtensions = map[string]string{
	".go":       "Go",
	".py":       "Python",
	".js":       "JavaScript",
	".ts":       "TypeScript",
	".java":     "Java",
	".c":        "C",
	".cpp":      "C++",
	".cc":       "C++",
	".cxx":      "C++",
	".h":        "C/C++",
	".hpp":      "C++",
	".hxx":      "C++",
	".cs":       "C#",
	".rb":       "Ruby",
	".php":      "PHP",
	".swift":    "Swift",
	".kt":       "Kotlin",
	".rs":       "Rust",
	".scala":    "Scala",
	".r":        "R",
	".m":        "Objective-C",
	".mm":       "Objective-C++",
	".sh":       "Shell",
	".bash":     "Shell",
	".zsh":      "Shell",
	".fish":     "Shell",
	".pl":       "Perl",
	".pm":       "Perl",
	".raku":     "Raku",
	".rakumod":  "Raku",
	".rakudoc":  "Raku",
	".rakutest": "Raku",
	".p6":       "Raku",
	".pm6":      "Raku",
	".lua":      "Lua",
	".vim":      "Vim Script",
	".el":       "Emacs Lisp",
	".clj":      "Clojure",
	".hs":       "Haskell",
	".ml":       "OCaml",
	".ex":       "Elixir",
	".exs":      "Elixir",
	".dart":     "Dart",
	".jl":       "Julia",
	".nim":      "Nim",
	".v":        "V",
	".zig":      "Zig",
	".html":     "HTML",
	".htm":      "HTML",
	".css":      "CSS",
	".scss":     "SCSS",
	".sass":     "Sass",
	".less":     "Less",
	".xml":      "XML",
	".json":     "JSON",
	".yaml":     "YAML",
	".yml":      "YAML",
	".toml":     "TOML",
	".ini":      "INI",
	".cfg":      "Config",
	".conf":     "Config",
	".sql":      "SQL",
	".tf":       "HCL",
	".tfvars":   "HCL",
	".hcl":      "HCL",
	".awk":      "AWK",
}

// documentationExtensions maps extensions that indicate documentation/text
// files, which are tracked separately from programming languages.
var documentationExtensions = map[string]string{
	".md":   "Markdown",
	".rst":  "reStructuredText",
	".tex":  "LaTeX",
	".txt":  "Text",
	".adoc": "AsciiDoc",
	".org":  "Org",
}

// specialFileLanguages maps well-known filenames (lower-cased, no
// extension-based detection possible) to the language/tooling they
// represent, e.g. "Makefile" or "go.mod".
var specialFileLanguages = map[string]string{
	"makefile":         "Make",
	"gnumakefile":      "Make",
	"dockerfile":       "Docker",
	"dockerfile.*":     "Docker",
	"cmakelists.txt":   "CMake",
	"rakefile":         "Ruby",
	"gemfile":          "Ruby",
	"package.json":     "JavaScript",
	"cargo.toml":       "Rust",
	"go.mod":           "Go",
	"go.sum":           "Go",
	"pom.xml":          "Java",
	"build.gradle":     "Gradle",
	"build.gradle.kts": "Kotlin",
	"requirements.txt": "Python",
	"setup.py":         "Python",
	"pyproject.toml":   "Python",
	"composer.json":    "PHP",
	"*.dockerfile":     "Docker",
	"containerfile":    "Docker",
	"jenkinsfile":      "Groovy",
	"vagrantfile":      "Ruby",
}

// detectLanguages detects programming languages used in the repository with
// line counts. Returns both programming languages and documentation/text
// files separately, each sorted by descending percentage of the total.
func detectLanguages(repoPath string) (languages []LanguageStats, documentation []LanguageStats, err error) {
	languageLines, documentationLines, err := countLanguageLines(repoPath)
	if err != nil {
		return nil, nil, err
	}

	languageStats := buildSortedPercentageStats(languageLines, sumLines(languageLines))
	docStats := buildSortedPercentageStats(documentationLines, sumLines(documentationLines))

	return languageStats, docStats, nil
}

// countLanguageLines walks the repository tree, classifies each file as a
// programming language or documentation type, and accumulates line counts
// per detected type. Directories that are hidden or known non-source
// locations (vendor, node_modules, build output, ...) are skipped entirely.
func countLanguageLines(repoPath string) (languageLines map[string]int, documentationLines map[string]int, err error) {
	languageLines = make(map[string]int)
	documentationLines = make(map[string]int)

	err = filepath.Walk(repoPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // Skip errors
		}

		if info.IsDir() {
			if shouldSkipDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		if info.Size() > maxScannableFileSize {
			return nil
		}

		language, isDoc := classifyFile(path, info)
		if language == "" {
			return nil
		}

		lines, lineErr := countFileLines(path)
		if lineErr != nil {
			return nil
		}
		if isDoc {
			documentationLines[language] += lines
		} else {
			languageLines[language] += lines
		}

		return nil
	})

	return languageLines, documentationLines, err
}

// shouldSkipDir reports whether a directory should be excluded from the
// language scan: hidden directories (other than "." itself) and well-known
// non-source directories such as dependency caches and build output.
func shouldSkipDir(name string) bool {
	if strings.HasPrefix(name, ".") && name != "." {
		return true
	}
	switch name {
	case "node_modules", "vendor", "target", "dist", "build", "out", "__pycache__", "coverage":
		return true
	}
	return false
}

// classifyFile determines the language or documentation type for a file.
// It checks, in order: well-known special filenames, documentation
// extensions, programming language extensions, and finally (for
// extension-less executables) the shebang line.
func classifyFile(path string, info os.FileInfo) (language string, isDoc bool) {
	basename := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(path))

	if lang, ok := specialFileLanguages[basename]; ok {
		return lang, false
	}
	if docType, ok := documentationExtensions[ext]; ok {
		return docType, true
	}
	if lang, ok := languageExtensions[ext]; ok {
		return lang, false
	}

	if info.Mode()&0111 != 0 {
		return detectShebangLanguage(path), false
	}

	return "", false
}

// detectShebangLanguage inspects the first line of an executable file for a
// shebang and maps known interpreters to a language name. Returns "" if the
// file can't be read, has no shebang, or the interpreter is unrecognized.
func detectShebangLanguage(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer closeFile(file)

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return ""
	}

	firstLine := scanner.Text()
	if !strings.HasPrefix(firstLine, "#!") {
		return ""
	}

	// Check for various interpreters in the shebang line.
	switch {
	case strings.Contains(firstLine, "python"):
		return "Python"
	case strings.Contains(firstLine, "node"):
		return "JavaScript"
	case strings.Contains(firstLine, "ruby"):
		return "Ruby"
	case strings.Contains(firstLine, "perl") && !strings.Contains(firstLine, "perl6"):
		return "Perl"
	case strings.Contains(firstLine, "perl6") || strings.Contains(firstLine, "raku"):
		return "Raku"
	case strings.Contains(firstLine, "awk") || strings.Contains(firstLine, "gawk") || strings.Contains(firstLine, "mawk"):
		return "AWK"
	case strings.Contains(firstLine, "sh") || strings.Contains(firstLine, "bash") || strings.Contains(firstLine, "zsh") || strings.Contains(firstLine, "fish"):
		return "Shell"
	case strings.Contains(firstLine, "php"):
		return "PHP"
	case strings.Contains(firstLine, "lua"):
		return "Lua"
	}

	return ""
}

// sumLines totals the line counts across all detected languages/types, used
// as the denominator when computing per-language percentages.
func sumLines(counts map[string]int) int {
	total := 0
	for _, lines := range counts {
		total += lines
	}
	return total
}

// countFileLines counts the number of lines in a file
func countFileLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer closeFile(file)

	scanner := bufio.NewScanner(file)
	lines := 0
	for scanner.Scan() {
		lines++
	}

	if err := scanner.Err(); err != nil {
		return 0, err
	}

	return lines, nil
}

// FormatLanguagesWithPercentages formats languages with their percentages
func FormatLanguagesWithPercentages(languages []LanguageStats) string {
	if len(languages) == 0 {
		return ""
	}

	var parts []string
	for _, lang := range languages {
		if lang.Percentage >= 0.1 { // Only show languages with at least 0.1%
			parts = append(parts, fmt.Sprintf("%s (%.1f%%)", lang.Name, lang.Percentage))
		}
	}

	// If all languages are below 0.1%, just show the names
	if len(parts) == 0 {
		for _, lang := range languages {
			parts = append(parts, lang.Name)
		}
	}

	return strings.Join(parts, ", ")
}
