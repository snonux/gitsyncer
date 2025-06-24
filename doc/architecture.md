# GitSyncer Architecture

## Overview

GitSyncer is designed as a command-line tool that synchronizes Git repositories across multiple platforms. It follows a modular architecture with clear separation of concerns between CLI handling, API clients, configuration management, and core synchronization logic.

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         CLI Layer                             │
│                    (cmd/gitsyncer/main.go)                   │
└─────────────────────────┬───────────────────────────────────┘
                          │
┌─────────────────────────┴───────────────────────────────────┐
│                     CLI Handlers                             │
│              (internal/cli/handlers.go)                      │
│           (internal/cli/sync_handlers.go)                    │
└──────┬──────────────────┬──────────────────┬────────────────┘
       │                  │                  │
┌──────┴──────┐    ┌──────┴──────┐    ┌─────┴──────┐
│   Config    │    │  API Clients │    │    Sync    │
│  Manager    │    │              │    │   Engine   │
│(config.go)  │    │ - GitHub     │    │ (sync.go)  │
│             │    │ - Codeberg   │    │            │
└─────────────┘    └──────────────┘    └────────────┘
```

## Component Architecture

### 1. Entry Point (cmd/gitsyncer/main.go)

The main function serves as the application entry point and:
- Parses command-line flags
- Routes to appropriate handlers based on flags
- Manages application lifecycle and exit codes

### 2. CLI Layer (internal/cli/)

The CLI layer is responsible for user interaction and consists of:

#### flags.go
- Defines all command-line flags
- Provides flag parsing logic
- Returns a structured `Flags` object

#### handlers.go
- General command handlers (version, config, list operations)
- Configuration loading and validation
- Error presentation to users

#### sync_handlers.go
- Sync-specific operations
- Orchestrates API clients and sync engine
- Handles batch operations

### 3. Configuration Management (internal/config/)

- Loads JSON configuration files
- Validates configuration structure
- Provides helper methods for finding organizations
- Supports multiple configuration file locations

### 4. API Clients

#### GitHub Client (internal/github/)
- Authenticates using personal access tokens
- Creates repositories via GitHub API
- Lists public repositories with pagination
- Handles multiple token sources (config, env, file)

#### Codeberg Client (internal/codeberg/)
- Interacts with Codeberg's Gitea API
- Lists public repositories for users/organizations
- Supports pagination for large repository lists
- No authentication required for public operations

### 5. Sync Engine (internal/sync/)

The core synchronization logic is divided into several components:

#### sync.go - Main Orchestrator
- Coordinates the entire sync process
- Manages working directory
- Handles repository-level operations

#### repository_setup.go
- Clones repositories or ensures they exist
- Configures Git remotes
- Handles initial repository setup

#### branch_sync.go
- Manages branch-level synchronization
- Tracks which remotes have which branches
- Orchestrates merge and push operations

#### git_operations.go
- Low-level Git command wrappers
- Handles merge conflicts
- Manages stashing and checkout operations

#### branch_filter.go
- Implements regex-based branch filtering
- Excludes branches based on patterns
- Provides filtering reports

#### branch_analyzer.go
- Detects abandoned branches (6+ months inactive)
- Generates abandonment reports
- Analyzes branch activity

### 6. Version Management (internal/version/)

- Provides version information
- Supports build-time metadata injection
- Formats version strings for display

## Data Flow

### Sync Operation Flow

1. **Configuration Loading**
   ```
   User → CLI → Config Loader → Config Validation
   ```

2. **Repository Discovery**
   ```
   Config → API Clients → Repository Lists → Filtering
   ```

3. **Synchronization Process**
   ```
   For each repository:
     └→ Setup/Clone Repository
     └→ Configure Remotes
     └→ Fetch All Remotes
     └→ Get All Branches
     └→ Filter Branches
     └→ For each branch:
         └→ Checkout/Create Branch
         └→ Merge from Remotes
         └→ Push to All Remotes
     └→ Analyze Abandoned Branches
     └→ Generate Reports
   ```

## Design Principles

### 1. Modularity
Each package has a single, well-defined responsibility:
- CLI handling is separate from business logic
- API clients are independent and interchangeable
- Core sync logic is platform-agnostic

### 2. Configuration-Driven
- All behavior is controlled via configuration
- No hard-coded organization or repository names
- Flexible remote naming based on hosts

### 3. Error Handling
- Graceful degradation (missing repos don't stop sync)
- Clear error messages with actionable guidance
- Proper exit codes for scripting

### 4. Extensibility
- New platforms can be added by implementing API clients
- Branch filtering is regex-based for flexibility
- Sync strategies can be extended

## Security Considerations

### Token Management
- GitHub tokens are never logged or displayed
- Multiple token sources for flexibility
- Tokens are loaded on-demand

### Git Operations
- All operations use standard Git commands
- No custom Git protocol implementation
- Respects Git's security model

## Performance Characteristics

### Scalability
- Handles multiple repositories in sequence
- Pagination support for large repository lists
- Efficient branch filtering

### Resource Usage
- Minimal memory footprint
- Disk usage proportional to repository sizes
- Network usage optimized with selective fetching

## Future Architecture Considerations

### Planned Enhancements
1. **Parallel Synchronization** - Sync multiple repos concurrently
2. **Webhook Support** - Trigger syncs on push events
3. **More Platforms** - GitLab, Bitbucket, Gitea support
4. **Conflict Resolution** - Automated conflict resolution strategies

### Extension Points
- Platform interface for new Git hosts
- Pluggable authentication mechanisms
- Custom sync strategies
- Hook system for pre/post sync actions