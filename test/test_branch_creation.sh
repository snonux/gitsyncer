#!/bin/bash

# Test script for branch creation functionality
set -e

echo "Testing automatic branch creation on remotes..."

TEST_DIR="$(cd "$(dirname "$0")" && pwd)"
REPOS_DIR="$TEST_DIR/repos"
mkdir -p "$REPOS_DIR"

# Clean up if they exist
rm -rf "$REPOS_DIR/org1" "$REPOS_DIR/org2"
mkdir -p "$REPOS_DIR/org1" "$REPOS_DIR/org2"

# Create a repository in org1 with multiple branches
echo "Creating repository in org1 with branches..."
cd "$REPOS_DIR/org1"
git init --bare test-branch-repo.git

# Create only the same repository in org2 but with just main branch
echo "Creating repository in org2..."
cd "$REPOS_DIR/org2"
git init --bare test-branch-repo.git

# Add content and branches to org1
WORK_DIR="$REPOS_DIR/work"
rm -rf "$WORK_DIR"
mkdir -p "$WORK_DIR"

echo "Creating branches in org1..."
cd "$WORK_DIR"
git clone "$REPOS_DIR/org1/test-branch-repo.git"
cd test-branch-repo

# Create main branch
echo "# Test Repo" > README.md
git add README.md
git commit -m "Initial commit"
git push origin main

# Create feature branch
git checkout -b feature/new-feature
echo "New feature" > feature.txt
git add feature.txt
git commit -m "Add new feature"
git push origin feature/new-feature

# Create hotfix branch
git checkout -b hotfix/urgent-fix
echo "Urgent fix" > hotfix.txt
git add hotfix.txt
git commit -m "Apply urgent fix"
git push origin hotfix/urgent-fix

# Push only main to org2
git checkout main
git remote add org2 "$REPOS_DIR/org2/test-branch-repo.git"
git push org2 main

# Clean up work directory
cd "$TEST_DIR"
rm -rf "$WORK_DIR"

# Create test config
cat > "$TEST_DIR/branch-test-config.json" << EOF
{
  "organizations": [
    {
      "host": "file://$REPOS_DIR/org1",
      "name": ""
    },
    {
      "host": "file://$REPOS_DIR/org2",
      "name": ""
    }
  ],
  "repositories": [
    "test-branch-repo"
  ]
}
EOF

echo ""
echo "Test setup complete!"
echo ""
echo "Initial state:"
echo "- org1 has: main, feature/new-feature, hotfix/urgent-fix"
echo "- org2 has: main only"
echo ""
echo "Run sync to see branch creation:"
echo "  ./gitsyncer --config test/branch-test-config.json --sync test-branch-repo"