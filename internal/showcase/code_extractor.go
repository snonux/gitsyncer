package showcase

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// extractCodeSnippet extracts a random code snippet from the repository
func extractCodeSnippet(repoPath string, languages []LanguageStats) (string, string, error) {
	if len(languages) == 0 {
		return "", "", fmt.Errorf("no programming languages found")
	}

	// Get the primary language (highest percentage)
	primaryLang := languages[0].Name
	
	// Define file extensions for each language
	langExtensions := map[string][]string{
		"Go":           {".go"},
		"Python":       {".py"},
		"JavaScript":   {".js"},
		"TypeScript":   {".ts"},
		"Java":         {".java"},
		"C":            {".c", ".h"},
		"C++":          {".cpp", ".cc", ".cxx", ".hpp"},
		"C/C++":        {".h"},
		"C#":           {".cs"},
		"Ruby":         {".rb"},
		"PHP":          {".php"},
		"Swift":        {".swift"},
		"Kotlin":       {".kt"},
		"Rust":         {".rs"},
		"Shell":        {".sh", ".bash"},
		"Perl":         {".pl", ".pm"},
		"Raku":         {".raku", ".rakumod", ".p6", ".pm6"},
		"Haskell":      {".hs"},
		"Lua":          {".lua"},
		"HTML":         {".html", ".htm"},
		"CSS":          {".css"},
		"SQL":          {".sql"},
		"Make":         {"Makefile", "makefile", "GNUmakefile"},
		"HCL":          {".tf", ".tfvars", ".hcl"},
		"AWK":          {".awk", ".cgi"},  // .cgi files can be AWK scripts
	}

	// Get file extensions for the primary language
	extensions, ok := langExtensions[primaryLang]
	if !ok {
		// Try other languages if primary doesn't have extensions defined
		for _, lang := range languages {
			if exts, exists := langExtensions[lang.Name]; exists {
				extensions = exts
				primaryLang = lang.Name
				break
			}
		}
		if len(extensions) == 0 {
			return "", "", fmt.Errorf("no known file extensions for languages")
		}
	}

	// Find all files matching the extensions
	var codeFiles []string
	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
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
			   name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip files that are too large
		if info.Size() > 1*1024*1024 { // 1MB
			return nil
		}

		// Check if file matches extensions
		basename := filepath.Base(path)
		ext := filepath.Ext(path)
		
		matched := false
		for _, validExt := range extensions {
			if validExt == basename || (strings.HasPrefix(validExt, ".") && ext == validExt) {
				matched = true
				break
			}
		}
		
		// For executable files, also check shebang if primary language is AWK and file has .cgi extension
		if !matched && primaryLang == "AWK" && ext == ".cgi" && info.Mode()&0111 != 0 {
			if file, err := os.Open(path); err == nil {
				scanner := bufio.NewScanner(file)
				if scanner.Scan() {
					firstLine := scanner.Text()
					if strings.Contains(firstLine, "awk") || strings.Contains(firstLine, "gawk") {
						matched = true
					}
				}
				file.Close()
			}
		}
		
		if matched {
			// Skip test files and generated files
			if !strings.Contains(basename, "_test") && 
			   !strings.Contains(basename, ".test.") &&
			   !strings.Contains(basename, ".min.") &&
			   !strings.Contains(path, "/test/") &&
			   !strings.Contains(path, "/tests/") {
				codeFiles = append(codeFiles, path)
			}
		}

		return nil
	})

	if err != nil {
		return "", "", err
	}

	if len(codeFiles) == 0 {
		return "", "", fmt.Errorf("no code files found")
	}

	// Select a random file
	selectedFile := codeFiles[rand.Intn(len(codeFiles))]
	
	// Read the file and extract a snippet (~10 lines but complete functions)
	snippet, err := extractSnippetFromFile(selectedFile, 10, 15)
	if err != nil {
		return "", "", err
	}

	// Get relative path for display
	relPath, _ := filepath.Rel(repoPath, selectedFile)
	
	return snippet, fmt.Sprintf("%s from `%s`", primaryLang, relPath), nil
}

