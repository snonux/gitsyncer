package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"codeberg.org/snonux/gitsyncer/internal/config"
	"codeberg.org/snonux/gitsyncer/internal/release"
)

// isVersionTag checks if a tag name is a version tag
// Supports formats: vX.Y.Z, vX.Y, vX, X.Y.Z, X.Y, X
func isVersionTag(tag string) bool {
	// Pattern matches version tags with optional 'v' prefix
	pattern := `^v?\d+(\.\d+)?(\.\d+)?$`
	matched, _ := regexp.MatchString(pattern, tag)
	return matched
}

// HandleCheckReleases checks for version tags without releases and creates them with confirmation
func HandleCheckReleases(cfg *config.Config, flags *Flags) int {
	// Get all repositories from work directory
	entries, err := os.ReadDir(flags.WorkDir)
	if err != nil {
		fmt.Printf("Error reading work directory %s: %v\n", flags.WorkDir, err)
		return 1
	}
	
	var repositories []string
	for _, entry := range entries {
		if entry.IsDir() {
			// Check if it's a git repository
			gitPath := filepath.Join(flags.WorkDir, entry.Name(), ".git")
			if _, err := os.Stat(gitPath); err == nil {
				repositories = append(repositories, entry.Name())
			}
		}
	}
	
	if len(repositories) == 0 {
		fmt.Println("No repositories found in work directory")
		return 1
	}
	
	fmt.Printf("Found %d repositories in work directory\n", len(repositories))
	return HandleCheckReleasesForRepos(cfg, flags, repositories)
}

// HandleCheckReleasesForRepo checks releases for a specific repository
func HandleCheckReleasesForRepo(cfg *config.Config, flags *Flags, repoName string) int {
	// Check only the specified repository
	return HandleCheckReleasesForRepos(cfg, flags, []string{repoName})
}

