#!/bin/bash

# Test script to demonstrate merge conflict handling
set -e

echo "Testing merge conflict detection..."

TEST_DIR="$(cd "$(dirname "$0")" && pwd)"
REPOS_DIR="$TEST_DIR/repos"
WORK_DIR="$REPOS_DIR/conflict-work"

# Clean up and create work directories
rm -rf "$WORK_DIR"
mkdir -p "$WORK_DIR"

# Clone both repos to create conflicting changes
echo "Creating conflicting changes in org1..."
cd "$WORK_DIR"
git clone "$REPOS_DIR/org1/test-repo.git" work1
cd work1
echo "Change from org1" > conflict.txt
git add conflict.txt
git commit -m "Add conflict.txt from org1"
git push origin main

echo "Creating conflicting changes in org2..."
cd "$WORK_DIR"
git clone "$REPOS_DIR/org2/test-repo.git" work2
cd work2
echo "Different change from org2" > conflict.txt
git add conflict.txt
git commit -m "Add conflict.txt from org2"
git push origin main

# Clean up work directories
cd "$TEST_DIR"
rm -rf "$WORK_DIR"

echo ""
echo "Conflicting changes created!"
echo "Now run gitsyncer to see conflict detection:"
echo "  ./gitsyncer --config test/test-config.json --sync test-repo --work-dir test/work-conflict"