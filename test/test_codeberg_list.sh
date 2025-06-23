#!/bin/bash

# Test script to list Codeberg public repos without syncing
set -e

echo "Testing Codeberg API to list public repositories..."

# Use curl to test the API directly
USER="snonux"
echo "Fetching public repos for user: $USER"

# Try as user
echo ""
echo "Trying user endpoint..."
curl -s "https://codeberg.org/api/v1/users/$USER/repos?limit=50" | \
    jq -r '.[] | select(.private == false and .fork == false and .archived == false) | .name' | \
    sort

echo ""
echo "Total public repos (non-fork, non-archived):"
curl -s "https://codeberg.org/api/v1/users/$USER/repos?limit=50" | \
    jq '[.[] | select(.private == false and .fork == false and .archived == false)] | length'