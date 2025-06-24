# GitSyncer Configuration Guide

## Overview

GitSyncer uses a JSON configuration file to define organizations, repositories, and sync behavior. The configuration file can be placed in several locations or specified via command line.

## Configuration File Locations

GitSyncer looks for configuration files in the following order:

1. Path specified by `--config` flag
2. `./gitsyncer.json` (current directory)
3. `~/.config/gitsyncer/config.json`
4. `~/.gitsyncer.json`

## Configuration Structure

### Basic Structure

```json
{
  "organizations": [
    {
      "host": "git@github.com",
      "name": "myorg",
      "github_token": "ghp_xxxxxxxxxxxx"
    },
    {
      "host": "git@codeberg.org",
      "name": "myorg"
    }
  ],
  "repositories": [
    "repo1",
    "repo2"
  ],
  "exclude_branches": [
    "^temp-",
    "-wip$"
  ]
}
```

### Configuration Fields

#### organizations (required)
Array of organization objects. At least one organization must be configured.

##### Organization Object
- **host** (string, required): Git host URL
  - Format: `git@hostname` for SSH
  - Format: `file:///path/to/repos` for local repositories
  - Examples: `git@github.com`, `git@codeberg.org`, `git@gitlab.com`
- **name** (string, required): Organization or username
- **github_token** (string, optional): GitHub personal access token
  - Only needed for GitHub organizations
  - Can also be set via environment variable or file

#### repositories (optional)
Array of repository names to sync. If empty, use `--sync-codeberg-public` or `--sync-github-public` to discover repositories.

#### exclude_branches (optional)
Array of regex patterns for branches to exclude from synchronization.

## Examples

### Minimal Configuration

Sync between GitHub and Codeberg:

```json
{
  "organizations": [
    {"host": "git@github.com", "name": "myusername"},
    {"host": "git@codeberg.org", "name": "myusername"}
  ]
}
```

### With Specific Repositories

```json
{
  "organizations": [
    {"host": "git@github.com", "name": "myorg"},
    {"host": "git@codeberg.org", "name": "myorg"}
  ],
  "repositories": [
    "project1",
    "project2",
    "project3"
  ]
}
```

### With Branch Filtering

```json
{
  "organizations": [
    {"host": "git@github.com", "name": "myorg"},
    {"host": "git@codeberg.org", "name": "myorg"}
  ],
  "repositories": ["myproject"],
  "exclude_branches": [
    "^feature/experimental-",
    "^temp-",
    "-wip$",
    "^old-"
  ]
}
```

### Multiple Organizations

```json
{
  "organizations": [
    {"host": "git@github.com", "name": "personal"},
    {"host": "git@github.com", "name": "work"},
    {"host": "git@codeberg.org", "name": "personal"},
    {"host": "git@gitlab.com", "name": "personal"}
  ],
  "repositories": ["shared-project"]
}
```

### Local Mirror Configuration

```json
{
  "organizations": [
    {"host": "git@github.com", "name": "myorg"},
    {"host": "file:///home/user/git-mirror", "name": "myorg"}
  ],
  "repositories": ["important-project"]
}
```

## GitHub Token Configuration

GitHub tokens are required for:
- Creating repositories (`--create-github-repos`)
- Listing private repositories
- Higher API rate limits

### Token Sources (in order of precedence)

1. **Configuration file**: `github_token` field in organization object
2. **Environment variable**: `GITHUB_TOKEN`
3. **Token file**: `~/.gitsyncer_github_token`

### Creating a GitHub Token

1. Go to GitHub Settings → Developer settings → Personal access tokens
2. Click "Generate new token (classic)"
3. Select scopes:
   - `repo` (full control of private repositories)
   - `read:org` (read organization membership)
4. Save the token securely

### Setting the Token

#### Method 1: Configuration File
```json
{
  "organizations": [
    {
      "host": "git@github.com",
      "name": "myorg",
      "github_token": "ghp_xxxxxxxxxxxx"
    }
  ]
}
```

#### Method 2: Environment Variable
```bash
export GITHUB_TOKEN="ghp_xxxxxxxxxxxx"
gitsyncer --sync-all
```

#### Method 3: Token File
```bash
echo "ghp_xxxxxxxxxxxx" > ~/.gitsyncer_github_token
chmod 600 ~/.gitsyncer_github_token
```

### Testing Token

```bash
gitsyncer --test-github-token
```

## Branch Exclusion Patterns

The `exclude_branches` field accepts regular expressions to filter out branches from synchronization.

### Common Patterns

- `^temp-` - Exclude branches starting with "temp-"
- `-wip$` - Exclude branches ending with "-wip"
- `^feature/experimental-` - Exclude experimental feature branches
- `^(dev|development)$` - Exclude specific branch names
- `^release/\d+\.` - Exclude release branches (e.g., release/1.x)

### Pattern Testing

To see which branches are excluded:
```bash
gitsyncer --sync repo-name
# Output will show excluded branches and patterns
```

## Best Practices

### 1. Start Simple
Begin with a minimal configuration and add complexity as needed.

### 2. Use Dry Run
Test your configuration with `--dry-run` before actual synchronization:
```bash
gitsyncer --sync-all --dry-run
```

### 3. Secure Your Tokens
- Never commit tokens to version control
- Use environment variables or token files for sensitive data
- Restrict token permissions to minimum required

### 4. Regular Expressions
- Test regex patterns before adding to configuration
- Use online regex testers to validate patterns
- Document complex patterns with comments

### 5. Organization Naming
- Keep organization names consistent across platforms
- Use the same name on GitHub and Codeberg when possible

## Troubleshooting

### Configuration Not Found
```bash
$ gitsyncer --sync myrepo
No configuration file found. Please create one of:
  - ./gitsyncer.json
  - /home/user/.config/gitsyncer/config.json
  - /home/user/.gitsyncer.json
```

**Solution**: Create a configuration file in one of the suggested locations.

### Invalid JSON
```bash
$ gitsyncer --list-orgs
Failed to load configuration: invalid character '}' looking for beginning of object key string
```

**Solution**: Validate your JSON syntax using a JSON validator.

### No Organizations Configured
```bash
$ gitsyncer --sync myrepo
Configuration must have at least one organization
```

**Solution**: Add at least one organization to the `organizations` array.

### Token Issues
```bash
$ gitsyncer --test-github-token
ERROR: Token test failed: authentication failed (401)
```

**Solution**: 
- Verify token is correct and not expired
- Check token has required permissions
- Ensure no extra whitespace in token