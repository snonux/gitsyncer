#!/bin/bash

# Test script for multiple repository sync functionality
set -e

echo "Setting up test for multiple repository sync..."

TEST_DIR="$(cd "$(dirname "$0")" && pwd)"
REPOS_DIR="$TEST_DIR/repos"
mkdir -p "$REPOS_DIR"

# Create two bare repositories in each organization
ORG1_DIR="$REPOS_DIR/org1"
ORG2_DIR="$REPOS_DIR/org2"

# Clean up if they exist
rm -rf "$ORG1_DIR" "$ORG2_DIR"
mkdir -p "$ORG1_DIR" "$ORG2_DIR"

# Create repo1 and repo2 in org1
echo "Creating repositories in org1..."
cd "$ORG1_DIR"
git init --bare repo1.git
git init --bare repo2.git

# Create only repo1 in org2 (repo2 will be synced)
echo "Creating repository in org2..."
cd "$ORG2_DIR"
git init --bare repo1.git

# Add initial content to repo1 in org1
WORK_DIR="$REPOS_DIR/work"
rm -rf "$WORK_DIR"
mkdir -p "$WORK_DIR"

echo "Adding content to repo1..."
cd "$WORK_DIR"
git clone "$ORG1_DIR/repo1.git"
cd repo1
echo "# Repo 1" > README.md
echo "This is repository 1" >> README.md
git add README.md
git commit -m "Initial commit for repo1"
git push origin main

# Add initial content to repo2 in org1
echo "Adding content to repo2..."
cd "$WORK_DIR"
git clone "$ORG1_DIR/repo2.git"
cd repo2
echo "# Repo 2" > README.md
echo "This is repository 2" >> README.md
mkdir src
echo "console.log('Hello from repo2');" > src/index.js
git add .
git commit -m "Initial commit for repo2"
git push origin main

# Create feature branch in repo2
git checkout -b feature/awesome
echo "Awesome feature" > feature.txt
git add feature.txt
git commit -m "Add awesome feature"
git push origin feature/awesome

# Clean up work directory
cd "$TEST_DIR"
rm -rf "$WORK_DIR"

# Create test config with repositories list
echo "Creating test configuration with repositories..."
cat > "$TEST_DIR/multi-repo-config.json" << EOF
{
  "organizations": [
    {
      "host": "file://$ORG1_DIR",
      "name": ""
    },
    {
      "host": "file://$ORG2_DIR",
      "name": ""
    }
  ],
  "repositories": [
    "repo1",
    "repo2"
  ]
}
EOF

echo ""
echo "Test setup complete!"
echo ""
echo "Repository structure:"
echo "- org1/repo1.git: Has initial content"
echo "- org1/repo2.git: Has main and feature/awesome branches"
echo "- org2/repo1.git: Empty (will receive updates)"
echo "- org2/repo2.git: Does not exist (will be created)"
echo ""
echo "Test with:"
echo "  ./gitsyncer --config test/multi-repo-config.json --list-repos"
echo "  ./gitsyncer --config test/multi-repo-config.json --sync-all"