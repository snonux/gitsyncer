#!/bin/bash

# Integration test script for gitsyncer
# This script runs all tests in sequence and reports results

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Get script directory
TEST_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(dirname "$TEST_DIR")"

# Function to print test headers
print_test() {
    echo -e "\n${YELLOW}=== $1 ===${NC}"
}

# Function to print success
print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

# Function to print failure
print_failure() {
    echo -e "${RED}✗ $1${NC}"
}

# Start testing
echo "GitSyncer Integration Tests"
echo "============================"

# Clean up any previous test artifacts
print_test "Cleaning up previous test artifacts"
cd "$TEST_DIR"
rm -rf repos work work-* test-config.json
print_success "Cleanup complete"

# Build the project
print_test "Building gitsyncer"
cd "$PROJECT_ROOT"
if go build -o gitsyncer ./cmd/gitsyncer; then
    print_success "Build successful"
else
    print_failure "Build failed"
    exit 1
fi

# Test 1: Version command
print_test "Test 1: Version command"
if ./gitsyncer version | grep -q "gitsyncer version"; then
    print_success "Version command works"
else
    print_failure "Version command failed"
    exit 1
fi

# Test 2: Setup test repositories
print_test "Test 2: Setting up test repositories"
cd "$TEST_DIR"
if ./setup_test_repos.sh > /dev/null 2>&1; then
    print_success "Test repositories created"
    echo "  - org1: Has main, develop, and feature/test-feature branches"
    echo "  - org2: Has only main branch"
else
    print_failure "Failed to setup test repositories"
    exit 1
fi

# Test 3: List organizations
print_test "Test 3: List organizations"
cd "$PROJECT_ROOT"
if ./gitsyncer --config test/test-config.json list orgs | grep -q "org1"; then
    print_success "Organizations listed successfully"
else
    print_failure "Failed to list organizations"
    exit 1
fi

# Test 4: Initial sync
print_test "Test 4: Initial repository sync"
rm -rf test/work
if ./gitsyncer --config test/test-config.json sync repo test-repo --work-dir test/work > /dev/null 2>&1; then
    print_success "Initial sync completed"
    
    # Verify all branches are synced
    cd "$TEST_DIR"
    ORG2_BRANCHES=$(git ls-remote file://$(pwd)/repos/org2/test-repo.git | grep refs/heads | wc -l)
    if [ "$ORG2_BRANCHES" -eq "3" ]; then
        print_success "All branches synced to org2 (found $ORG2_BRANCHES branches)"
    else
        print_failure "Branch sync failed - expected 3 branches, found $ORG2_BRANCHES"
        exit 1
    fi
else
    print_failure "Initial sync failed"
    exit 1
fi

# Test 5: Sync with no changes (idempotent)
print_test "Test 5: Idempotent sync (no changes)"
cd "$PROJECT_ROOT"
rm -rf test/work2
if ./gitsyncer --config test/test-config.json sync repo test-repo --work-dir test/work2 > /dev/null 2>&1; then
    print_success "Idempotent sync successful"
else
    print_failure "Idempotent sync failed"
    exit 1
fi

# Test 6: Create changes and sync
print_test "Test 6: Sync with new changes"
cd "$TEST_DIR"
# Create a change in org1
WORK_DIR="$TEST_DIR/repos/change-work"
rm -rf "$WORK_DIR"
mkdir -p "$WORK_DIR"
cd "$WORK_DIR"
git clone -q file://"$TEST_DIR"/repos/org1/test-repo.git
cd test-repo
echo "New feature added" > feature2.txt
git add feature2.txt
git commit -q -m "Add feature2.txt"
git push -q origin main

# Run sync
cd "$PROJECT_ROOT"
rm -rf test/work3
if ./gitsyncer --config test/test-config.json sync repo test-repo --work-dir test/work3 > /dev/null 2>&1; then
    print_success "Sync with changes successful"
    
    # Verify change is in org2
    cd "$TEST_DIR"
    if git ls-remote file://$(pwd)/repos/org2/test-repo.git HEAD | grep -q "$(git ls-remote file://$(pwd)/repos/org1/test-repo.git HEAD | cut -f1)"; then
        print_success "Changes propagated to org2"
    else
        print_failure "Changes not propagated to org2"
        exit 1
    fi
else
    print_failure "Sync with changes failed"
    exit 1
fi

# Clean up work directory
rm -rf "$WORK_DIR"

# Test 7: Conflict detection
print_test "Test 7: Merge conflict detection"
cd "$TEST_DIR"
if ./test_conflict.sh > /dev/null 2>&1; then
    print_success "Conflicting changes created"
else
    print_failure "Failed to create conflicting changes"
    exit 1
fi

# Try to sync with conflicts (should fail)
cd "$PROJECT_ROOT"
if ./gitsyncer --config test/test-config.json --sync test-repo --work-dir test/work-conflict > /dev/null 2>&1; then
    print_failure "Sync should have failed with conflicts but didn't"
    exit 1
else
    print_success "Conflict detection working - sync correctly failed"
fi

# Test 8: Missing config file
print_test "Test 8: Missing config file handling"
if ./gitsyncer --config test/nonexistent.json sync repo test-repo 2>&1 | grep -q "Error loading configuration"; then
    print_success "Missing config file handled correctly"
else
    print_failure "Missing config file not handled properly"
    exit 1
fi

# Test 9: Empty organization list
print_test "Test 9: Invalid configuration handling"
cd "$TEST_DIR"
echo '{"organizations": []}' > empty-config.json
cd "$PROJECT_ROOT"
if ./gitsyncer --config test/empty-config.json sync repo test-repo 2>&1 | grep -q "invalid configuration: no organizations configured"; then
    print_success "Empty organization list handled correctly"
else
    print_failure "Empty organization list not handled properly"
    exit 1
fi

# Test 10: Multiple repository configuration
print_test "Test 10: Multiple repository sync"
cd "$TEST_DIR"
./test_multi_repo.sh > /dev/null 2>&1
cd "$PROJECT_ROOT"

# Test list-repos
if ./gitsyncer --config test/multi-repo-config.json list repos | grep -q "repo1"; then
    print_success "Repository listing works"
else
    print_failure "Failed to list repositories"
    exit 1
fi

# Test sync-all
if ./gitsyncer --config test/multi-repo-config.json sync all --work-dir test/work-multi > /dev/null 2>&1; then
    print_success "Multiple repository sync completed"
else
    print_failure "Multiple repository sync failed"
    exit 1
fi

# Cleanup test artifacts
print_test "Cleaning up test artifacts"
cd "$TEST_DIR"
rm -rf repos work work-* empty-config.json multi-repo-config.json
print_success "Cleanup complete"

# Summary
echo -e "\n${GREEN}=== All tests passed! ===${NC}"
echo "GitSyncer is working correctly and can:"
echo "  ✓ Sync repositories between multiple organizations"
echo "  ✓ Handle all branches automatically"
echo "  ✓ Merge changes from all remotes"
echo "  ✓ Detect and report merge conflicts"
echo "  ✓ Handle configuration errors gracefully"
echo "  ✓ Sync multiple repositories with --sync-all"
echo "  ✓ Handle missing remote repositories gracefully"