// HandleCheckReleasesForRepos checks for version tags without releases and creates them with confirmation
func HandleCheckReleasesForRepos(cfg *config.Config, flags *Flags, repositories []string) int {
	releaseManager := release.NewManager(flags.WorkDir)
	releaseManager.SetAITool(flags.AITool)
	
	// Load persistent AI release notes cache
	cacheFile := filepath.Join(flags.WorkDir, ".gitsyncer-ai-release-notes-cache.json")
	aiReleaseNotesCache := loadAIReleaseNotesCache(cacheFile)
	initialCacheSize := len(aiReleaseNotesCache)
	
	// Track failed AI generations
	failedAIGenerations := []string{}
	
	// Print summary at the end
	defer func() {
		if len(aiReleaseNotesCache) > initialCacheSize {
			fmt.Printf("\nAI release notes cache updated: %d new entries added (total: %d entries)\n", 
				len(aiReleaseNotesCache)-initialCacheSize, len(aiReleaseNotesCache))
			fmt.Printf("Cache file: %s\n", cacheFile)
		}
		
		if len(failedAIGenerations) > 0 {
			fmt.Printf("\n⚠️  AI release notes generation failed for %d releases:\n", len(failedAIGenerations))
			for _, failed := range failedAIGenerations {
				fmt.Printf("  - %s\n", failed)
			}
			fmt.Println("\nThese releases were skipped. Their cache entries were cleared.")
			fmt.Println("Run again to retry generation for these releases.")
		}
	}()
	
	// Set tokens from config with fallback to environment variables and files
	githubOrg := cfg.FindGitHubOrg()
	if githubOrg != nil {
		fmt.Printf("Found GitHub org: %s\n", githubOrg.Name)
		
		// Try config token first, then fallback to env var and file
		token := githubOrg.GitHubToken
		if token == "" {
			// Try environment variable
			token = os.Getenv("GITHUB_TOKEN")
			if token == "" {
				// Try token file
				home, err := os.UserHomeDir()
				if err == nil {
					tokenFile := filepath.Join(home, ".gitsyncer_github_token")
					data, err := os.ReadFile(tokenFile)
					if err == nil {
						token = strings.TrimSpace(string(data))
					}
				}
			}
		}
		
		if token != "" {
			releaseManager.SetGitHubToken(token)
		} else {
			fmt.Println("WARNING: No GitHub token found - cannot create GitHub releases")
		}
	} else {
		fmt.Println("No GitHub organization found in config")
	}
	
	codebergOrg := cfg.FindCodebergOrg()
	if codebergOrg != nil {
		fmt.Printf("Found Codeberg org: %s\n", codebergOrg.Name)
		
		// Try config token first, then fallback to env var and file
		token := codebergOrg.CodebergToken
		if token == "" {
			// Try environment variable
			token = os.Getenv("CODEBERG_TOKEN")
			if token == "" {
				// Try token file
				home, err := os.UserHomeDir()
				if err == nil {
					tokenFile := filepath.Join(home, ".gitsyncer_codeberg_token")
					data, err := os.ReadFile(tokenFile)
					if err == nil {
						token = strings.TrimSpace(string(data))
					}
				}
			}
		}
		
		if token != "" {
			releaseManager.SetCodebergToken(token)
			fmt.Printf("  Codeberg token loaded (length: %d)\n", len(token))
		} else {
			fmt.Println("WARNING: No Codeberg token found - cannot create Codeberg releases")
		}
	} else {
		fmt.Println("No Codeberg organization found in config")
	}
	
        // Process the specified repositories
        for _, repoName := range repositories {
            fmt.Printf("\nChecking releases for repository: %s\n", repoName)
		
		// Check if the repository is cloned locally
		repoPath := filepath.Join(flags.WorkDir, repoName)
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			fmt.Printf("  Repository not found locally at %s, skipping...\n", repoPath)
			continue
		}
		
		// Get local tags
		localTags, err := releaseManager.GetLocalTags(repoPath)
		if err != nil {
			fmt.Printf("  Error getting local tags: %v\n", err)
			continue
		}
		
		if len(localTags) == 0 {
			fmt.Println("  No version tags found")
			continue
		}
		
            fmt.Printf("  Found %d version tags: %s\n", len(localTags), strings.Join(localTags, ", "))
            // Log configured skip rules for this repo, if any
            if cfg.SkipReleases != nil {
                if skipTags, ok := cfg.SkipReleases[repoName]; ok && len(skipTags) > 0 {
                    fmt.Printf("  Config skip_releases for %s: %s\n", repoName, strings.Join(skipTags, ", "))
                }
            }
		
		// Check GitHub releases if GitHub is configured
		var missingGitHub []string
		githubOrg := cfg.FindGitHubOrg()
            if githubOrg != nil && githubOrg.Name != "" {
                githubReleases, err := releaseManager.GetGitHubReleases(githubOrg.Name, repoName)
                if err != nil {
                    fmt.Printf("  Error checking GitHub releases: %v\n", err)
                } else {
                    missingGitHub = releaseManager.FindMissingReleases(localTags, githubReleases)
                    // Filter out tags that should be skipped per config
                    if len(missingGitHub) > 0 {
                        var filtered []string
                        var skipped []string
                        for _, t := range missingGitHub {
                            if cfg.ShouldSkipRelease(repoName, t) {
                                skipped = append(skipped, t)
                            } else {
                                filtered = append(filtered, t)
                            }
                        }
                        if len(skipped) > 0 {
                            fmt.Printf("  Skipping GitHub releases per config for tags: %s\n", strings.Join(skipped, ", "))
                        }
                        missingGitHub = filtered
                        if len(missingGitHub) > 0 {
                            fmt.Printf("  Missing GitHub releases: %s\n", strings.Join(missingGitHub, ", "))
                        }
                    }
                }
            }
		
		// Check Codeberg releases if Codeberg is configured
		var missingCodeberg []string
		codebergOrg := cfg.FindCodebergOrg()
            if codebergOrg != nil && codebergOrg.Name != "" {
                codebergReleases, err := releaseManager.GetCodebergReleases(codebergOrg.Name, repoName)
                if err != nil {
                    fmt.Printf("  Error checking Codeberg releases: %v\n", err)
                } else {
                    missingCodeberg = releaseManager.FindMissingReleases(localTags, codebergReleases)
                    // Filter out tags that should be skipped per config
                    if len(missingCodeberg) > 0 {
                        var filtered []string
                        var skipped []string
                        for _, t := range missingCodeberg {
                            if cfg.ShouldSkipRelease(repoName, t) {
                                skipped = append(skipped, t)
                            } else {
                                filtered = append(filtered, t)
                            }
                        }
                        if len(skipped) > 0 {
                            fmt.Printf("  Skipping Codeberg releases per config for tags: %s\n", strings.Join(skipped, ", "))
                        }
                        missingCodeberg = filtered
                        if len(missingCodeberg) > 0 {
                            fmt.Printf("  Missing Codeberg releases: %s\n", strings.Join(missingCodeberg, ", "))
                        }
                    }
                }
            }
		
		// Create missing releases with confirmation
            if len(missingGitHub) > 0 && githubOrg != nil {
                for _, tag := range missingGitHub {
                    // Skip if configured to skip this repo/tag
                    if cfg.ShouldSkipRelease(repoName, tag) {
                        fmt.Printf("  Skipping GitHub release for %s:%s per config skip_releases\n", repoName, tag)
                        continue
                    }
				// Get commits for this tag
				commits, err := releaseManager.GetCommitsSinceTag(repoPath, "", tag)
				if err != nil {
					commits = []string{}
				}
				
				// Generate release notes
				var releaseNotes string
				if flags.AIReleaseNotes {
					// Check cache first (unless --force is used)
					cacheKey := fmt.Sprintf("%s:%s", repoName, tag)
					if cachedNotes, exists := aiReleaseNotesCache[cacheKey]; exists && !flags.Force {
						fmt.Printf("  Using cached AI release notes for %s\n", tag)
						releaseNotes = cachedNotes
					} else {
						if flags.Force && aiReleaseNotesCache[cacheKey] != "" {
							fmt.Printf("  Force regenerating AI release notes for %s (ignoring cache)\n", tag)
						} else {
							fmt.Printf("  Generating AI release notes for %s...\n", tag)
						}
						aiNotes, err := releaseManager.GenerateAIReleaseNotes(repoPath, repoName, tag, localTags, commits)
						if err != nil {
							fmt.Printf("  Warning: Failed to generate AI release notes: %v\n", err)
							fmt.Printf("  Falling back to standard release notes\n")
							releaseNotes = releaseManager.GenerateReleaseNotes(repoPath, tag, localTags)
							// Clear cache on failure and track
							delete(aiReleaseNotesCache, cacheKey)
							failedAIGenerations = append(failedAIGenerations, fmt.Sprintf("%s/%s:%s", githubOrg.Name, repoName, tag))
							// Save cache after clearing the failed entry
							saveAIReleaseNotesCache(cacheFile, aiReleaseNotesCache)
						} else {
							releaseNotes = aiNotes
							aiReleaseNotesCache[cacheKey] = aiNotes // Cache only on success
							// Save cache immediately after successful generation
							if err := saveAIReleaseNotesCache(cacheFile, aiReleaseNotesCache); err != nil {
								fmt.Printf("  Warning: Failed to save cache: %v\n", err)
							}
							fmt.Printf("  AI release notes generated successfully and cached\n")
						}
					}
				} else {
					releaseNotes = releaseManager.GenerateReleaseNotes(repoPath, tag, localTags)
				}
				
				// Print release notes to stdout
				fmt.Printf("\n%s\n", strings.Repeat("=", 70))
				fmt.Printf("Release Notes for %s/%s tag %s:\n", githubOrg.Name, repoName, tag)
				fmt.Printf("%s\n", strings.Repeat("-", 70))
				fmt.Println(releaseNotes)
				fmt.Printf("%s\n\n", strings.Repeat("=", 70))
				
				msg := fmt.Sprintf("Create GitHub release for %s/%s tag %s?", githubOrg.Name, repoName, tag)
				
				// Check if auto-create is enabled
				createRelease := false
				if flags.AutoCreateReleases {
					fmt.Printf("  Auto-creating GitHub release for %s/%s tag %s\n", githubOrg.Name, repoName, tag)
					createRelease = true
				} else {
					createRelease = release.PromptConfirmation(msg)
				}
				
				if createRelease {
					if err := releaseManager.CreateGitHubRelease(githubOrg.Name, repoName, tag, releaseNotes); err != nil {
						fmt.Printf("  Error creating GitHub release: %v\n", err)
					} else {
						fmt.Printf("  Created GitHub release for tag %s\n", tag)
					}
				}
			}
		}
		
            if len(missingCodeberg) > 0 && codebergOrg != nil {
                for _, tag := range missingCodeberg {
                    // Skip if configured to skip this repo/tag
                    if cfg.ShouldSkipRelease(repoName, tag) {
                        fmt.Printf("  Skipping Codeberg release for %s:%s per config skip_releases\n", repoName, tag)
                        continue
                    }
				// Get commits for this tag
				commits, err := releaseManager.GetCommitsSinceTag(repoPath, "", tag)
				if err != nil {
					commits = []string{}
				}
				
				// Generate release notes
				var releaseNotes string
				if flags.AIReleaseNotes {
					// Check cache first (unless --force is used)
					cacheKey := fmt.Sprintf("%s:%s", repoName, tag)
					if cachedNotes, exists := aiReleaseNotesCache[cacheKey]; exists && !flags.Force {
						fmt.Printf("  Using cached AI release notes for %s\n", tag)
						releaseNotes = cachedNotes
					} else {
						if flags.Force && aiReleaseNotesCache[cacheKey] != "" {
							fmt.Printf("  Force regenerating AI release notes for %s (ignoring cache)\n", tag)
						} else {
							fmt.Printf("  Generating AI release notes for %s...\n", tag)
						}
						aiNotes, err := releaseManager.GenerateAIReleaseNotes(repoPath, repoName, tag, localTags, commits)
						if err != nil {
							fmt.Printf("  Warning: Failed to generate AI release notes: %v\n", err)
							fmt.Printf("  Falling back to standard release notes\n")
							releaseNotes = releaseManager.GenerateReleaseNotes(repoPath, tag, localTags)
							// Clear cache on failure and track
							delete(aiReleaseNotesCache, cacheKey)
							failedAIGenerations = append(failedAIGenerations, fmt.Sprintf("%s/%s:%s", githubOrg.Name, repoName, tag))
							// Save cache after clearing the failed entry
							saveAIReleaseNotesCache(cacheFile, aiReleaseNotesCache)
						} else {
							releaseNotes = aiNotes
							aiReleaseNotesCache[cacheKey] = aiNotes // Cache only on success
							// Save cache immediately after successful generation
							if err := saveAIReleaseNotesCache(cacheFile, aiReleaseNotesCache); err != nil {
								fmt.Printf("  Warning: Failed to save cache: %v\n", err)
							}
							fmt.Printf("  AI release notes generated successfully and cached\n")
						}
					}
				} else {
					releaseNotes = releaseManager.GenerateReleaseNotes(repoPath, tag, localTags)
				}
				
				// Print release notes to stdout
				fmt.Printf("\n%s\n", strings.Repeat("=", 70))
				fmt.Printf("Release Notes for %s/%s tag %s:\n", codebergOrg.Name, repoName, tag)
				fmt.Printf("%s\n", strings.Repeat("-", 70))
				fmt.Println(releaseNotes)
				fmt.Printf("%s\n\n", strings.Repeat("=", 70))
				
				msg := fmt.Sprintf("Create Codeberg release for %s/%s tag %s?", codebergOrg.Name, repoName, tag)
				
				// Check if auto-create is enabled
				createRelease := false
				if flags.AutoCreateReleases {
					fmt.Printf("  Auto-creating Codeberg release for %s/%s tag %s\n", codebergOrg.Name, repoName, tag)
					createRelease = true
				} else {
					createRelease = release.PromptConfirmation(msg)
				}
				
				if createRelease {
					if err := releaseManager.CreateCodebergRelease(codebergOrg.Name, repoName, tag, releaseNotes); err != nil {
						fmt.Printf("  Error creating Codeberg release: %v\n", err)
					} else {
						fmt.Printf("  Created Codeberg release for tag %s\n", tag)
					}
				}
			}
		}
		
		// Update existing releases if requested
		if flags.UpdateReleases {
			// Update GitHub releases
			if githubOrg != nil && githubOrg.Name != "" {
				githubReleases, err := releaseManager.GetGitHubReleases(githubOrg.Name, repoName)
				if err == nil && len(githubReleases) > 0 {
					fmt.Printf("\n  Updating existing GitHub releases...\n")
					for _, tag := range githubReleases {
						// Check if this is a version tag
						if !isVersionTag(tag) {
							continue
						}
						
						// Get commits for this tag
						commits, err := releaseManager.GetCommitsSinceTag(repoPath, "", tag)
						if err != nil {
							commits = []string{}
						}
						
						// Generate AI release notes
						if flags.AIReleaseNotes {
							// Check cache first (unless --force is used)
							cacheKey := fmt.Sprintf("%s:%s", repoName, tag)
							var aiNotes string
							if cachedNotes, exists := aiReleaseNotesCache[cacheKey]; exists && !flags.Force {
								fmt.Printf("  Using cached AI release notes for existing release %s\n", tag)
								aiNotes = cachedNotes
							} else {
								if flags.Force && aiReleaseNotesCache[cacheKey] != "" {
									fmt.Printf("  Force regenerating AI release notes for existing release %s (ignoring cache)\n", tag)
								} else {
									fmt.Printf("  Generating AI release notes for existing release %s...\n", tag)
								}
								var err error
								aiNotes, err = releaseManager.GenerateAIReleaseNotes(repoPath, repoName, tag, localTags, commits)
								if err != nil {
									fmt.Printf("  Warning: Failed to generate AI release notes: %v\n", err)
									// Clear cache on failure and track
									delete(aiReleaseNotesCache, cacheKey)
									// Determine which org we're updating for the failure message
									orgName := ""
									if githubOrg != nil && githubOrg.Name != "" {
										orgName = githubOrg.Name
									} else if codebergOrg != nil && codebergOrg.Name != "" {
										orgName = codebergOrg.Name
									}
									failedAIGenerations = append(failedAIGenerations, fmt.Sprintf("%s/%s:%s", orgName, repoName, tag))
									// Save cache after clearing the failed entry
									saveAIReleaseNotesCache(cacheFile, aiReleaseNotesCache)
									continue
								}
								aiReleaseNotesCache[cacheKey] = aiNotes // Cache only on success
								// Save cache immediately after successful generation
								if err := saveAIReleaseNotesCache(cacheFile, aiReleaseNotesCache); err != nil {
									fmt.Printf("  Warning: Failed to save cache: %v\n", err)
								}
							}
							
							// Print release notes to stdout
							fmt.Printf("\n%s\n", strings.Repeat("=", 70))
							fmt.Printf("Updated Release Notes for %s/%s tag %s:\n", githubOrg.Name, repoName, tag)
							fmt.Printf("%s\n", strings.Repeat("-", 70))
							fmt.Println(aiNotes)
							fmt.Printf("%s\n\n", strings.Repeat("=", 70))
							
							msg := fmt.Sprintf("Update GitHub release for %s/%s tag %s?", githubOrg.Name, repoName, tag)
							
							updateRelease := false
							if flags.AutoCreateReleases {
								fmt.Printf("  Auto-updating GitHub release for %s/%s tag %s\n", githubOrg.Name, repoName, tag)
								updateRelease = true
							} else {
								updateRelease = release.PromptConfirmation(msg)
							}
							
							if updateRelease {
								if err := releaseManager.UpdateGitHubRelease(githubOrg.Name, repoName, tag, aiNotes); err != nil {
									fmt.Printf("  Error updating GitHub release: %v\n", err)
								} else {
									fmt.Printf("  Updated GitHub release for tag %s\n", tag)
								}
							}
						}
					}
				}
			}
			
			// Update Codeberg releases
			if codebergOrg != nil && codebergOrg.Name != "" {
				codebergReleases, err := releaseManager.GetCodebergReleases(codebergOrg.Name, repoName)
				if err == nil && len(codebergReleases) > 0 {
					fmt.Printf("\n  Updating existing Codeberg releases...\n")
					for _, tag := range codebergReleases {
						// Check if this is a version tag
						if !isVersionTag(tag) {
							continue
						}
						
						// Get commits for this tag
						commits, err := releaseManager.GetCommitsSinceTag(repoPath, "", tag)
						if err != nil {
							commits = []string{}
						}
						
						// Generate AI release notes
						if flags.AIReleaseNotes {
							// Check cache first (unless --force is used)
							cacheKey := fmt.Sprintf("%s:%s", repoName, tag)
							var aiNotes string
							if cachedNotes, exists := aiReleaseNotesCache[cacheKey]; exists && !flags.Force {
								fmt.Printf("  Using cached AI release notes for existing release %s\n", tag)
								aiNotes = cachedNotes
							} else {
								if flags.Force && aiReleaseNotesCache[cacheKey] != "" {
									fmt.Printf("  Force regenerating AI release notes for existing release %s (ignoring cache)\n", tag)
								} else {
									fmt.Printf("  Generating AI release notes for existing release %s...\n", tag)
								}
								var err error
								aiNotes, err = releaseManager.GenerateAIReleaseNotes(repoPath, repoName, tag, localTags, commits)
								if err != nil {
									fmt.Printf("  Warning: Failed to generate AI release notes: %v\n", err)
									// Clear cache on failure and track
									delete(aiReleaseNotesCache, cacheKey)
									// Determine which org we're updating for the failure message
									orgName := ""
									if githubOrg != nil && githubOrg.Name != "" {
										orgName = githubOrg.Name
									} else if codebergOrg != nil && codebergOrg.Name != "" {
										orgName = codebergOrg.Name
									}
									failedAIGenerations = append(failedAIGenerations, fmt.Sprintf("%s/%s:%s", orgName, repoName, tag))
									// Save cache after clearing the failed entry
									saveAIReleaseNotesCache(cacheFile, aiReleaseNotesCache)
									continue
								}
								aiReleaseNotesCache[cacheKey] = aiNotes // Cache only on success
								// Save cache immediately after successful generation
								if err := saveAIReleaseNotesCache(cacheFile, aiReleaseNotesCache); err != nil {
									fmt.Printf("  Warning: Failed to save cache: %v\n", err)
								}
							}
							
							// Print release notes to stdout
							fmt.Printf("\n%s\n", strings.Repeat("=", 70))
							fmt.Printf("Updated Release Notes for %s/%s tag %s:\n", codebergOrg.Name, repoName, tag)
							fmt.Printf("%s\n", strings.Repeat("-", 70))
							fmt.Println(aiNotes)
							fmt.Printf("%s\n\n", strings.Repeat("=", 70))
							
							msg := fmt.Sprintf("Update Codeberg release for %s/%s tag %s?", codebergOrg.Name, repoName, tag)
							
							updateRelease := false
							if flags.AutoCreateReleases {
								fmt.Printf("  Auto-updating Codeberg release for %s/%s tag %s\n", codebergOrg.Name, repoName, tag)
								updateRelease = true
							} else {
								updateRelease = release.PromptConfirmation(msg)
							}
							
							if updateRelease {
								if err := releaseManager.UpdateCodebergRelease(codebergOrg.Name, repoName, tag, aiNotes); err != nil {
									fmt.Printf("  Error updating Codeberg release: %v\n", err)
								} else {
									fmt.Printf("  Updated Codeberg release for tag %s\n", tag)
								}
							}
						}
					}
				}
			}
		}
	}
	
	return 0
}

// loadAIReleaseNotesCache loads the AI release notes cache from disk
func loadAIReleaseNotesCache(cacheFile string) map[string]string {
	cache := make(map[string]string)
	
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		// Cache file doesn't exist yet, return empty cache
		return cache
	}
	
	if err := json.Unmarshal(data, &cache); err != nil {
		fmt.Printf("Warning: Failed to parse AI release notes cache: %v\n", err)
		return make(map[string]string)
	}
	
	fmt.Printf("Loaded AI release notes cache with %d entries\n", len(cache))
	return cache
}

// saveAIReleaseNotesCache saves the AI release notes cache to disk
func saveAIReleaseNotesCache(cacheFile string, cache map[string]string) error {
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}
	
	if err := os.WriteFile(cacheFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}
	
	// Don't print on every save since we save after each generation
	return nil
}
