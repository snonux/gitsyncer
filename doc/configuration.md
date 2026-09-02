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
  ],
  "showcase_output_dir": "~/git/foo.zone-content/gemtext/about",
  "showcase_stats_branches": {
    "foo.zone": "content-gemtext"
  },
  "sync_codeberg": true
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
- **codeberg_token** (string, optional): Codeberg personal access token
  - Only needed for Codeberg organizations
  - Can also be set via environment variable or file
- **forgejo_api_base** (string, optional): Gitea-compatible API root for a Forgejo target, such as `https://code.example/api/v1`
- **forgejo_owner** (string, required with `forgejo_api_base`): User or organization that owns the repositories
- **forgejo_owner_type** (`user` or `organization`, optional): Owner kind; defaults to `user` for backward compatibility
- **backupLocation** (boolean): Makes the destination a one-way backup; it is pushed but never fetched
- **optional** (boolean): Makes the organization a bidirectional sync peer whose Git fetch or push failures cause it to be skipped for the remainder of the run
- **forcePush** (boolean, optional): Force-update backup branches and tags

Forgejo credentials are read first from `FORGEJO_TOKEN`, then from
`~/.gitsyncer_forgejo_token`; surrounding whitespace is trimmed. The token needs
minimal scopes `write:repository` plus `write:user` for a user owner, or
`write:repository` plus `write:organization` for an organization owner.
For an organization owner, the token must belong to an interactive user that is
authorized to create repositories in the organization; an organization
pseudo-user cannot issue a usable token. Protect the token file with
`chmod 600 ~/.gitsyncer_forgejo_token`. Forgejo tokens are not accepted from
tracked JSON configuration. Forgejo creation is public and uninitialized.
Existing repositories must belong to the configured owner and must be public.
The Git SSH URL is formed as `<host>/<forgejo_owner>/<repo>.git`; do not
configure `descriptionSyncHost` or `descriptionSyncRoot` for Forgejo because
metadata is updated through the API.

Forgejo requires exactly one of `backupLocation: true` or `optional: true`.
Use `optional` for three-way GitHub ↔ Codeberg ↔ Forgejo synchronization.
Unlike a backup, an optional Forgejo peer is fetched and contributes branches
and commits to the merge. If Forgejo is offline, GitSyncer disables it for the
rest of that run and continues syncing the remaining hosts. A metadata API
failure is reported but does not disable an otherwise reachable Git-over-SSH
peer.

#### repositories (optional)
Array of repository names to sync. If empty, use `gitsyncer sync codeberg-to-github` or `gitsyncer sync github-to-codeberg` to discover repositories.

#### exclude_branches (optional)
Array of regex patterns for branches to exclude from synchronization.

#### skip_releases (optional)
Map of repository names to an array of tag names for which releases should not be created on any platform (GitHub and Codeberg). Useful to suppress auto-release for specific historical tags.

#### sync_codeberg (optional, default: false)
Opts in to Codeberg syncing. Codeberg is never synced unless this is explicitly set to `true`, even when a Codeberg organization is present in `organizations`. When `false`:

- `sync repo` / `sync all` do not push to or fetch from Codeberg remotes.
- `sync codeberg-to-github` and `sync github-to-codeberg` skip the Codeberg portion.
- `manage batch-run` and `sync bidirectional` skip Codeberg syncing (and no longer fail just because no Codeberg sync is configured).
- Repository descriptions are not synced to Codeberg.
- Releases are not created on Codeberg.
- `manage delete-repo` skips Codeberg organizations.
- The showcase only renders a "View on Codeberg" link for repositories that are actually synced to Codeberg.

Set `"sync_codeberg": true` to enable all of the above. A Codeberg organization can still be listed in `organizations` (e.g. to keep a token or to generate Codeberg links) without enabling syncing.

When `sync_codeberg` is `true` **and** `repositories` is non-empty, the `repositories` list acts as an allowlist: the discovery-based commands (`sync codeberg-to-github`, `sync github-to-codeberg`, `sync bidirectional`, and `manage batch-run`) only sync the repos in that list to/from Codeberg, rather than every public repo they discover. This prevents accidentally mirroring unintended repos to Codeberg. When `repositories` is empty, discovery mode is preserved and all public repos are synced.

