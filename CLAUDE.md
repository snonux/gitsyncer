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

# Manually check for version tags without releases
./gitsyncer --check-releases

# Disable automatic release checking during sync operations
./gitsyncer --sync-all --no-check-releases

# Automatically create releases without confirmation prompts
./gitsyncer --check-releases --auto-create-releases
./gitsyncer --sync-all --auto-create-releases
```

Note: Release checking is enabled by default after sync operations. It will check for version tags (formats: vX.Y.Z, vX.Y, vX, X.Y.Z, X.Y, X) that don't have corresponding releases on GitHub/Codeberg and prompt for confirmation before creating them.

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
- Automatic release checking and creation (internal/release)
- Repository syncing across GitHub, Codeberg, and other platforms
- Project showcase generation

## Next Steps

The project needs:
1. Support for other platforms (GitLab, Gitea, etc.)
2. Webhook support for automatic syncing
3. Conflict resolution strategies
4. Better handling of large repositories

## Release Process

- When releasing a version, increment the version in version.go, commit all changes to git and push. and tag the version and push to git.
- Gitsyncer will automatically detect the new version tag and prompt to create releases on GitHub and Codeberg using the tokens from your gitsyncer configuration file.