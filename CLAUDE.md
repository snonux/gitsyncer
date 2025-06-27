# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

Essential commands for development:

```bash
# Build the binary (using go-task if installed)
task            # or: go build -o gitsyncer ./cmd/gitsyncer

# Build for all platforms
task build-all

# Run the application
task run        # or: ./gitsyncer

# Run tests
task test

# Format code
task fmt

# Clean build artifacts
task clean

# Show version
./gitsyncer --version

# Delete a repository from all configured organizations (with confirmation)
./gitsyncer --delete-repo <repository-name>
```

Note: The Taskfile.yaml is configured for [go-task](https://taskfile.dev/). Install with:
```bash
# macOS
brew install go-task

# Linux
sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b ~/.local/bin

# Or use Go directly if task is not installed
go build -o gitsyncer ./cmd/gitsyncer
```

## Project Structure

```
gitsyncer/
├── cmd/
│   └── gitsyncer/
│       └── main.go           # Main entry point with CLI flags
├── internal/
│   └── version/
│       └── version.go        # Version information
├── go.mod                    # Go module definition
└── Taskfile.yaml            # Task automation (go-task)
```

This follows the standard Go project layout with:
- `cmd/` for application entry points
- `internal/` for private application code
- Root directory for public libraries (if any)

## Architecture

The application currently provides:
- Version information system (internal/version)
- CLI flag parsing for --version
- Placeholder for main gitsyncer functionality

## Next Steps

The project needs:
1. Support for other platforms (GitLab, Gitea, etc.)
2. Webhook support for automatic syncing
3. Conflict resolution strategies
4. Better handling of large repositories