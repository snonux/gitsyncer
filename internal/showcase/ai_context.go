package showcase

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// buildAIInputContext prepares a textual context for AI tools when no README exists.
// It returns a string to be piped to the AI tool's stdin and a boolean indicating
// whether this was sourced from an actual README (true) or synthesized (false).
func buildAIInputContext(repoPath string) (string, bool) {
	// 1) Try to load a README first
	readmeFiles := []string{
		"README.md", "readme.md", "Readme.md",
		"README.MD", "README.txt", "readme.txt",
		"README", "readme",
	}
	for _, f := range readmeFiles {
		p := filepath.Join(repoPath, f)
		if b, err := os.ReadFile(p); err == nil {
			return string(b), true
		}
	}

	// 2) No README: synthesize compact context
	var sb strings.Builder

	// File tree (depth-limited)
	sb.WriteString("[CONTEXT]\n")
	sb.WriteString("Repository does not contain a README.\n")
	sb.WriteString("The following is a compact file tree and key manifests/snippets.\n\n")

	sb.WriteString("FILE TREE (depth 2):\n")
	tree := listFileTree(repoPath, 2, 200)
	for _, line := range tree {
		sb.WriteString("- ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// Key manifests we often care about
	manifests := []string{
		"go.mod", "go.sum", "package.json", "Cargo.toml", "Cargo.lock",
		"pyproject.toml", "requirements.txt", "Makefile", "Dockerfile",
		"build.gradle", "pom.xml", "composer.json",
	}
	wroteHeader := false
	for _, m := range manifests {
		p := filepath.Join(repoPath, m)
		if b, err := os.ReadFile(p); err == nil {
			if !wroteHeader {
				sb.WriteString("KEY MANIFESTS:\n")
				wroteHeader = true
			}
			sb.WriteString(fmt.Sprintf("--- %s ---\n", m))
			sb.WriteString(trimTo(string(b), 2000))
			sb.WriteString("\n\n")
		}
	}

	// Source hints: capture first main-ish entry file snippets
	// Priority: Go main, Rust main, Node entry, Python main
	candidates := []string{
		"cmd", // Go convention
		"main.go",
		"cmd/main.go",
		"src/main.rs",
		"index.js",
		"src/index.js",
		"main.py",
		"src/main.py",
	}
	wroteSrc := false
	for _, c := range candidates {
		p := filepath.Join(repoPath, c)
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if info.IsDir() {
			// collect a few go files under cmd/*/main.go
			if c == "cmd" {
				_ = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
					if err != nil {
						return nil
					}
					if d.IsDir() {
						return nil
					}
					base := filepath.Base(path)
					if base == "main.go" {
						if b, e := os.ReadFile(path); e == nil {
							if !wroteSrc {
								sb.WriteString("PRIMARY SOURCE SNIPPETS:\n")
								wroteSrc = true
							}
							rel, _ := filepath.Rel(repoPath, path)
							sb.WriteString(fmt.Sprintf("--- %s ---\n", rel))
							sb.WriteString(trimTo(string(b), 2000))
							sb.WriteString("\n\n")
						}
					}
					return nil
				})
			}
			continue
		}
		if b, e := os.ReadFile(p); e == nil {
			if !wroteSrc {
				sb.WriteString("PRIMARY SOURCE SNIPPETS:\n")
				wroteSrc = true
			}
			rel, _ := filepath.Rel(repoPath, p)
			sb.WriteString(fmt.Sprintf("--- %s ---\n", rel))
			sb.WriteString(trimTo(string(b), 2000))
			sb.WriteString("\n\n")
		}
	}

	// Fallback: include a few top-level .go, .rs, .py, .js files if we still have nothing
	if !wroteSrc {
		topFiles := listTopFiles(repoPath, []string{".go", ".rs", ".py", ".js", ".ts", ".tsx"}, 5)
		for _, f := range topFiles {
			if b, e := os.ReadFile(filepath.Join(repoPath, f)); e == nil {
				if !wroteSrc {
					sb.WriteString("PRIMARY SOURCE SNIPPETS:\n")
					wroteSrc = true
				}
				sb.WriteString(fmt.Sprintf("--- %s ---\n", f))
				sb.WriteString(trimTo(string(b), 2000))
				sb.WriteString("\n\n")
			}
		}
	}

	// Instruction to the model
	sb.WriteString("[TASK]\n")
	sb.WriteString("Summarize this project in 1–2 paragraphs: what it does, why it's useful, and how it's implemented. Mention notable tech choices. Be concise and informative.\n")

	return sb.String(), false
}

// listFileTree returns a sorted list of relative paths up to a given depth and limit.
func listFileTree(root string, maxDepth int, maxEntries int) []string {
	var entries []string
	var count int
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == root {
			return nil
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return nil
		}
		// depth check
		depth := 1 + strings.Count(rel, string(os.PathSeparator))
		if depth > maxDepth {
			return fs.SkipDir
		}
		entries = append(entries, rel)
		count++
		if count >= maxEntries {
			return fs.SkipDir
		}
		return nil
	})
	sort.Strings(entries)
	if len(entries) > maxEntries {
		entries = entries[:maxEntries]
	}
	return entries
}

// listTopFiles lists top-level files with certain extensions up to a limit.
func listTopFiles(root string, exts []string, limit int) []string {
	dir, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range dir {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		for _, x := range exts {
			if strings.HasSuffix(strings.ToLower(name), strings.ToLower(x)) {
				out = append(out, name)
				break
			}
		}
		if len(out) >= limit {
			break
		}
	}
	sort.Strings(out)
	return out
}

// trimTo soft-limits content length for inclusion in AI context.
func trimTo(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n... [truncated]"
}