Example:
```json
{
  "skip_releases": {
    "fapi": ["0.0.1", "0.0.2"],
    "another-repo": ["v1.0.0"]
  }
}
```

#### showcase_stats_branches (optional)
Map of repository names to the branch that should be used when generating showcase statistics and cached code snippets. This is useful when the primary content for a repo lives on a non-default branch.

Example:
```json
{
  "showcase_stats_branches": {
    "foo.zone": "content-gemtext"
  }
}
```

#### showcase_output_dir (optional)
Directory where showcase files are written (`showcase.gmi.tpl`, `showcase-rank-history.svg`, and extracted images).

Default: `~/git/foo.zone-content/gemtext/about`

Example:
```json
{
  "showcase_output_dir": "~/git/foo.zone-content/gemtext/about"
}
```

#### Forgejo showcase links
Showcase repository links require a Forgejo target and are derived from its `forgejo_api_base` and `forgejo_owner`. For example, API base `https://code.example/api/v1` with owner `snonux` produces `https://code.example/snonux/<repo>`. This avoids a separate host setting that can drift from the Forgejo configuration.

The legacy `showcase_cgit_host` key remains accepted for configuration compatibility, but it is ignored. Remove it when updating existing configurations.

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
  ],
  "showcase_output_dir": "~/git/foo.zone-content/gemtext/about",
  "showcase_stats_branches": {
    "foo.zone": "content-gemtext"
  }
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
- Creating repositories (`sync ... --create-repos`)
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
gitsyncer sync all
```

#### Method 3: Token File
```bash
echo "ghp_xxxxxxxxxxxx" > ~/.gitsyncer_github_token
chmod 600 ~/.gitsyncer_github_token
```

### Testing Token

```bash
gitsyncer test github-token
```

## Codeberg Token Configuration

Codeberg tokens are required for:
- Creating repositories (`sync ... --create-repos`)
- Listing private repositories

### Token Sources (in order of precedence)

1. **Configuration file**: `codeberg_token` field in organization object
2. **Environment variable**: `CODEBERG_TOKEN`
3. **Token file**: `~/.gitsyncer_codeberg_token`

### Creating a Codeberg Token

1. Go to Codeberg Settings → Applications → Manage Access Tokens
2. Click "Generate New Token"
3. Select scopes:
   - `repository` (full control of repositories)
4. Save the token securely

### Setting the Token

#### Method 1: Configuration File
```json
{
  "organizations": [
    {
      "host": "git@codeberg.org",
      "name": "myorg",
      "codeberg_token": "xxxxxxxxxxxx"
    }
  ]
}
```

#### Method 2: Environment Variable
```bash
export CODEBERG_TOKEN="xxxxxxxxxxxx"
gitsyncer sync all
```

#### Method 3: Token File
```bash
echo "xxxxxxxxxxxx" > ~/.gitsyncer_codeberg_token
chmod 600 ~/.gitsyncer_codeberg_token
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
gitsyncer sync repo repo-name
# Output will show excluded branches and patterns
```

## Best Practices

### 1. Start Simple
Begin with a minimal configuration and add complexity as needed.

### 2. Use Dry Run
Test your configuration with `--dry-run` before actual synchronization:
```bash
gitsyncer sync all --dry-run
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
$ gitsyncer sync repo myrepo
Error loading configuration: failed to read config file: open /home/user/.config/gitsyncer/config.json: no such file or directory

Please create a configuration file with your organizations and repositories.
See 'gitsyncer help' for more information.
```

**Solution**: Create a configuration file in one of the suggested locations.

### Invalid JSON
```bash
$ gitsyncer list orgs
Error loading configuration: failed to parse config: invalid character '}' looking for beginning of value

Please create a configuration file with your organizations and repositories.
See 'gitsyncer help' for more information.
```

**Solution**: Validate your JSON syntax using a JSON validator.

### No Organizations Configured
```bash
$ gitsyncer sync repo myrepo
Error loading configuration: invalid configuration: no organizations configured

Please create a configuration file with your organizations and repositories.
See 'gitsyncer help' for more information.
```

**Solution**: Add at least one organization to the `organizations` array.

### Token Issues
```bash
$ gitsyncer test github-token
ERROR: Token test failed: authentication failed (401)
```

**Solution**: 
- Verify token is correct and not expired
- Check token has required permissions
- Ensure no extra whitespace in token
