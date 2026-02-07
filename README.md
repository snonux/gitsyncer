# GitSyncer

GitSyncer is a tool for synchronizing git repositories between multiple organizations (e.g., GitHub and Codeberg). It automatically keeps all branches in sync across different git hosting platforms.

It has been vibe coded mainly using AI tools (Claude Code CLI and amp).

## Documentation

📚 **[Full documentation is available in the doc/ directory](doc/README.md)**

- [Architecture Overview](doc/architecture.md) - System design and components
- [API Reference](doc/api-reference.md) - Complete reference of all packages and functions
- [Configuration Guide](doc/configuration.md) - Detailed configuration options
- [Usage Examples](doc/examples.md) - Common usage patterns and workflows
- [Development Guide](doc/development.md) - Contributing and development setup

## Features

- Sync repositories between multiple git organizations
- Automatic branch creation on remotes that don't have them
- Batch sync multiple repositories with a single command
- Sync all public repositories from Codeberg to GitHub
- Sync all public repositories from GitHub to Codeberg
- Automatic repository creation on GitHub and Codeberg
- SSH backup locations with automatic bare repository creation
- One-way backup to private SSH servers (e.g., home NAS)
- Merge conflict detection with clear error messages
- Never deletes branches (only adds/updates)
- GitHub token validation tool
- Opt-in backup mode with --backup flag for resilient offline backups
- Opt-in sync throttling with --throttle based on local activity
- AI-powered project showcase generation for documentation
- Weekly batch run mode with --batch-run for automated synchronization

## Installation

```bash
go build -o gitsyncer ./cmd/gitsyncer
```

## Configuration

Create a configuration file at `~/.config/gitsyncer/config.json` (or specify a custom path with `-c`):

```json
{
  "organizations": [
    {
      "host": "git@codeberg.org",
      "name": "yourusername"
    },
    {
      "host": "git@github.com",
      "name": "yourusername"
    },
    {
      "host": "user@nas.local:git",
      "backupLocation": true
    }
  ],
  "repositories": [
    "repo1",
    "repo2"
  ]
}
```

## Usage

### Command Structure

GitSyncer uses a modern command-based structure that provides:
- Clear organization of related functionality
- Built-in help for every command and subcommand
- Consistent flag naming and behavior
- Better discoverability of features

### Quick Start

Explore available commands and get help:

```bash
# Show available commands
gitsyncer --help

# Show help for a specific command
gitsyncer sync --help
```

### Synchronization Commands

```bash
gitsyncer sync repo myproject

# Include backup locations
gitsyncer sync repo myproject --backup

# Preview what would be synced
gitsyncer sync repo myproject --dry-run

# Sync without AI-generated release notes
gitsyncer sync repo myproject --no-ai-release-notes

# Auto-create releases without prompts (AI notes enabled by default)
gitsyncer sync repo myproject --auto-create-releases
```

#### Throttled sync
```bash
# Throttle syncing based on local activity in ~/git/<repo>
gitsyncer sync repo myproject --throttle

# Throttle all public repo sync modes
gitsyncer sync bidirectional --throttle
gitsyncer sync codeberg-to-github --throttle
gitsyncer sync github-to-codeberg --throttle
```
When `--throttle` is enabled, GitSyncer checks `~/git/<repo>` for commits in the last 7 days. If no recent commits are found (or the repo is missing locally), the repo sync is allowed only once per random interval between 60 and 120 days and the next allowed date is stored. Throttle state is stored in `.gitsyncer-state.json` in the work directory.

#### Sync all configured repositories
```bash
gitsyncer sync all

# Include backup locations
gitsyncer sync all --backup
```

#### Sync Codeberg to GitHub
```bash
# Sync all public Codeberg repositories to GitHub
gitsyncer sync codeberg-to-github

# Auto-create missing GitHub repos
gitsyncer sync codeberg-to-github --create-repos

# Preview changes
gitsyncer sync codeberg-to-github --dry-run
```

#### Sync GitHub to Codeberg
```bash
# Sync all public GitHub repositories to Codeberg
gitsyncer sync github-to-codeberg

# Auto-create missing Codeberg repos
gitsyncer sync github-to-codeberg --create-repos
```

#### Full bidirectional sync
```bash
# Complete bidirectional sync of all public repos
gitsyncer sync bidirectional

# Preview what would be synced
gitsyncer sync bidirectional --dry-run

# Include backup locations
gitsyncer sync bidirectional --backup
```

### Release Management

#### Check for missing releases
```bash
# Check all repositories
gitsyncer release check

# Check specific repository
gitsyncer release check myproject
```