// extractSnippetFromFile extracts a code snippet from a file
func extractSnippetFromFile(filePath string, minLines, maxLines int) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Read all lines
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	totalLines := len(lines)
	if totalLines == 0 {
		return "", fmt.Errorf("file is empty")
	}

	// Try to find the smallest complete function
	bestFunction := findSmallestCompleteFunction(lines)
	if bestFunction != "" {
		return stripComments(bestFunction), nil
	}

	// If no complete function found, try to find a complete function/method
	functionStart, functionEnd := findCompleteFunctionOrMethod(lines, minLines, maxLines*2) // Allow larger functions
	if functionStart >= 0 && functionEnd >= 0 {
		snippet := strings.Join(lines[functionStart:functionEnd+1], "\n")
		return stripComments(snippet), nil
	}

	// Fallback to finding an interesting start with at least minLines
	interestingStart := findInterestingStart(lines, minLines)
	if interestingStart >= 0 {
		endLine := interestingStart + minLines
		if endLine > totalLines {
			endLine = totalLines
		}
		snippet := strings.Join(lines[interestingStart:endLine], "\n")
		return stripComments(snippet), nil
	}

	// Last resort: return first minLines (skip imports if possible)
	skipLines := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "import") && 
		   !strings.HasPrefix(trimmed, "package") && !strings.HasPrefix(trimmed, "using") &&
		   !strings.HasPrefix(trimmed, "#include") && !strings.HasPrefix(trimmed, "from") {
			skipLines = i
			break
		}
	}

	endLine := skipLines + minLines
	if endLine > totalLines {
		endLine = totalLines
	}

	snippet := strings.Join(lines[skipLines:endLine], "\n")
	return stripComments(snippet), nil
}

// findSmallestCompleteFunction finds the smallest complete function in the file
func findSmallestCompleteFunction(lines []string) string {
	type functionInfo struct {
		start int
		end   int
		size  int
	}
	
	var functions []functionInfo
	
	// Keywords that typically start functions/methods
	functionKeywords := []string{
		"func ", "function ", "def ", "public ", "private ", "protected ",
		"static ", "async ", "procedure ", "sub ", "method ",
	}
	
	// Find all complete functions
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		
		// Check if this line starts a function
		isFunction := false
		for _, keyword := range functionKeywords {
			if strings.Contains(line, keyword) && !strings.HasPrefix(line, "//") && !strings.HasPrefix(line, "#") {
				isFunction = true
				break
			}
		}
		
		if !isFunction {
			continue
		}
		
		// Try to find the end of this function
		functionEnd := findFunctionEnd(lines, i)
		if functionEnd > i {
			size := functionEnd - i + 1
			// Only consider functions between 5 and 50 lines
			if size >= 5 && size <= 50 {
				functions = append(functions, functionInfo{
					start: i,
					end:   functionEnd,
					size:  size,
				})
			}
		}
	}
	
	// Find the smallest function
	if len(functions) > 0 {
		smallest := functions[0]
		for _, f := range functions[1:] {
			if f.size < smallest.size {
				smallest = f
			}
		}
		return strings.Join(lines[smallest.start:smallest.end+1], "\n")
	}
	
	return ""
}

// findFunctionEnd finds the end of a function starting at the given line
func findFunctionEnd(lines []string, start int) int {
	if start >= len(lines) {
		return -1
	}
	
	// For brace-based languages
	braceCount := 0
	inFunction := false
	
	// For Python - track initial indentation
	isPython := strings.Contains(lines[start], "def ") || strings.Contains(lines[start], "class ")
	var initialIndent int
	if isPython && start < len(lines)-1 {
		// Get indentation of first line after def
		for i := start + 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) != "" {
				initialIndent = len(lines[i]) - len(strings.TrimLeft(lines[i], " \t"))
				break
			}
		}
	}
	
	for i := start; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		
		// Handle Python indentation
		if isPython && i > start {
			if trimmed == "" {
				continue
			}
			currentIndent := len(line) - len(strings.TrimLeft(line, " \t"))
			if currentIndent < initialIndent {
				return i - 1
			}
		}
		
		// Handle brace-based languages
		for _, ch := range line {
			if ch == '{' {
				braceCount++
				inFunction = true
			} else if ch == '}' {
				braceCount--
				if braceCount == 0 && inFunction {
					return i
				}
			}
		}
	}
	
	// If we're in Python and reached the end, return the last line
	if isPython {
		return len(lines) - 1
	}
	
	return -1
}

// findCompleteFunctionOrMethod finds a complete function or method within size constraints
func findCompleteFunctionOrMethod(lines []string, minLines, maxLines int) (int, int) {
	// Keywords that typically start functions/methods
	functionKeywords := []string{
		"func ", "function ", "def ", "public ", "private ", "protected ",
		"static ", "async ", "procedure ", "sub ", "method ",
	}
	
	// Try to find a function that fits within our size constraints
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		
		// Check if this line starts a function
		isFunction := false
		for _, keyword := range functionKeywords {
			if strings.Contains(line, keyword) && !strings.HasPrefix(line, "//") && !strings.HasPrefix(line, "#") {
				isFunction = true
				break
			}
		}
		
		if !isFunction {
			continue
		}
		
		// Try to find the end of this function
		functionEnd := findFunctionEnd(lines, i)
		if functionEnd > i {
			functionLength := functionEnd - i + 1
			if functionLength >= minLines && functionLength <= maxLines {
				return i, functionEnd
			}
		}
	}
	
	return -1, -1
}

