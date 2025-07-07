package showcase

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// extractImagesFromRepo extracts up to 2 images from README.md and copies them to showcase directory
func extractImagesFromRepo(repoPath, repoName, showcaseDir string) ([]string, error) {
	// Look for README files
	readmeFiles := []string{"README.md", "readme.md", "Readme.md", "README.MD"}
	var readmePath string
	
	for _, filename := range readmeFiles {
		path := filepath.Join(repoPath, filename)
		if _, err := os.Stat(path); err == nil {
			readmePath = path
			break
		}
	}
	
	if readmePath == "" {
		return nil, nil // No README found, not an error
	}
	
	// Read README content
	content, err := os.ReadFile(readmePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read README: %w", err)
	}
	
	fmt.Printf("Found README at: %s\n", readmePath)
	
	// Extract image references
	images := extractImageReferences(string(content))
	fmt.Printf("Found %d images in README\n", len(images))
	for i, img := range images {
		fmt.Printf("  Image %d: %s\n", i+1, img)
	}
	
	if len(images) == 0 {
		return nil, nil
	}
	
	// Limit to first and last image (max 2)
	var selectedImages []string
	if len(images) == 1 {
		selectedImages = images
	} else {
		selectedImages = []string{images[0], images[len(images)-1]}
	}
	
	// Create showcase subdirectory for this repo
	repoShowcaseDir := filepath.Join(showcaseDir, "showcase", repoName)
	if err := os.MkdirAll(repoShowcaseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create showcase directory: %w", err)
	}
	
	// Copy images and collect relative paths
	var copiedImages []string
	for i, imgPath := range selectedImages {
		var destFilename string
		var err error
		
		if strings.HasPrefix(imgPath, "http://") || strings.HasPrefix(imgPath, "https://") {
			// Handle URL - download the image
			// Extract extension from URL, handling query parameters
			urlParts := strings.Split(imgPath, "?")
			basePath := urlParts[0]
			ext := filepath.Ext(basePath)
			if ext == "" || len(ext) > 5 { // Likely not a real extension
				ext = ".png" // Default extension
			}
			destFilename = fmt.Sprintf("image-%d%s", i+1, ext)
			destPath := filepath.Join(repoShowcaseDir, destFilename)
			
			if err = downloadImage(imgPath, destPath); err != nil {
				fmt.Printf("Warning: Failed to download image %s: %v\n", imgPath, err)
				continue
			}
		} else {
			// Handle local file
			srcPath := imgPath
			if !filepath.IsAbs(imgPath) {
				srcPath = filepath.Join(repoPath, imgPath)
			}
			
			// Check if image exists
			if _, err := os.Stat(srcPath); err != nil {
				fmt.Printf("Warning: Image not found: %s\n", srcPath)
				continue
			}
			
			// Generate destination filename
			ext := filepath.Ext(srcPath)
			destFilename = fmt.Sprintf("image-%d%s", i+1, ext)
			destPath := filepath.Join(repoShowcaseDir, destFilename)
			
			// Copy image
			if err := copyFile(srcPath, destPath); err != nil {
				fmt.Printf("Warning: Failed to copy image %s: %v\n", srcPath, err)
				continue
			}
		}
		
		// Store relative path from showcase directory
		relativePath := filepath.Join("showcase", repoName, destFilename)
		copiedImages = append(copiedImages, relativePath)
		fmt.Printf("Copied/Downloaded image: %s -> %s\n", imgPath, relativePath)
	}
	
	return copiedImages, nil
}

// extractImageReferences extracts image references from markdown content
func extractImageReferences(content string) []string {
	var images []string
	seen := make(map[string]bool)
	
	// Regex patterns for markdown images
	patterns := []string{
		`!\[([^\]]*)\]\(([^)]+)\)`,                    // ![alt](url)
		`<img[^>]+src=["']([^"']+)["'][^>]*>`,        // <img src="url">
		`!\[([^\]]*)\]\[([^\]]+)\]`,                   // ![alt][ref]
		`\[([^\]]+)\]:\s*(.+?)(?:\s+"[^"]+")?\s*$`,   // [ref]: url "title"
	}
	
	fmt.Printf("DEBUG: Content length: %d bytes\n", len(content))
	
	// Extract from markdown image syntax
	for i, pattern := range patterns[:2] { // First two patterns have URLs in different positions
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(content, -1)
		fmt.Printf("DEBUG: Pattern %d (%s) found %d matches\n", i, pattern, len(matches))
		
		for _, match := range matches {
			var url string
			if pattern == patterns[0] {
				url = match[2] // For ![alt](url)
			} else {
				url = match[1] // For <img src="url">
			}
			
			// Clean and validate URL
			url = strings.TrimSpace(url)
			fmt.Printf("DEBUG: Found potential image URL: %s\n", url)
			
			if isImageFile(url) {
				fmt.Printf("DEBUG: URL is image file\n")
				if !seen[url] {
					// Handle different types of URLs
					if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
						// Local file
						fmt.Printf("DEBUG: Adding local image: %s\n", url)
						images = append(images, url)
						seen[url] = true
					} else if isGitHostedImage(url) {
						// GitHub/Codeberg hosted images - we can download these
						fmt.Printf("DEBUG: Found git-hosted image: %s\n", url)
						images = append(images, url)
						seen[url] = true
					} else {
						fmt.Printf("DEBUG: Skipping external URL: %s\n", url)
					}
				}
			} else {
				fmt.Printf("DEBUG: Not recognized as image file: %s\n", url)
			}
		}
	}
	
	// Handle reference-style images
	refPattern := regexp.MustCompile(patterns[3])
	refMatches := refPattern.FindAllStringSubmatch(content, -1)
	refs := make(map[string]string)
	for _, match := range refMatches {
		refs[match[1]] = strings.TrimSpace(match[2])
	}
	
	// Find reference-style image uses
	refUsePattern := regexp.MustCompile(patterns[2])
	refUseMatches := refUsePattern.FindAllStringSubmatch(content, -1)
	for _, match := range refUseMatches {
		ref := match[2]
		if url, ok := refs[ref]; ok && isImageFile(url) && !seen[url] {
			if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
				images = append(images, url)
				seen[url] = true
			}
		}
	}
	
	return images
}

// isImageFile checks if a URL points to an image file
func isImageFile(url string) bool {
	url = strings.ToLower(url)
	extensions := []string{".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".bmp", ".ico"}
	for _, ext := range extensions {
		if strings.HasSuffix(url, ext) {
			return true
		}
	}
	return false
}

// isGitHostedImage checks if URL is from GitHub/Codeberg
func isGitHostedImage(url string) bool {
	return strings.Contains(url, "github.com") || 
		strings.Contains(url, "githubusercontent.com") ||
		strings.Contains(url, "codeberg.org") ||
		strings.Contains(url, "codeberg.page")
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	
	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()
	
	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}
	
	return destFile.Sync()
}

// downloadImage downloads an image from URL to dst
func downloadImage(url, dst string) error {
	// Use curl to download the image
	cmd := exec.Command("curl", "-L", "-o", dst, url)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("curl failed: %v, output: %s", err, string(output))
	}
	
	// Verify the file was created
	if _, err := os.Stat(dst); err != nil {
		return fmt.Errorf("downloaded file not found: %v", err)
	}
	
	return nil
}