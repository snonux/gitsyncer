# GitSyncer

GitSyncer is a tool for synchronizing git repositories between multiple organizations (e.g., GitHub and Forgejo). It automatically keeps all branches in sync across different git hosting platforms.

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
- Discover and sync public GitHub repositories to configured destinations
- Automatic repository creation on GitHub and Forgejo
- SSH backup locations with automatic bare repository creation
- One-way backup to private SSH servers (e.g., home NAS)
- Merge conflict detection with clear error messages
- Never deletes branches (only adds/updates)
- GitHub token validation tool
- Backup sync for full-sync modes, with `--backup` available for single-repo and `sync all` runs
- Per-destination backup fail-fast for a run: after a backup fails, later repos skip only that destination
- Default once-daily sync limit with --force override
- Opt-in sync throttling with --throttle based on local activity
- AI-powered project showcase generation for documentation
- Weekly batch run mode with `manage batch-run` for automated synchronization

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
      "host": "ssh://git@forgejo.example",
      "name": "yourusername",
      "forgejo_api_base": "https://forgejo.example/api/v1",
      "forgejo_owner": "yourusername"
    },
    {
      "host": "git@github.com",
      "name": "yourusername"
    },
    {
      "host": "user@nas.local:git",
      "backupLocation": true,
      "forcePush": true,
      "descriptionSyncHost": "root@nas.local",
      "descriptionSyncRoot": "/srv/git/repos"
    }
  ],
  "repositories": [
    "repo1",
    "repo2"
  ],
  "showcase_output_dir": "~/git/foo.zone-content/gemtext/about",
  "showcase_stats_branches": {
    "foo.zone": "content-gemtext"
  },
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

# Include backup locations for a single repository run
gitsyncer sync repo myproject --backup

# Preview what would be synced
gitsyncer sync repo myproject --dry-run

# Override sync interval checks
gitsyncer sync repo myproject --force

# Sync without AI-generated release notes
gitsyncer sync repo myproject --no-ai-release-notes

# Auto-create releases without prompts (AI notes enabled by default)
gitsyncer sync repo myproject --auto-create-releases
```

Each repository is synced at most once every 24 hours by default. Successful sync times are stored in `.gitsyncer-state.json` inside the work directory. Use `--force` to bypass sync interval checks.

#### Throttled sync
```bash
# Throttle syncing based on local activity in ~/git/<repo>
gitsyncer sync repo myproject --throttle

# Throttle all public repo sync modes
gitsyncer sync bidirectional --throttle
gitsyncer sync github-to-forgejo --throttle
```
When `--throttle` is enabled, GitSyncer still applies the default once-daily limit first, then checks `~/git/<repo>` for commits in the last 7 days. If no recent commits are found (or the repo is missing locally), the repo sync is allowed only once per random interval between 60 and 120 days and the next allowed date is stored. Sync state is stored in `.gitsyncer-state.json` in the work directory. Use `--force` to bypass both interval checks.

#### Sync all configured repositories
```bash
gitsyncer sync all

# Include backup locations when running sync all
gitsyncer sync all --backup
```

#### Full bidirectional sync
```bash
# Complete bidirectional sync of all public repos
gitsyncer sync bidirectional

