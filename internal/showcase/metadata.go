package showcase

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// RepoMetadata holds metadata about a repository
type RepoMetadata struct {
	Languages       []string
	CommitCount     int
	LinesOfCode     int
	FirstCommitDate string
	LastCommitDate  string
	License         string
	AvgCommitAge    float64 // Average age of last 100 commits in days
}

// extractRepoMetadata extracts metadata from a repository
func extractRepoMetadata(repoPath string) (*RepoMetadata, error) {
	metadata := &RepoMetadata{}

	// Get programming languages by analyzing file extensions
	languages, err := detectLanguages(repoPath)
	if err != nil {
		fmt.Printf("Warning: Failed to detect languages: %v\n", err)
	}
	metadata.Languages = languages

	// Get commit count
	commitCount, err := getCommitCount(repoPath)
	if err != nil {
		fmt.Printf("Warning: Failed to get commit count: %v\n", err)
	}
	metadata.CommitCount = commitCount

	// Get lines of code
	loc, err := countLinesOfCode(repoPath)
	if err != nil {
		fmt.Printf("Warning: Failed to count lines of code: %v\n", err)
	}
	metadata.LinesOfCode = loc

	// Get first and last commit dates
	firstDate, err := getFirstCommitDate(repoPath)
	if err != nil {
		fmt.Printf("Warning: Failed to get first commit date: %v\n", err)
	}
	metadata.FirstCommitDate = firstDate

	lastDate, err := getLastCommitDate(repoPath)
	if err != nil {
		fmt.Printf("Warning: Failed to get last commit date: %v\n", err)
	}
	metadata.LastCommitDate = lastDate

	// Check for license file
	license := detectLicense(repoPath)
	metadata.License = license

	// Get average age of last 100 commits
	avgAge, err := getAverageCommitAge(repoPath, 100)
	if err != nil {
		fmt.Printf("Warning: Failed to get average commit age: %v\n", err)
	}
	metadata.AvgCommitAge = avgAge

	return metadata, nil
}

// detectLanguages detects programming languages used in the repository
func detectLanguages(repoPath string) ([]string, error) {
	languageMap := make(map[string]bool)
	
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
		".h":     "C/C++",
		".hpp":   "C++",
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
		".bash":  "Bash",
		".zsh":   "Zsh",
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
	}

	// Walk through the repository
	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip hidden directories and common non-code directories
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "target" || name == "dist" || name == "build" {
				return filepath.SkipDir
			}
			return nil
		}

		// Check file extension
		ext := strings.ToLower(filepath.Ext(path))
		if lang, ok := langExtensions[ext]; ok {
			languageMap[lang] = true
		}

		// Check for special files
		basename := filepath.Base(path)
		switch strings.ToLower(basename) {
		case "makefile", "gnumakefile":
			languageMap["Make"] = true
		case "dockerfile":
			languageMap["Docker"] = true
		case "cmakelists.txt":
			languageMap["CMake"] = true
		case "rakefile":
			languageMap["Ruby"] = true
		case "gemfile":
			languageMap["Ruby"] = true
		case "package.json":
			languageMap["JavaScript/Node.js"] = true
		case "cargo.toml":
			languageMap["Rust"] = true
		case "go.mod":
			languageMap["Go"] = true
		case "pom.xml":
			languageMap["Java/Maven"] = true
		case "build.gradle", "build.gradle.kts":
			languageMap["Java/Gradle"] = true
		case "requirements.txt", "setup.py", "pyproject.toml":
			languageMap["Python"] = true
		case "composer.json":
			languageMap["PHP"] = true
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Convert map to slice
	var languages []string
	for lang := range languageMap {
		languages = append(languages, lang)
	}

	return languages, nil
}

// getCommitCount returns the total number of commits
func getCommitCount(repoPath string) (int, error) {
	cmd := exec.Command("git", "-C", repoPath, "rev-list", "--all", "--count")
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	count, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0, err
	}

	return count, nil
}