#### Create releases
```bash
# Create releases with confirmation prompts (AI notes enabled by default)
gitsyncer release create

# Auto-create without prompts
gitsyncer release create --auto

# Create without AI-generated notes
gitsyncer release create --no-ai-notes

# Update existing releases with AI notes
gitsyncer release create --update-existing

# Create for specific repository without AI
gitsyncer release create myproject --no-ai-notes

# Use amp for AI release notes (default)
gitsyncer release create --ai-tool amp
```

#### AI Release Notes Engines

- Default flow: tries `amp` first by piping the generated commit/diff payload to stdin and passing an instruction prompt via `--execute` (equivalent to `echo SOMETEXT | amp --execute 'PROMPT'`).
- Fallback: if `amp` is not available or fails, falls back to `hexai`, then `claude --model sonnet`, then to `aichat`.
- Explicit tool: `--ai-tool claude` or `--ai-tool aichat` influences the fallback preference, but `amp` is still attempted first when available.
- Requirements: ensure `amp`, `hexai`, `claude`, or `aichat` are installed and available in `PATH`.

### Project Showcase

```bash
# Generate showcase with cached summaries
gitsyncer showcase

# Force regeneration of all summaries
gitsyncer showcase --force

# Custom output path
gitsyncer showcase --output ~/my-showcase.md

# Different output format
gitsyncer showcase --format markdown

# Exclude certain repositories
gitsyncer showcase --exclude "test-.*"
```

### Repository Management

#### Delete repository
```bash
# Delete repository from all organizations (with confirmation)
gitsyncer manage delete-repo old-project
```

#### Clean workspace
```bash
# Clean work directory (with confirmation)
gitsyncer manage clean

# Force clean without confirmation
gitsyncer manage clean --force
```

#### Automated weekly sync
```bash
# Run weekly batch sync (full sync + showcase)
gitsyncer manage batch-run

# Force run even if already run this week
gitsyncer manage batch-run --force
```

### Testing and Information

#### Test authentication
```bash
# Test GitHub token
gitsyncer test github-token

# Test Codeberg token
gitsyncer test codeberg-token

# Validate configuration
gitsyncer test config
```

#### List configured items
```bash
# List organizations
gitsyncer list orgs

# List repositories
gitsyncer list repos
```

#### Show version
```bash
gitsyncer version
```

### Global Options

These options are available for all commands:

- `-c, --config` - Path to configuration file (default: ~/.config/gitsyncer/config.json)
- `-w, --work-dir` - Working directory (default: ~/git/gitsyncer-workdir)
- `-h, --help` - Show help for any command

## The --backup Flag

The `--backup` flag enables syncing to backup locations configured in your config file. This is particularly useful when:
- Your backup server might be offline (e.g., home NAS)
- You want to control when backups happen
- You need to separate regular syncing from backup operations

Without `--backup`: GitSyncer only syncs between primary git hosts (GitHub, Codeberg, etc.)
With `--backup`: GitSyncer also pushes to backup locations marked with `"backupLocation": true`

```bash
# Regular sync (backup locations ignored)
gitsyncer sync repo myrepo

# Sync with backup enabled
gitsyncer sync repo myrepo --backup
```

## How It Works

1. GitSyncer clones the repository from the first configured organization
2. Adds all other organizations as git remotes
3. For each branch:
   - Fetches from all remotes
   - Merges changes from remotes that have the branch
   - Pushes to all remotes (creating branches if needed)

## Branch Exclusion

You can exclude branches from synchronization using regex patterns in your configuration:

```json
{
  "organizations": [...],
  "repositories": [...],
  "exclude_branches": [
    "^codex/",    // Exclude branches starting with "codex/"
    "^temp-",     // Exclude branches starting with "temp-"
    "-wip$",      // Exclude branches ending with "-wip"
    "experimental" // Exclude branches containing "experimental"
  ]
}
```

Excluded branches will be reported during sync but not synchronized.

## SSH Backup Locations

You can configure SSH backup locations for one-way repository backups to private servers:

```json
{
  "organizations": [
    {
      "host": "git@github.com",
      "name": "yourusername"
    },
    {
      "host": "paul@t450:git",
      "backupLocation": true
    }
  ]
}
```

### How SSH Backup Works

1. **Opt-in feature**: Backup locations are disabled by default. Use the `--backup` flag to enable syncing to them
2. **One-way sync**: Repositories are only pushed TO backup locations, never pulled FROM them
3. **Automatic repository creation**: If a repository doesn't exist on the SSH server, GitSyncer will:
   - SSH into the server
   - Create the directory structure
   - Initialize a bare git repository
