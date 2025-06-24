# GitSyncer Usage Examples

This guide provides practical examples of using GitSyncer for various scenarios.

## Table of Contents

- [Basic Operations](#basic-operations)
- [Repository Discovery](#repository-discovery)
- [Advanced Synchronization](#advanced-synchronization)
- [Automation Examples](#automation-examples)
- [Troubleshooting Scenarios](#troubleshooting-scenarios)

## Basic Operations

### Sync a Single Repository

```bash
# Sync a specific repository
gitsyncer --sync my-project

# Sync with custom working directory
gitsyncer --sync my-project --work-dir /tmp/gitsyncer-work

# Dry run to preview changes
gitsyncer --sync my-project --dry-run
```

### Sync All Configured Repositories

```bash
# Sync all repositories in config
gitsyncer --sync-all

# Create missing GitHub repos automatically
gitsyncer --sync-all --create-github-repos
```

### List Operations

```bash
# List configured organizations
gitsyncer --list-orgs

# List configured repositories
gitsyncer --list-repos

# Show version
gitsyncer --version
```

## Repository Discovery

### Sync All Public Codeberg Repositories to GitHub

```bash
# Discover and sync all public repos from Codeberg
gitsyncer --sync-codeberg-public

# Also create repos on GitHub if they don't exist
gitsyncer --sync-codeberg-public --create-github-repos

# Dry run to see what would be synced
gitsyncer --sync-codeberg-public --dry-run
```

### Sync All Public GitHub Repositories to Codeberg

```bash
# Discover and sync all public repos from GitHub
gitsyncer --sync-github-public

# Note: Codeberg repos must already exist
gitsyncer --sync-github-public
```

### Full Bidirectional Sync

```bash
# Sync all public repos in both directions
gitsyncer --full

# Equivalent to:
# gitsyncer --sync-codeberg-public --sync-github-public --create-github-repos
```

## Advanced Synchronization

### Working with Branch Filters

Configuration with branch exclusions:
```json
{
  "organizations": [
    {"host": "git@github.com", "name": "myorg"},
    {"host": "git@codeberg.org", "name": "myorg"}
  ],
  "repositories": ["my-project"],
  "exclude_branches": [
    "^temp-",
    "^feature/experimental-",
    "-wip$"
  ]
}
```

Output shows excluded branches:
```bash
$ gitsyncer --sync my-project

🚫 Excluded 3 branches based on patterns:
   Patterns: '^temp-', '^feature/experimental-', '-wip$'
   Excluded branches:
   - temp-fix
   - feature/experimental-ai
   - feature-wip
```

### Handling Merge Conflicts

When conflicts occur:
```bash
$ gitsyncer --sync my-project

ERROR: repository has unresolved merge conflicts
Please resolve conflicts in: /home/user/.gitsyncer-work/my-project
Or delete the directory to start fresh: rm -rf /home/user/.gitsyncer-work/my-project
```

Resolution options:
```bash
# Option 1: Manually resolve conflicts
cd /home/user/.gitsyncer-work/my-project
git status
# Fix conflicts
git add .
git commit -m "Resolved conflicts"
cd -
gitsyncer --sync my-project

# Option 2: Start fresh
rm -rf /home/user/.gitsyncer-work/my-project
gitsyncer --sync my-project
```

### Abandoned Branch Detection

GitSyncer detects branches inactive for 6+ months:
```bash
$ gitsyncer --sync-all

[1/3] Syncing project1...
Repository project1 synchronized successfully!

🔍 Abandoned branches in project1:
   Main branch (main) is abandoned - last commit: 2023-01-15
   Other abandoned branches:
   - feature/old-feature (2023-02-01) - abandoned: main branch is abandoned
   - bugfix/old-fix (2023-03-15) - abandoned: main branch is abandoned

=== Summary of Abandoned Branches ===
Total repositories with abandoned branches: 1

Repository: project1
  - feature/old-feature
  - bugfix/old-fix
```

## Automation Examples

### Cron Job for Regular Sync

```bash
# Add to crontab (crontab -e)
# Sync all repos every 6 hours
0 */6 * * * /usr/local/bin/gitsyncer --sync-all --config /home/user/.gitsyncer.json >> /var/log/gitsyncer.log 2>&1

# Sync public repos daily at 2 AM
0 2 * * * /usr/local/bin/gitsyncer --full >> /var/log/gitsyncer-public.log 2>&1
```

### Shell Script Wrapper

```bash
#!/bin/bash
# sync-repos.sh

set -e

CONFIG_FILE="$HOME/.config/gitsyncer/config.json"
LOG_FILE="$HOME/.gitsyncer/sync.log"

echo "Starting sync at $(date)" >> "$LOG_FILE"

# Test GitHub token first
if ! gitsyncer --test-github-token; then
    echo "GitHub token test failed" >> "$LOG_FILE"
    exit 1
fi

# Sync all repos
if gitsyncer --sync-all --config "$CONFIG_FILE" >> "$LOG_FILE" 2>&1; then
    echo "Sync completed successfully at $(date)" >> "$LOG_FILE"
else
    echo "Sync failed at $(date)" >> "$LOG_FILE"
    exit 1
fi
```

### CI/CD Integration

GitHub Actions example:
```yaml
name: Sync Repositories

on:
  schedule:
    - cron: '0 */6 * * *'  # Every 6 hours
  workflow_dispatch:  # Manual trigger

jobs:
  sync:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Install GitSyncer
        run: |
          wget https://github.com/yourusername/gitsyncer/releases/latest/download/gitsyncer-linux-amd64
          chmod +x gitsyncer-linux-amd64
          sudo mv gitsyncer-linux-amd64 /usr/local/bin/gitsyncer
      
      - name: Create config
        run: |
          cat > gitsyncer.json << EOF
          {
            "organizations": [
              {"host": "git@github.com", "name": "${{ github.repository_owner }}"},
              {"host": "git@codeberg.org", "name": "${{ secrets.CODEBERG_ORG }}"}
            ]
          }
          EOF
      
      - name: Sync repositories
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: gitsyncer --sync-all --create-github-repos
```

## Troubleshooting Scenarios

### Testing GitHub Authentication

```bash
# Test token is valid
$ gitsyncer --test-github-token
Testing GitHub token authentication...
  Loaded token from env var (length: 40)
  Checking URL: https://api.github.com/repos/myorg/gitsyncer
  Token present: true (length: 40)
SUCCESS: Token is valid! Repository check returned: true
```

### Debugging Sync Issues

```bash
# Check git status in work directory
cd ~/.gitsyncer-work/my-project
git status
git remote -v
git branch -a

# Check for stashed changes
git stash list

# View recent operations
git reflog
```

### Repository Not Found

```bash
$ gitsyncer --sync nonexistent-repo
ERROR: Failed to clone from any organization
```

Solutions:
1. Verify repository exists on at least one platform
2. Check repository name spelling
3. For private repos, ensure proper authentication

### Working Directory Issues

```bash
# Permission denied
$ gitsyncer --sync my-project --work-dir /root/work
ERROR: failed to create work directory: permission denied

# Solution: Use accessible directory
gitsyncer --sync my-project --work-dir ~/gitsyncer-work

# Disk space issues
$ gitsyncer --sync large-repo
ERROR: write error: no space left on device

# Solution: Clean up or use different disk
df -h
rm -rf ~/.gitsyncer-work/old-repo
gitsyncer --sync large-repo --work-dir /mnt/storage/gitsyncer
```

### Network and Connectivity

```bash
# SSH key issues
$ gitsyncer --sync my-project
ERROR: git@github.com: Permission denied (publickey)

# Solution: Add SSH key to agent
ssh-add ~/.ssh/id_rsa

# Firewall/proxy issues
$ gitsyncer --sync my-project
ERROR: Failed to connect to github.com port 22: Connection timed out

# Solution: Use HTTPS URLs in config
{
  "organizations": [
    {"host": "https://github.com", "name": "myorg"},
    {"host": "https://codeberg.org", "name": "myorg"}
  ]
}
```

## Best Practices

### 1. Start with Dry Run
Always test with `--dry-run` first:
```bash
gitsyncer --sync-all --dry-run
```

### 2. Use Specific Working Directories
Organize syncs by project or purpose:
```bash
gitsyncer --sync personal-projects --work-dir ~/sync/personal
gitsyncer --sync work-projects --work-dir ~/sync/work
```

### 3. Monitor Sync Operations
Keep logs for troubleshooting:
```bash
gitsyncer --sync-all 2>&1 | tee -a ~/gitsyncer.log
```

### 4. Regular Maintenance
Clean up old working directories:
```bash
# Remove repos no longer in config
cd ~/.gitsyncer-work
ls -la
rm -rf old-project-name
```

### 5. Handle Secrets Securely
Never put tokens in scripts directly:
```bash
# Bad
gitsyncer --config config-with-token.json

# Good
export GITHUB_TOKEN="$(pass show github/token)"
gitsyncer --sync-all
```