// countLinesOfCode counts lines of code (excluding binary files and common non-code files)
func countLinesOfCode(repoPath string) (int, error) {
	// Use git ls-files to get tracked files, then count lines
	// Exclude binary files and common non-code files
	cmd := exec.Command("bash", "-c", fmt.Sprintf(
		`cd "%s" && git ls-files | grep -E '\.(go|py|js|ts|java|c|cpp|h|hpp|cs|rb|php|swift|kt|rs|scala|r|sh|bash|zsh|pl|lua|vim|el|clj|hs|ml|ex|exs|dart|jl|nim|v|zig|html|css|scss|sass|json|xml|yaml|yml|toml|ini|conf|cfg)$' | xargs wc -l 2>/dev/null | tail -n 1 | awk '{print $1}'`,
		repoPath,
	))
	
	output, err := cmd.Output()
	if err != nil {
		// Fallback: try a simpler approach
		cmd = exec.Command("bash", "-c", fmt.Sprintf(
			`find "%s" -type f -name "*.go" -o -name "*.py" -o -name "*.js" -o -name "*.java" -o -name "*.c" -o -name "*.cpp" -o -name "*.rs" | xargs wc -l 2>/dev/null | tail -n 1 | awk '{print $1}'`,
			repoPath,
		))
		output, err = cmd.Output()
		if err != nil {
			return 0, err
		}
	}

	loc, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0, err
	}

	return loc, nil
}

// getFirstCommitDate returns the date of the first commit
func getFirstCommitDate(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "log", "--reverse", "--pretty=format:%ai", "--date=short")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 && lines[0] != "" {
		// Extract just the date part (YYYY-MM-DD)
		parts := strings.Fields(lines[0])
		if len(parts) > 0 {
			return parts[0], nil
		}
	}

	return "", fmt.Errorf("no commits found")
}

// getLastCommitDate returns the date of the last commit
func getLastCommitDate(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "log", "-1", "--pretty=format:%ai", "--date=short")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// Extract just the date part (YYYY-MM-DD)
	parts := strings.Fields(string(output))
	if len(parts) > 0 {
		return parts[0], nil
	}

	return "", fmt.Errorf("no commits found")
}

// detectLicense checks for common license files
func detectLicense(repoPath string) string {
	licenseFiles := []string{
		"LICENSE",
		"LICENSE.txt",
		"LICENSE.md",
		"license",
		"license.txt",
		"license.md",
		"COPYING",
		"COPYING.txt",
		"COPYRIGHT",
		"COPYRIGHT.txt",
	}

	for _, filename := range licenseFiles {
		path := filepath.Join(repoPath, filename)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			// Try to detect license type by reading the file
			content, err := os.ReadFile(path)
			if err == nil {
				contentStr := string(content)
				switch {
				case strings.Contains(contentStr, "MIT License"):
					return "MIT"
				case strings.Contains(contentStr, "Apache License") && strings.Contains(contentStr, "Version 2.0"):
					return "Apache-2.0"
				case strings.Contains(contentStr, "GNU GENERAL PUBLIC LICENSE") && strings.Contains(contentStr, "Version 3"):
					return "GPL-3.0"
				case strings.Contains(contentStr, "GNU GENERAL PUBLIC LICENSE") && strings.Contains(contentStr, "Version 2"):
					return "GPL-2.0"
				case strings.Contains(contentStr, "BSD 3-Clause License"):
					return "BSD-3-Clause"
				case strings.Contains(contentStr, "BSD 2-Clause License"):
					return "BSD-2-Clause"
				case strings.Contains(contentStr, "Mozilla Public License Version 2.0"):
					return "MPL-2.0"
				case strings.Contains(contentStr, "ISC License"):
					return "ISC"
				case strings.Contains(contentStr, "GNU LESSER GENERAL PUBLIC LICENSE"):
					return "LGPL"
				case strings.Contains(contentStr, "The Unlicense"):
					return "Unlicense"
				case strings.Contains(contentStr, "CC0"):
					return "CC0"
				default:
					return "Custom License"
				}
			}
			return "License file found"
		}
	}

	return "No license found"
}

// getAverageCommitAge calculates the average age of the last N commits in days
func getAverageCommitAge(repoPath string, commitCount int) (float64, error) {
	// Get the last N commit dates
	cmd := exec.Command("git", "-C", repoPath, "log", fmt.Sprintf("-%d", commitCount), "--pretty=format:%at")
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return 0, fmt.Errorf("no commits found")
	}

	// Calculate average age
	now := float64(time.Now().Unix())
	var totalAge float64
	validCommits := 0

	for _, line := range lines {
		if line == "" {
			continue
		}
		
		timestamp, err := strconv.ParseInt(line, 10, 64)
		if err != nil {
			continue
		}
		
		age := (now - float64(timestamp)) / 86400 // Convert to days
		totalAge += age
		validCommits++
	}

	if validCommits == 0 {
		return 0, fmt.Errorf("no valid commits found")
	}

	return totalAge / float64(validCommits), nil
}