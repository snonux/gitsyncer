#!/bin/bash

# Test script to validate GitHub token
set -e

echo "Testing GitHub token authentication..."

# Try to load token from different sources
TOKEN=""

# 1. Environment variable
if [ -n "$GITHUB_TOKEN" ]; then
    echo "Found GITHUB_TOKEN environment variable"
    TOKEN="$GITHUB_TOKEN"
fi

# 2. Token file
if [ -z "$TOKEN" ] && [ -f ~/.gitsyncer_github_token ]; then
    echo "Found ~/.gitsyncer_github_token file"
    TOKEN=$(cat ~/.gitsyncer_github_token | tr -d '\n\r ')
fi

if [ -z "$TOKEN" ]; then
    echo "ERROR: No GitHub token found!"
    echo "Please set GITHUB_TOKEN environment variable or create ~/.gitsyncer_github_token file"
    exit 1
fi

echo "Token length: ${#TOKEN}"
echo "Token prefix: ${TOKEN:0:10}..."

# Test the token
echo ""
echo "Testing token with GitHub API..."
RESPONSE=$(curl -s -w "\nHTTP_STATUS:%{http_code}" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Accept: application/vnd.github.v3+json" \
    https://api.github.com/user)

HTTP_STATUS=$(echo "$RESPONSE" | grep HTTP_STATUS | cut -d: -f2)
BODY=$(echo "$RESPONSE" | grep -v HTTP_STATUS)

echo "HTTP Status: $HTTP_STATUS"

if [ "$HTTP_STATUS" = "200" ]; then
    echo "SUCCESS: Token is valid!"
    echo "Authenticated as: $(echo "$BODY" | jq -r .login)"
elif [ "$HTTP_STATUS" = "401" ]; then
    echo "ERROR: Token is invalid (401 Unauthorized)"
    echo "Response: $BODY"
    echo ""
    echo "Common issues:"
    echo "1. Token has expired"
    echo "2. Token doesn't have required scopes (need 'repo' scope)"
    echo "3. Token was revoked"
    echo ""
    echo "To create a new token:"
    echo "1. Go to https://github.com/settings/tokens"
    echo "2. Click 'Generate new token (classic)'"
    echo "3. Select 'repo' scope"
    echo "4. Save the token to ~/.gitsyncer_github_token"
else
    echo "ERROR: Unexpected status code: $HTTP_STATUS"
    echo "Response: $BODY"
fi

# Test specific repository access
echo ""
echo "Testing access to snonux/dtail repository..."
REPO_RESPONSE=$(curl -s -w "\nHTTP_STATUS:%{http_code}" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Accept: application/vnd.github.v3+json" \
    https://api.github.com/repos/snonux/dtail)

REPO_STATUS=$(echo "$REPO_RESPONSE" | grep HTTP_STATUS | cut -d: -f2)
REPO_BODY=$(echo "$REPO_RESPONSE" | grep -v HTTP_STATUS)

echo "Repository check status: $REPO_STATUS"
if [ "$REPO_STATUS" = "200" ]; then
    echo "SUCCESS: Can access repository"
elif [ "$REPO_STATUS" = "404" ]; then
    echo "Repository does not exist"
elif [ "$REPO_STATUS" = "401" ]; then
    echo "ERROR: Authentication failed for repository"
    echo "Response: $REPO_BODY"
fi