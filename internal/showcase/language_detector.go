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
func detectLanguages(repoPath string) ([]LanguageStats, error) {
	languageLines := make(map[string]int)
	
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
		".md":    "Markdown",
		".rst":   "reStructuredText",
		".tex":   "LaTeX",
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
	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
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

		// Determine the language
		var language string
		
		// Check special files first
		if lang, ok := specialFiles[basename]; ok {
			language = lang
		} else {
			// Check by extension
			if lang, ok := langExtensions[ext]; ok {
				language = lang
			}
		}

		// If we identified a language, count its lines
		if language != "" {
			lines, err := countFileLines(path)
			if err == nil {
				languageLines[language] += lines
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Calculate total lines
	totalLines := 0
	for _, lines := range languageLines {
		totalLines += lines
	}

	// Convert to LanguageStats with percentages
	var stats []LanguageStats
	for lang, lines := range languageLines {
		percentage := 0.0
		if totalLines > 0 {
			percentage = float64(lines) * 100.0 / float64(totalLines)
		}
		stats = append(stats, LanguageStats{
			Name:       lang,
			Lines:      lines,
			Percentage: percentage,
		})
	}

	// Sort by percentage (descending)
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Percentage > stats[j].Percentage
	})

	return stats, nil
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