# Preview what would be synced
gitsyncer sync bidirectional --dry-run
```

`sync bidirectional`, `sync github-to-forgejo`, and `manage batch-run` now always try configured backup locations when `backupLocation: true` is present in the config. If a backup push fails because that host is offline or unavailable, GitSyncer records that failure in memory and skips that destination for the rest of the process while continuing the primary sync targets and other backups.

Forgejo is a bidirectional sync peer by default: changes from either GitHub or Forgejo are fetched, merged, and pushed to both locations. Repository creation uses its Gitea-compatible API, while Git data is transferred over SSH. Forgejo backup descriptions are updated through the API. Set `optional: true` when an unavailable Forgejo peer should be skipped for the current run:

```json
{
  "organizations": [{
    "host": "ssh://git@code.f3s.buetow.org:2022",
    "forgejo_api_base": "https://code.f3s.buetow.org/api/v1",
    "forgejo_owner": "snonux",
    "forgejo_owner_type": "organization",
    "optional": true
  }]
}
```

To mirror GitHub to Forgejo one way, set `pushOnly: true` (it may be combined
with `optional: true` so an unreachable Forgejo is skipped instead of failing
the run):

```json
{
  "organizations": [{
    "host": "ssh://git@code.f3s.buetow.org:2022",
    "forgejo_api_base": "https://code.f3s.buetow.org/api/v1",
    "forgejo_owner": "snonux",
    "forgejo_owner_type": "organization",
    "optional": true,
    "pushOnly": true
  }]
}
```

A `pushOnly` organization is pushed on every sync (no `--backup` needed) but is
never fetched, merged, or cloned from, so commits, branches, and tags that exist
only there never reach GitHub. While a `pushOnly` organization is configured,
the GitHub discovery commands (`sync github-to-forgejo`, `sync bidirectional`,
`manage batch-run`) sync every public, non-fork, non-archived GitHub repository
instead of only those in `repositories`; that list then only decides which
repositories also go to Forgejo. Pushes are not forced unless `forcePush: true`
is set, so a branch or tag that diverged on the mirror makes its push fail
rather than being overwritten. GitSyncer never deletes branches or tags on the
mirror.

Set an API token with the minimal scopes for the configured owner type:
`write:repository` plus `write:organization` for an organization, or
`write:repository` plus `write:user` for a user. The owner type may be `user`
(the default for backward compatibility) or `organization`. For an organization
owner, use a token from an interactive user that is authorized to create
repositories in that organization; an organization pseudo-user cannot issue a
usable token. Store the token in
protected process environment as `FORGEJO_TOKEN`, or store it in
`~/.gitsyncer_forgejo_token` with mode `0600`. The environment takes precedence;
surrounding
whitespace is removed from either source. Never put a Forgejo token in the JSON
configuration. A repository named `demo` uses
`ssh://git@code.f3s.buetow.org:2022/snonux/demo.git`. Forgejo targets with
`backupLocation: true` or `pushOnly: true` are never fetched. Forgejo targets
without either setting participate fully in branch discovery, fetch, merge, and push; `optional: true` only makes a failed peer non-fatal. Metadata API
failures are reported but do not prevent Git-over-SSH synchronization; a fetch
or push failure skips an `optional` Forgejo peer for the remainder of the run without
interrupting synchronization between the other hosts. GitSyncer never runs
remote shell repository-creation or description-file commands for Forgejo.
When a Forgejo organization is configured and `FORGEJO_TOKEN` (or
`~/.gitsyncer_forgejo_token`) is available, release checking also creates and
updates releases on Forgejo using the same Gitea-compatible API.

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

# Use amp for AI release notes
gitsyncer release create --ai-tool amp
```

#### AI Release Notes Engines

- Default flow: tries `ollama run glm-5.3-flash:cloud` first with the commit/diff payload and instruction prompt combined into a single argument (`--hidethinking` so chain-of-thought is not captured).
- Fallback: if ollama is not available or fails, falls back to `hexai`, then `claude --model sonnet`, then `amp`.
- Explicit tool: `--ai-tool hexai`, `--ai-tool claude`, `--ai-tool amp`, etc. adjust the fallback chain order. OpenRouter/`pi` is no longer used.
- Requirements: ensure `ollama`, `hexai`, `claude`, or `amp` are installed and available in `PATH`.

### Project Showcase

```bash
# Generate showcase with cached summaries
gitsyncer showcase

# Force regeneration of all summaries
gitsyncer showcase --force

# Generate showcase for one repository
gitsyncer showcase --repo gitsyncer

# Use a specific AI tool
gitsyncer showcase --ai-tool opencode
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

# # Validate configuration
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

Without `--backup`: GitSyncer only syncs between primary git hosts (GitHub, Forgejo, etc.)
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

## Branch Selection and Exclusion

By default, GitSyncer synchronizes only `main`, `master`, and `develop`. Set a
global list with `branches`, then override it for individual repositories with
`repository_branches`:

```json
{
  "branches": ["main", "stable"],
  "repository_branches": {
    "legacy-app": ["master", "release"]
  }
}
```

Branches outside the selected list are not synchronized. You can also exclude
branches from that list using regex patterns in `exclude_branches`:

```json
{
  "organizations": [...],
  "repositories": [...],
  "branches": ["main", "master", "develop"],
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
      "backupLocation": true,
      "forcePush": true
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
   - Use `descriptionSyncHost` and `descriptionSyncRoot` for creation when configured, allowing the Git remote endpoint itself to remain restricted
4. **Archive functionality**: Repositories that exist only on the backup location are considered archived and won't be synced to other organizations
5. **All branches and tags**: Every branch and tag is pushed to the backup location when `--backup` is used
6. **Optional cgit description sync**: Set `descriptionSyncHost` and `descriptionSyncRoot` on a backup organization to mirror the canonical repository description into the bare repo `description` file used by cgit
7. **Optional force push**: Set `forcePush` to `true` on a backup organization to overwrite divergent branches and tags. This setting is rejected for non-backup organizations

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

GitSyncer can generate a comprehensive showcase of all your projects using AI (ollama `glm-5.3-flash:cloud` by default). This feature creates a formatted document with project summaries, statistics, and code snippets.

### How it works

1. **Repository Analysis**: GitSyncer analyzes all cloned repositories to extract:
   - Programming languages and their usage percentages
   - Commit history and development activity
   - Lines of code and documentation
   - License information
   - Latest release version and date
   - AI-assistance detection (looks for CLAUDE.md, GEMINI.md files)

2. **AI-Powered Summaries**: Uses AI (opencode, hexai, claude, or amp) to generate concise project descriptions that explain:
   - What the project does
   - Why it's useful
   - How it's implemented
   - Key features and architecture

3. **Automatic Features**:
   - Extracts README images (including SVG support)
   - Selects representative code snippets
   - Orders projects by recent activity
   - Tracks weekly rank history snapshots
   - Generates overall portfolio statistics
   - Caches summaries to avoid redundant AI calls

### Usage

```bash
# Generate showcase (uses cached summaries when available)
gitsyncer showcase

# Force regeneration of all summaries
gitsyncer showcase --force

# Generate showcase for one repository
gitsyncer showcase --repo gitsyncer

# Use a specific AI tool
gitsyncer showcase --ai-tool opencode
```

### Output

The showcase is generated in Gemini Gemtext format and includes:
- Overall statistics (total projects, commits, lines of code, languages)
- Release status breakdown (released vs experimental projects)
- AI-assistance statistics
- Individual project sections with:
  - Rank history in the title for `now`, `1w`, `2w`, `3w`, and `4w`
  - UTF-8 arrows (`↑`, `→`, `↓`) to show movement from older to newer positions
  - Language breakdown
  - Development metrics
  - Latest release information or experimental status
  - Project description
  - Code snippet example
  - Links to repositories

Weekly rank snapshots are written on full showcase runs (all repositories), including `manage batch-run`. Single-repository showcase updates (`--repo`) read existing history but do not write snapshots.

### Configuration

The showcase output defaults to `~/git/foo.zone-content/gemtext/about/showcase.gmi.tpl`. You can override the output directory with `showcase_output_dir`.

Forgejo links in project sections are derived from the configured Forgejo target's `forgejo_api_base` and `forgejo_owner`. For example, `https://code.f3s.buetow.org/api/v1` with owner `snonux` produces `https://code.f3s.buetow.org/snonux/<repo>`, keeping showcase links aligned with the sync destination. The legacy `showcase_cgit_host` key is accepted but ignored and may be removed from configurations.

You can override the branch used for showcase stats and cached code snippets on a per-repository basis with `showcase_stats_branches`. For example, `foo.zone` can use `content-gemtext` while the rest of the repos continue to use their current checkout branch.

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
1. Create repositories on all configured platforms
2. Add the repository name to your configuration file
3. Run `gitsyncer sync repo repo-name`
4. GitSyncer will:
   - Clone from the first organization
   - Push all branches to other organizations
   - Keep them in sync going forward

## Error Handling

- **Merge conflicts**: GitSyncer will detect conflicts and exit with an error message
- **Missing repositories**: Must be created manually on all platforms
- **Missing branches**: Automatically created on remotes that don't have them

## License

BSD 2-Clause License. See [LICENSE](LICENSE) file for details.
