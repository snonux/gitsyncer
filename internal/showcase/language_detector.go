package showcase

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// detectLanguages detects programming languages used in the repository with line counts
// Returns both programming languages and documentation/text files separately
func detectLanguages(repoPath string) (languages []LanguageStats, documentation []LanguageStats, err error) {
	languageLines := make(map[string]int)
	documentationLines := make(map[string]int)
	
	// Define common language extensions
	langExtensions := map[string]string{
		".go":    "Go",
		".py":    "Python",
		".js":    "JavaScript",
		".ts":    "TypeScript",
		".java":  "Java",
		".c":     "C",
		".cpp":   "C++",
		".cc":    "C++",
		".cxx":   "C++",
		".h":     "C/C++",
		".hpp":   "C++",
		".hxx":   "C++",
		".cs":    "C#",
		".rb":    "Ruby",
		".php":   "PHP",
		".swift": "Swift",
		".kt":    "Kotlin",
		".rs":    "Rust",
		".scala": "Scala",
		".r":     "R",
		".m":     "Objective-C",
		".mm":    "Objective-C++",
		".sh":    "Shell",
		".bash":  "Shell",
		".zsh":   "Shell",
		".fish":  "Shell",
		".pl":    "Perl",
		".pm":    "Perl",
		".raku":  "Raku",
		".rakumod": "Raku",
		".rakudoc": "Raku",
		".rakutest": "Raku",
		".p6":    "Raku",
		".pm6":   "Raku",
		".lua":   "Lua",
		".vim":   "Vim Script",
		".el":    "Emacs Lisp",
		".clj":   "Clojure",
		".hs":    "Haskell",
		".ml":    "OCaml",
		".ex":    "Elixir",
		".exs":   "Elixir",
		".dart":  "Dart",
		".jl":    "Julia",
		".nim":   "Nim",
		".v":     "V",
		".zig":   "Zig",
		".html":  "HTML",
		".htm":   "HTML",
		".css":   "CSS",
		".scss":  "SCSS",
		".sass":  "Sass",
		".less":  "Less",
		".xml":   "XML",
		".json":  "JSON",
		".yaml":  "YAML",
		".yml":   "YAML",
		".toml":  "TOML",
		".ini":   "INI",
		".cfg":   "Config",
		".conf":  "Config",
		".sql":   "SQL",
		".tf":    "HCL",
		".tfvars": "HCL",
		".hcl":   "HCL",
		".awk":   "AWK",
	}
	
	// Define documentation/text extensions
	docExtensions := map[string]string{
		".md":    "Markdown",
		".rst":   "reStructuredText",
		".tex":   "LaTeX",
		".txt":   "Text",
		".adoc":  "AsciiDoc",
		".org":   "Org",
	}

	// Special files that indicate specific languages
	specialFiles := map[string]string{
		"makefile":            "Make",
		"gnumakefile":         "Make",
		"dockerfile":          "Docker",
		"dockerfile.*":        "Docker",
		"cmakelists.txt":      "CMake",
		"rakefile":            "Ruby",
		"gemfile":             "Ruby",
		"package.json":        "JavaScript",
		"cargo.toml":          "Rust",
		"go.mod":              "Go",
		"go.sum":              "Go",
		"pom.xml":             "Java",
		"build.gradle":        "Gradle",
		"build.gradle.kts":    "Kotlin",
		"requirements.txt":    "Python",
		"setup.py":            "Python",
		"pyproject.toml":      "Python",
		"composer.json":       "PHP",
		"*.dockerfile":        "Docker",
		"containerfile":       "Docker",
		"jenkinsfile":         "Groovy",
		"vagrantfile":         "Ruby",
	}

	// Count lines for each language
	err = filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip directories
		if info.IsDir() {
			name := info.Name()
			// Skip hidden directories and common non-code directories
			if strings.HasPrefix(name, ".") && name != "." || 
			   name == "node_modules" || 
			   name == "vendor" || 
			   name == "target" || 
			   name == "dist" || 
			   name == "build" || 
			   name == "out" ||
			   name == "__pycache__" ||
			   name == "coverage" {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip binary and large files
		if info.Size() > 10*1024*1024 { // Skip files larger than 10MB
			return nil
		}

		// Get the filename and extension
		basename := strings.ToLower(filepath.Base(path))
		ext := strings.ToLower(filepath.Ext(path))

		// Determine the language or documentation type
		var language string
		var isDoc bool
		
		// Check special files first
		if lang, ok := specialFiles[basename]; ok {
			language = lang
		} else {
			// Check documentation extensions
			if docType, ok := docExtensions[ext]; ok {
				language = docType
				isDoc = true
			} else if lang, ok := langExtensions[ext]; ok {
				// Check programming language extensions
				language = lang
			}
		}

		// If we identified a language, count its lines
		if language != "" {
			lines, err := countFileLines(path)
			if err == nil {
				if isDoc {
					documentationLines[language] += lines
				} else {
					languageLines[language] += lines
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	// Process programming languages
	totalCodeLines := 0
	for _, lines := range languageLines {
		totalCodeLines += lines
	}

	var languageStats []LanguageStats
	for lang, lines := range languageLines {
		percentage := 0.0
		if totalCodeLines > 0 {
			percentage = float64(lines) * 100.0 / float64(totalCodeLines)
		}
		languageStats = append(languageStats, LanguageStats{
			Name:       lang,
			Lines:      lines,
			Percentage: percentage,
		})
	}

	// Sort languages by percentage (descending)
	sort.Slice(languageStats, func(i, j int) bool {
		return languageStats[i].Percentage > languageStats[j].Percentage
	})

	// Process documentation
	totalDocLines := 0
	for _, lines := range documentationLines {
		totalDocLines += lines
	}

	var docStats []LanguageStats
	for docType, lines := range documentationLines {
		percentage := 0.0
		if totalDocLines > 0 {
			percentage = float64(lines) * 100.0 / float64(totalDocLines)
		}
		docStats = append(docStats, LanguageStats{
			Name:       docType,
			Lines:      lines,
			Percentage: percentage,
		})
	}

	// Sort documentation by percentage (descending)
	sort.Slice(docStats, func(i, j int) bool {
		return docStats[i].Percentage > docStats[j].Percentage
	})

	return languageStats, docStats, nil
}

// countFileLines counts the number of lines in a file
func countFileLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

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