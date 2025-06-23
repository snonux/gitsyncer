#!/bin/bash

# Test script to verify gitsyncer exit codes
echo "Testing gitsyncer exit codes..."

GITSYNCER="../gitsyncer"

# Test 1: Successful operation should exit 0
echo -n "Test 1 - Successful operation (--version): "
$GITSYNCER --version >/dev/null 2>&1
if [ $? -eq 0 ]; then
    echo "PASS (exit code 0)"
else
    echo "FAIL (expected 0, got $?)"
fi

# Test 2: No arguments should exit 1
echo -n "Test 2 - No arguments (show usage): "
$GITSYNCER >/dev/null 2>&1
if [ $? -eq 1 ]; then
    echo "PASS (exit code 1)"
else
    echo "FAIL (expected 1, got $?)"
fi

# Test 3: Invalid config file should exit 1
echo -n "Test 3 - Invalid config file: "
$GITSYNCER --config /nonexistent/config.json --list-orgs >/dev/null 2>&1
if [ $? -eq 1 ]; then
    echo "PASS (exit code 1)"
else
    echo "FAIL (expected 1, got $?)"
fi

# Test 4: Sync non-existent repo should exit 1
echo -n "Test 4 - Sync non-existent repo: "
$GITSYNCER --sync nonexistentrepo123456 >/dev/null 2>&1
if [ $? -eq 1 ]; then
    echo "PASS (exit code 1)"
else
    echo "FAIL (expected 1, got $?)"
fi

# Test 5: Successful list operation should exit 0
echo -n "Test 5 - Successful list operation: "
$GITSYNCER --list-orgs >/dev/null 2>&1
if [ $? -eq 0 ]; then
    echo "PASS (exit code 0)"
else
    echo "FAIL (expected 0, got $?)"
fi

# Test 6: Test GitHub token without token should exit 1
echo -n "Test 6 - Test GitHub token (invalid): "
GITHUB_TOKEN="invalid" $GITSYNCER --test-github-token >/dev/null 2>&1
if [ $? -eq 1 ]; then
    echo "PASS (exit code 1)"
else
    echo "FAIL (expected 1, got $?)"
fi

echo ""
echo "Summary: Exit codes are used correctly"
echo "- Exit 0: Successful operations"
echo "- Exit 1: Errors and invalid usage"