4. **Archive functionality**: Repositories that exist only on the backup location are considered archived and won't be synced to other organizations
5. **All branches and tags**: Every branch and tag is pushed to the backup location when `--backup` is used

### SSH Backup Example

```bash
# Configure your config file with an SSH backup location
# Backup locations are DISABLED by default to handle offline servers

# Sync without backup (default behavior)
gitsyncer sync repo myrepo

# Sync WITH backup enabled
gitsyncer sync repo myrepo --backup

# Sync all repositories with backup
gitsyncer sync all --backup

# Full sync with backup
gitsyncer sync bidirectional --backup
```

The backup location path format is: `user@host:path/REPONAME.git`
- `user@host`: SSH connection string
- `path`: Base directory for repositories
- `REPONAME.git`: Automatically appended repository name

**Note**: The `--backup` flag is required to sync to backup locations. This allows GitSyncer to work normally even when backup servers are offline or unreachable.

## Project Showcase Generation

GitSyncer can generate a comprehensive showcase of all your projects using AI (amp by default). This feature creates a formatted document with project summaries, statistics, and code snippets.

### How it works

1. **Repository Analysis**: GitSyncer analyzes all cloned repositories to extract:
   - Programming languages and their usage percentages
   - Commit history and development activity
   - Lines of code and documentation
   - License information
   - Latest release version and date
   - AI-assistance detection (looks for CLAUDE.md, GEMINI.md files)

2. **AI-Powered Summaries**: Uses AI (amp, hexai, claude, or aichat) to generate concise project descriptions that explain:
   - What the project does
   - Why it's useful
   - How it's implemented
   - Key features and architecture

3. **Automatic Features**:
   - Extracts README images (including SVG support)
   - Selects representative code snippets
   - Orders projects by recent activity
   - Generates overall portfolio statistics
   - Caches summaries to avoid redundant AI calls

### Usage

```bash
# Generate showcase (uses cached summaries when available)
gitsyncer showcase

# Force regeneration of all summaries
gitsyncer showcase --force

# Custom output path and format
gitsyncer showcase --output ~/showcase.md --format markdown
```

### Output

The showcase is generated in Gemini Gemtext format and includes:
- Overall statistics (total projects, commits, lines of code, languages)
- Release status breakdown (released vs experimental projects)
- AI-assistance statistics
- Individual project sections with:
  - Language breakdown
  - Development metrics
  - Latest release information or experimental status
  - Project description
  - Code snippet example
  - Links to repositories

### Configuration

The showcase output is written to `~/git/foo.zone-content/gemtext/about/showcase.gmi.tpl` by default (currently hardcoded).

Projects can be excluded from the showcase by creating a `.nosync` file in their repository root.

## Example Workflows

### Automated weekly synchronization
The batch-run feature is designed for automated weekly synchronization from cron jobs or shell scripts:

1. Add to your crontab or shell profile:
   ```bash
   # Run daily - gitsyncer will only execute once per week
   0 2 * * * /path/to/gitsyncer manage batch-run
   ```

2. On each run, GitSyncer will:
   - Check if a week has passed since the last batch run
   - If yes: Execute full sync and showcase generation
   - If no: Skip execution and show when the last run occurred
   - Save the timestamp to `.gitsyncer-state.json` in your work directory

3. Benefits:
   - Prevents excessive API usage
   - Can be safely called daily/hourly without worry
   - Maintains weekly sync cadence automatically
   - Shows state file location for debugging

### Sync specific repositories
1. Create repositories on all platforms (GitHub, Codeberg, etc.)
2. Add the repository name to your configuration file
3. Run `gitsyncer sync repo repo-name`
4. GitSyncer will:
   - Clone from the first organization
   - Push all branches to other organizations
   - Keep them in sync going forward

### Sync all public Codeberg repositories
1. Ensure Codeberg is in your organizations list in the config
2. Run `gitsyncer sync codeberg-to-github`
3. GitSyncer will:
   - Fetch all public repositories from your Codeberg account
   - Sync each one to all other configured organizations
   - Skip any that fail (e.g., don't exist on other platforms)

## Error Handling

- **Merge conflicts**: GitSyncer will detect conflicts and exit with an error message
- **Missing repositories**: Must be created manually on all platforms
- **Missing branches**: Automatically created on remotes that don't have them

## License

BSD 2-Clause License. See [LICENSE](LICENSE) file for details.
