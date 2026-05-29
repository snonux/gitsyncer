package showcase

import "strings"

func extractUsefulSummary(text string, maxParagraphs int) string {
	if maxParagraphs <= 0 {
		maxParagraphs = 1
	}

	parts := splitSummaryParagraphs(text)
	useful := make([]string, 0, maxParagraphs)

	for _, part := range parts {
		part = normalizeSummaryParagraph(part)
		if part == "" {
			continue
		}

		useful = append(useful, part)
		if len(useful) >= maxParagraphs {
			break
		}
	}

	return strings.Join(useful, "\n\n")
}

func normalizeSummaryParagraph(paragraph string) string {
	rawParagraph := strings.TrimSpace(paragraph)
	switch {
	case isHeadingOnlyParagraph(rawParagraph):
		return ""
	case isImageOnlyParagraph(rawParagraph):
		return ""
	case isHTMLOnlyParagraph(rawParagraph):
		return ""
	case isTOCParagraph(rawParagraph):
		return ""
	case isListOnlyParagraph(rawParagraph):
		return ""
	case isBadgeParagraph(rawParagraph):
		return ""
	}

	paragraph = sanitizeSummaryForGemtext(paragraph)
	if paragraph == "" {
		return ""
	}

	if normalized, ok := normalizeManpageParagraph(paragraph); ok {
		paragraph = normalized
	}

	if isLabelOnlyParagraph(paragraph) {
		return ""
	}

	return paragraph
}

func splitSummaryParagraphs(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	rawParts := strings.Split(text, "\n\n")
	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parts = append(parts, part)
	}

	return parts
}

func sanitizeSummaryForGemtext(summary string) string {
	summary = strings.ReplaceAll(summary, "\r\n", "\n")
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}

	lines := strings.Split(summary, "\n")
	cleaned := make([]string, 0, len(lines))
	inCodeFence := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if inCodeFence {
				continue
			}
			cleaned = append(cleaned, "")
			continue
		}

		if strings.HasPrefix(trimmed, "```") {
			inCodeFence = !inCodeFence
			continue
		}
		if inCodeFence {
			continue
		}

		if isHTMLOnlyLine(trimmed) || isMarkdownImageLine(trimmed) {
			continue
		}

		if isSetextUnderline(trimmed) && len(cleaned) > 0 && strings.TrimSpace(cleaned[len(cleaned)-1]) != "" {
			continue
		}

		if heading, ok := trimMarkdownHeading(trimmed); ok {
			if heading != "" {
				cleaned = append(cleaned, heading)
			}
			continue
		}

		cleaned = append(cleaned, strings.TrimRight(line, " \t"))
	}

	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func isHeadingOnlyParagraph(paragraph string) bool {
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(paragraph, "\r\n", "\n")), "\n")
	if len(lines) == 1 {
		_, ok := trimMarkdownHeading(strings.TrimSpace(lines[0]))
		return ok
	}
	if len(lines) == 2 {
		return strings.TrimSpace(lines[0]) != "" && isSetextUnderline(strings.TrimSpace(lines[1]))
	}
	return false
}

func isImageOnlyParagraph(paragraph string) bool {
	trimmed := strings.TrimSpace(paragraph)
	if trimmed == "" || strings.Contains(trimmed, "\n") {
		return false
	}
	return strings.HasPrefix(trimmed, "<img") || strings.HasPrefix(trimmed, "![")
}

func isHTMLOnlyParagraph(paragraph string) bool {
	lines := strings.Split(strings.TrimSpace(paragraph), "\n")
	if len(lines) == 0 {
		return false
	}

	seen := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !isHTMLOnlyLine(trimmed) {
			return false
		}
		seen = true
	}

	return seen
}

func isHTMLOnlyLine(line string) bool {
	return strings.HasPrefix(line, "<") && strings.HasSuffix(line, ">")
}

func isMarkdownImageLine(line string) bool {
	return strings.HasPrefix(line, "![") && strings.Contains(line, "](")
}

func isTOCParagraph(paragraph string) bool {
	lines := strings.Split(strings.TrimSpace(paragraph), "\n")
	if len(lines) == 0 {
		return false
	}

	first := strings.TrimSpace(lines[0])
	if !strings.EqualFold(first, "toc:") && !strings.EqualFold(first, "table of contents:") {
		return false
	}

	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !isOrderedListLine(trimmed) {
			return false
		}
	}

	return true
}

func isListOnlyParagraph(paragraph string) bool {
	lines := strings.Split(strings.TrimSpace(paragraph), "\n")
	if len(lines) == 0 {
		return false
	}

	seen := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !isListLine(trimmed) {
			return false
		}
		seen = true
	}

	return seen
}

func isListLine(line string) bool {
	return strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "- ") || isOrderedListLine(line)
}

func isOrderedListLine(line string) bool {
	if line == "" || line[0] < '0' || line[0] > '9' {
		return false
	}

	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == 0 || i >= len(line) {
		return false
	}
	if (line[i] != '.' && line[i] != ')') || i+1 >= len(line) || line[i+1] != ' ' {
		return false
	}

	return true
}

func isLabelOnlyParagraph(paragraph string) bool {
	lines := strings.Split(strings.TrimSpace(paragraph), "\n")
	if len(lines) != 1 {
		return false
	}

	line := strings.TrimSpace(lines[0])
	if line == "" {
		return false
	}
	if strings.HasSuffix(line, ":") && len(strings.Fields(line)) <= 5 {
		return true
	}

	return line == strings.ToUpper(line) && len(strings.Fields(line)) <= 4
}

func isBadgeParagraph(paragraph string) bool {
	lines := strings.Split(strings.TrimSpace(paragraph), "\n")
	if len(lines) != 1 {
		return false
	}

	line := strings.TrimSpace(lines[0])
	if line == "" {
		return false
	}

	markerCount := strings.Count(line, "](") + strings.Count(line, "![")
	return markerCount >= 2
}

func normalizeManpageParagraph(paragraph string) (string, bool) {
	lines := strings.Split(strings.TrimSpace(paragraph), "\n")
	if len(lines) < 2 {
		return "", false
	}
	if strings.TrimSpace(lines[0]) != "NAME" {
		return "", false
	}

	body := make([]string, 0, len(lines)-1)
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		body = append(body, trimmed)
	}
	if len(body) == 0 {
		return "", false
	}

	return strings.Join(body, " "), true
}

func trimMarkdownHeading(line string) (string, bool) {
	if line == "" || !strings.HasPrefix(line, "#") {
		return "", false
	}

	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 {
		return "", false
	}
	if level < len(line) && line[level] != ' ' && line[level] != '\t' {
		return "", false
	}

	heading := strings.TrimSpace(line[level:])
	heading = strings.TrimSpace(strings.TrimRight(heading, "#"))
	return heading, true
}

func isSetextUnderline(line string) bool {
	if len(line) < 3 {
		return false
	}
	return strings.Trim(line, "=") == "" || strings.Trim(line, "-") == ""
}
