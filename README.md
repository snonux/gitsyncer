# GitSyncer

GitSyncer is a tool for synchronizing git repositories between multiple organizations (e.g., GitHub and Codeberg). It automatically keeps all branches in sync across different git hosting platforms.

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

## Installation

```bash
go build -o gitsyncer ./cmd/gitsyncer
```

## Configuration

Create a `gitsyncer.json` file:

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

### Sync a single repository
```bash
./gitsyncer --sync repo-name

# Include backup locations
./gitsyncer --sync repo-name --backup
```

### Sync all configured repositories
```bash
./gitsyncer --sync-all

# Include backup locations
./gitsyncer --sync-all --backup
```

### Sync all public Codeberg repositories to GitHub
```bash
# Dry run - see what would be synced
./gitsyncer --sync-codeberg-public --dry-run

# Actually sync all public repos
./gitsyncer --sync-codeberg-public

# With automatic GitHub repo creation
./gitsyncer --sync-codeberg-public --create-github-repos
```

### Sync all public GitHub repositories to Codeberg
```bash
# Dry run - see what would be synced
./gitsyncer --sync-github-public --dry-run

# Actually sync all public repos
./gitsyncer --sync-github-public
```

### Full bidirectional sync
```bash
# Sync all public repos from both Codeberg and GitHub
# This enables --sync-codeberg-public --sync-github-public 
# --create-github-repos --create-codeberg-repos
./gitsyncer --full

# With dry run to see what would happen
./gitsyncer --full --dry-run
```

### List configured organizations
```bash
./gitsyncer --list-orgs
```

### List configured repositories  
```bash
./gitsyncer --list-repos
```

### Show version
```bash
./gitsyncer --version
```

### The --backup Flag

The `--backup` flag enables syncing to backup locations configured in your `gitsyncer.json`. This is particularly useful when:
- Your backup server might be offline (e.g., home NAS)
- You want to control when backups happen
- You need to separate regular syncing from backup operations

Without `--backup`: GitSyncer only syncs between primary git hosts (GitHub, Codeberg, etc.)
With `--backup`: GitSyncer also pushes to backup locations marked with `"backupLocation": true`

```bash
# Regular sync (backup locations ignored)
./gitsyncer --sync myrepo

# Sync with backup enabled
./gitsyncer --sync myrepo --backup
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
# Configure your gitsyncer.json with an SSH backup location
# Backup locations are DISABLED by default to handle offline servers

# Sync without backup (default behavior)
./gitsyncer --sync myrepo

# Sync WITH backup enabled
./gitsyncer --sync myrepo --backup

# Sync all repositories with backup
./gitsyncer --sync-all --backup

# Full sync with backup
./gitsyncer --full --backup
```

The backup location path format is: `user@host:path/REPONAME.git`
- `user@host`: SSH connection string
- `path`: Base directory for repositories
- `REPONAME.git`: Automatically appended repository name

**Note**: The `--backup` flag is required to sync to backup locations. This allows GitSyncer to work normally even when backup servers are offline or unreachable.

## Example Workflows

### Sync specific repositories
1. Create repositories on all platforms (GitHub, Codeberg, etc.)
2. Add the repository name to your `gitsyncer.json`
3. Run `./gitsyncer --sync repo-name`
4. GitSyncer will:
   - Clone from the first organization
   - Push all branches to other organizations
   - Keep them in sync going forward

### Sync all public Codeberg repositories
1. Ensure Codeberg is in your organizations list
2. Run `./gitsyncer --sync-codeberg-public`
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