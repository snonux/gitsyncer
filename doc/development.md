# GitSyncer Development Guide

This guide is for contributors who want to help develop GitSyncer.

## Table of Contents

- [Development Setup](#development-setup)
- [Project Structure](#project-structure)
- [Building](#building)
- [Testing](#testing)
- [Code Style](#code-style)
- [Adding Features](#adding-features)
- [Contributing](#contributing)

## Development Setup

### Prerequisites

- Go 1.21 or later
- Git
- Make (optional, for Makefile)
- Task (optional, for Taskfile)

### Clone the Repository

```bash
# Clone from Codeberg
git clone https://codeberg.org/snonux/gitsyncer.git
cd gitsyncer

# Or from GitHub mirror
git clone https://github.com/snonux/gitsyncer.git
cd gitsyncer
```

### Install Dependencies

```bash
# Download Go modules
go mod download

# Verify modules
go mod verify
```

### Install Development Tools

```bash
# Install Task runner (optional)
# macOS
brew install go-task

# Linux
sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b ~/.local/bin

# Install golangci-lint for linting
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

## Project Structure

```
gitsyncer/
├── cmd/gitsyncer/          # Application entry point
│   └── main.go            # Main function
├── internal/              # Private packages
│   ├── cli/              # CLI handling
│   │   ├── flags.go      # Command-line flags
│   │   ├── handlers.go   # General handlers
│   │   └── sync_handlers.go # Sync handlers
│   ├── codeberg/         # Codeberg API client
│   │   └── codeberg.go   # Codeberg implementation
│   ├── config/           # Configuration
│   │   └── config.go     # Config structures
│   ├── github/           # GitHub API client
│   │   └── github.go     # GitHub implementation
│   ├── sync/             # Core sync logic
│   │   ├── sync.go       # Main syncer
│   │   ├── branch_analyzer.go    # Branch analysis
│   │   ├── branch_filter.go      # Branch filtering
│   │   ├── branch_sync.go        # Branch operations
│   │   ├── git_operations.go     # Git commands
│   │   └── repository_setup.go   # Repo setup
│   └── version/          # Version info
│       └── version.go    # Version constants
├── test/                 # Integration tests
│   ├── run_integration_tests.sh  # Main test runner
│   └── test_*.sh         # Individual tests
├── doc/                  # Documentation
├── go.mod               # Go module definition
├── go.sum               # Go module checksums
├── LICENSE              # BSD 2-Clause License
├── README.md            # Project README
├── CLAUDE.md            # AI assistant hints
└── Taskfile.yaml        # Task automation
```

## Building

### Using Task (Recommended)

```bash
# Build for current platform
task build

# Build for all platforms
task build-all

# Run directly
task run

# Run with arguments
task run -- --version
```

### Using Go Directly

```bash
# Build binary
go build -o gitsyncer ./cmd/gitsyncer

# Build with version info
go build -ldflags "\
  -X codeberg.org/snonux/gitsyncer/internal/version.Version=0.1.0 \
  -X codeberg.org/snonux/gitsyncer/internal/version.GitCommit=$(git rev-parse --short HEAD) \
  -X codeberg.org/snonux/gitsyncer/internal/version.BuildDate=$(date -u +%Y-%m-%d)" \
  -o gitsyncer ./cmd/gitsyncer

# Cross-compile for Linux
GOOS=linux GOARCH=amd64 go build -o gitsyncer-linux-amd64 ./cmd/gitsyncer

# Cross-compile for macOS
GOOS=darwin GOARCH=amd64 go build -o gitsyncer-darwin-amd64 ./cmd/gitsyncer
GOOS=darwin GOARCH=arm64 go build -o gitsyncer-darwin-arm64 ./cmd/gitsyncer
```

## Testing

### Unit Tests

Currently, the project has no unit tests. When adding new features, please include tests.

```bash
# Run all tests (when available)
go test ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/sync/...
```

### Integration Tests

```bash
# Run all integration tests
cd test
./run_integration_tests.sh

# Run specific test
./test_branch_creation.sh
./test_conflict.sh
```

### Writing Tests

Example test structure:
```go
// internal/sync/sync_test.go
package sync

import (
    "testing"
    "codeberg.org/snonux/gitsyncer/internal/config"
)

func TestNew(t *testing.T) {
    cfg := &config.Config{
        Organizations: []config.Organization{
            {Host: "git@github.com", Name: "test"},
        },
    }
    
    syncer := New(cfg, "/tmp/test")
    if syncer == nil {
        t.Fatal("New() returned nil")
    }
    
    if syncer.workDir != "/tmp/test" {
        t.Errorf("workDir = %q, want %q", syncer.workDir, "/tmp/test")
    }
}
```

## Code Style

### Go Code

Follow standard Go conventions:

1. **Formatting**: Use `gofmt` or `goimports`
   ```bash
   # Format all files
   task fmt
   # or
   gofmt -w .
   ```

2. **Naming**: 
   - Exported names start with capital letter
   - Use CamelCase, not snake_case
   - Acronyms should be all caps (URL, API, ID)

3. **Comments**:
   - Export functions/types need comments starting with the name
   - Keep comments up-to-date with code changes

4. **Error Handling**:
   ```go
   // Good
   if err := doSomething(); err != nil {
       return fmt.Errorf("failed to do something: %w", err)
   }
   
   // Avoid bare returns
   if err != nil {
       return err  // Missing context
   }
   ```

5. **Interfaces**: Accept interfaces, return structs
   ```go
   // Good
   func ProcessRepo(r Repository) error { ... }
   
   // Avoid
   func ProcessRepo(r *GitHubRepository) error { ... }
   ```

### Commit Messages

Follow conventional commits:
```
type(scope): description

[optional body]

[optional footer]
```

Examples:
```
feat(sync): add support for GitLab repositories
fix(config): handle missing organizations gracefully
docs: update API reference for new methods
refactor(cli): extract common handler logic
test: add integration tests for branch filtering
```

## Adding Features

### Adding a New Git Platform

1. Create new package in `internal/`:
   ```bash
   mkdir internal/gitlab
   touch internal/gitlab/gitlab.go
   ```

2. Implement platform client:
   ```go
   package gitlab
   
   type Client struct {
       baseURL string
       token   string
       org     string
   }
   
   func NewClient(token, org string) Client {
       return Client{
           baseURL: "https://gitlab.com/api/v4",
           token:   token,
           org:     org,
       }
   }
   
   func (c *Client) ListPublicRepos() ([]Repository, error) {
       // Implementation
   }
   ```

3. Update config to recognize platform:
   ```go
   func (o *Organization) IsGitLab() bool {
       return strings.Contains(o.Host, "gitlab.com")
   }
   ```

4. Add sync support in handlers

### Adding a New Command Flag

1. Add flag in `internal/cli/flags.go`:
   ```go
   type Flags struct {
       // ... existing flags
       NewFeature bool  // Add new flag
   }
   
   func ParseFlags() *Flags {
       flags := &Flags{}
       // ... existing flags
       flag.BoolVar(&flags.NewFeature, "new-feature", false, "Enable new feature")
   }
   ```

2. Handle flag in `cmd/gitsyncer/main.go`:
   ```go
   if flags.NewFeature {
       os.Exit(cli.HandleNewFeature(cfg, flags))
   }
   ```

3. Implement handler in `internal/cli/handlers.go`

### Adding a New Configuration Option

1. Update config struct in `internal/config/config.go`:
   ```go
   type Config struct {
       Organizations   []Organization `json:"organizations"`
       Repositories    []string       `json:"repositories"`
       ExcludeBranches []string       `json:"exclude_branches"`
       NewOption       string         `json:"new_option"`  // Add new field
   }
   ```

2. Add validation if needed:
   ```go
   func (c *Config) Validate() error {
       // ... existing validation
       if c.NewOption != "" && !isValidOption(c.NewOption) {
           return fmt.Errorf("invalid new_option: %s", c.NewOption)
       }
   }
   ```

3. Use in sync logic as needed

## Contributing

### Before Submitting

1. **Test your changes**:
   ```bash
   # Run integration tests
   cd test && ./run_integration_tests.sh
   
   # Test manually
   ./gitsyncer --sync test-repo
   ```

2. **Format code**:
   ```bash
   task fmt
   ```

3. **Update documentation**:
   - Update relevant docs in `doc/`
   - Update README if adding user-facing features
   - Add examples to `doc/examples.md`

4. **Update CLAUDE.md** if adding development commands

### Pull Request Process

1. Fork the repository
2. Create feature branch:
   ```bash
   git checkout -b feat/my-feature
   ```

3. Make changes and commit:
   ```bash
   git add .
   git commit -m "feat: add my feature"
   ```

4. Push to your fork:
   ```bash
   git push origin feat/my-feature
   ```

5. Create pull request with:
   - Clear description of changes
   - Link to related issues
   - Test results

### Code Review Guidelines

- Respond to feedback constructively
- Make requested changes promptly
- Keep PR scope focused
- Update PR description as changes evolve

## Debugging

### Debug Output

Add debug logging:
```go
import "log"

func (s *Syncer) debugOperation() {
    if os.Getenv("GITSYNCER_DEBUG") != "" {
        log.Printf("Debug: operation details: %+v", s)
    }
}
```

Use:
```bash
GITSYNCER_DEBUG=1 gitsyncer --sync test-repo
```

### Common Issues

1. **Import cycle**: Keep dependencies acyclic
2. **Nil pointer**: Always check returns from New functions
3. **Git operations**: Ensure working directory is clean

## Release Process

1. Update version in `internal/version/version.go`
2. Update CHANGELOG.md
3. Tag release:
   ```bash
   git tag -a v0.1.0 -m "Release v0.1.0"
   git push origin v0.1.0
   ```
4. Build releases:
   ```bash
   task build-all
   ```
5. Create GitHub/Codeberg release with binaries