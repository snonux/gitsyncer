#!/bin/bash

# Setup script for creating test git repositories
set -e

echo "Setting up test repositories..."

# Create test directory structure
TEST_DIR="$(cd "$(dirname "$0")" && pwd)"
REPOS_DIR="$TEST_DIR/repos"
mkdir -p "$REPOS_DIR"

# Create two bare repositories (simulating remote repos)
REPO1_DIR="$REPOS_DIR/org1"
REPO2_DIR="$REPOS_DIR/org2"

# Clean up if they exist
rm -rf "$REPO1_DIR" "$REPO2_DIR"
mkdir -p "$REPO1_DIR" "$REPO2_DIR"

# Initialize bare repositories
echo "Creating bare repository in org1..."
cd "$REPO1_DIR"
git init --bare test-repo.git

echo "Creating bare repository in org2..."
cd "$REPO2_DIR"
git init --bare test-repo.git

# Create a temporary working directory to add initial content
WORK_DIR="$REPOS_DIR/work"
rm -rf "$WORK_DIR"
mkdir -p "$WORK_DIR"

# Clone from org1 and add initial content
echo "Adding initial content..."
cd "$WORK_DIR"
git clone "$REPO1_DIR/test-repo.git"
cd test-repo

# Create initial files
echo "# Test Repository" > README.md
echo "This is a test repository for gitsyncer" >> README.md

mkdir -p src
echo "package main

import \"fmt\"

func main() {
    fmt.Println(\"Hello from test repo!\")
}" > src/main.go

# Create initial commit
git add .
git commit -m "Initial commit"

# Create develop branch
git checkout -b develop
echo "Development branch" > DEVELOP.md
git add DEVELOP.md
git commit -m "Add develop branch marker"

# Create feature branch
git checkout -b feature/test-feature
echo "Feature content" > feature.txt
git add feature.txt
git commit -m "Add feature"

# Push all branches to org1
git push origin main
git push origin develop
git push origin feature/test-feature

# Add org2 as remote and push only main branch initially
git remote add org2 "$REPO2_DIR/test-repo.git"
git checkout main
git push org2 main

# Clean up work directory
cd "$TEST_DIR"
rm -rf "$WORK_DIR"

# Create test config file
echo "Creating test configuration..."
cat > "$TEST_DIR/test-config.json" << EOF
{
  "organizations": [
    {
      "host": "file://$REPO1_DIR",
      "name": ""
    },
    {
      "host": "file://$REPO2_DIR",
      "name": ""
    }
  ]
}
EOF

echo "Test setup complete!"
echo ""
echo "Repository structure:"
echo "- org1/test-repo.git: Has main, develop, and feature/test-feature branches"
echo "- org2/test-repo.git: Has only main branch"
echo ""
echo "Test config file created at: $TEST_DIR/test-config.json"