// findInterestingStart tries to find a good starting point for the snippet
func findInterestingStart(lines []string, snippetSize int) int {
	// Look for function/class definitions
	keywords := []string{
		"func ", "function ", "def ", "class ", "public class",
		"interface ", "struct ", "type ", "const ", "var ",
		"procedure ", "sub ", "method ",
	}

	for i := 0; i < len(lines)-snippetSize; i++ {
		line := strings.TrimSpace(lines[i])
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") ||
		   strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*") {
			continue
		}

		// Check for interesting keywords
		for _, keyword := range keywords {
			if strings.Contains(line, keyword) {
				// Found something interesting, start here
				return i
			}
		}
	}

	// No interesting start found
	return -1
}

// stripComments removes comment lines from code snippets to make them more concise
func stripComments(code string) string {
	lines := strings.Split(code, "\n")
	var result []string
	inMultilineComment := false
	
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		
		// Handle multi-line comments for C-style languages
		if strings.Contains(line, "/*") {
			inMultilineComment = true
			// If comment ends on same line, process the rest
			if strings.Contains(line, "*/") {
				inMultilineComment = false
				// Skip this line entirely if it's just a comment
				if strings.TrimSpace(strings.Split(line, "/*")[0]) == "" {
					continue
				}
			} else {
				continue
			}
		}
		
		if inMultilineComment {
			if strings.Contains(line, "*/") {
				inMultilineComment = false
			}
			continue
		}
		
		// Skip single-line comments
		if trimmed == "" {
			// Keep empty lines for readability
			result = append(result, line)
		} else if strings.HasPrefix(trimmed, "//") || 
			strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "#include") && !strings.HasPrefix(trimmed, "#define") ||
			strings.HasPrefix(trimmed, "<!--") ||
			strings.HasPrefix(trimmed, "*") && len(trimmed) > 1 && trimmed[1] == ' ' {
			// Skip comment lines
			continue
		} else if strings.HasPrefix(trimmed, "\"\"\"") || strings.HasPrefix(trimmed, "'''") {
			// Skip Python docstrings (simplified - doesn't handle all cases)
			continue
		} else {
			// Keep the line but remove inline comments for some languages
			// This is a simple approach - doesn't handle all edge cases
			if idx := strings.Index(line, " //"); idx > 0 {
				// Check if it's not inside a string (very basic check)
				beforeComment := line[:idx]
				if strings.Count(beforeComment, "\"")%2 == 0 && strings.Count(beforeComment, "'")%2 == 0 {
					line = strings.TrimRight(line[:idx], " \t")
				}
			}
			result = append(result, line)
		}
	}
	
	// Remove leading and trailing empty lines
	for len(result) > 0 && strings.TrimSpace(result[0]) == "" {
		result = result[1:]
	}
	for len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
		result = result[:len(result)-1]
	}
	
	// Remove unnecessary indentation
	result = removeCommonIndentation(result)
	
	return strings.Join(result, "\n")
}

// removeCommonIndentation removes common leading whitespace from all lines
func removeCommonIndentation(lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	
	// Find the common prefix of whitespace
	var commonPrefix string
	firstNonEmpty := -1
	
	// Find first non-empty line to use as reference
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			firstNonEmpty = i
			break
		}
	}
	
	if firstNonEmpty == -1 {
		return lines
	}
	
	// Get the whitespace prefix of the first non-empty line
	firstLine := lines[firstNonEmpty]
	for i, ch := range firstLine {
		if ch != ' ' && ch != '\t' {
			commonPrefix = firstLine[:i]
			break
		}
	}
	
	// If the first line has no indentation, return as-is
	if commonPrefix == "" {
		return lines
	}
	
	// Find the actual common prefix among all non-empty lines
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		
		// Reduce commonPrefix to what this line shares
		for i := 0; i < len(commonPrefix); i++ {
			if i >= len(line) || line[i] != commonPrefix[i] {
				commonPrefix = commonPrefix[:i]
				break
			}
		}
		
		if commonPrefix == "" {
			break
		}
	}
	
	// If no common prefix found, return as-is
	if commonPrefix == "" {
		return lines
	}
	
	// Remove common prefix from all lines
	result := make([]string, len(lines))
	prefixLen := len(commonPrefix)
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			result[i] = ""
		} else if strings.HasPrefix(line, commonPrefix) {
			result[i] = line[prefixLen:]
		} else {
			result[i] = line
		}
	}
	
	return result
}