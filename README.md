# GitSyncer

GitSyncer is a tool for synchronizing git repositories between multiple organizations (e.g., GitHub and Codeberg). It automatically keeps all branches in sync across different git hosting platforms.

## Features

- Sync repositories between multiple git organizations
- Automatic branch creation on remotes that don't have them
- Batch sync multiple repositories with a single command
- Sync all public repositories from Codeberg to other platforms
- Merge conflict detection
- Never deletes branches (only adds/updates)

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
```

### Sync all configured repositories
```bash
./gitsyncer --sync-all
```

### Sync all public Codeberg repositories
```bash
# Dry run - see what would be synced
./gitsyncer --sync-codeberg-public --dry-run

# Actually sync all public repos
./gitsyncer --sync-codeberg-public
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

## How It Works

1. GitSyncer clones the repository from the first configured organization
2. Adds all other organizations as git remotes
3. For each branch:
   - Fetches from all remotes
   - Merges changes from remotes that have the branch
   - Pushes to all remotes (creating branches if needed)

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

[Add your license here]