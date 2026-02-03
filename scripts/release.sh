#!/bin/bash
set -e

# Release script for Airbridge
# Usage: ./scripts/release.sh bump [major|minor|patch]

COMMAND=$1
BUMP_TYPE=$2

usage() {
    echo "Usage: $0 bump [major|minor|patch]"
    echo ""
    echo "Examples:"
    echo "  $0 bump patch   # v1.0.0 -> v1.0.1"
    echo "  $0 bump minor   # v1.0.0 -> v1.1.0"
    echo "  $0 bump major   # v1.0.0 -> v2.0.0"
    exit 1
}

if [ "$COMMAND" != "bump" ] || [ -z "$BUMP_TYPE" ]; then
    usage
fi

if [[ ! "$BUMP_TYPE" =~ ^(major|minor|patch)$ ]]; then
    echo "Error: Bump type must be 'major', 'minor', or 'patch'"
    usage
fi

# Ensure we're on main branch
BRANCH=$(git branch --show-current)
if [ "$BRANCH" != "main" ]; then
    echo "Error: Must be on main branch (currently on $BRANCH)"
    exit 1
fi

# Ensure working directory is clean
if [ -n "$(git status --porcelain)" ]; then
    echo "Error: Working directory is not clean. Commit or stash changes first."
    exit 1
fi

# Get the latest version tag
LATEST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
echo "Current version: $LATEST_TAG"

# Parse version (remove 'v' prefix)
VERSION=${LATEST_TAG#v}
IFS='.' read -r MAJOR MINOR PATCH <<< "$VERSION"

# Bump the appropriate part
case $BUMP_TYPE in
    major)
        MAJOR=$((MAJOR + 1))
        MINOR=0
        PATCH=0
        ;;
    minor)
        MINOR=$((MINOR + 1))
        PATCH=0
        ;;
    patch)
        PATCH=$((PATCH + 1))
        ;;
esac

NEW_VERSION="v${MAJOR}.${MINOR}.${PATCH}"
echo "New version: $NEW_VERSION"

# Pull latest changes
echo ""
echo "Pulling latest changes..."
git pull origin main

# Create and push tag
echo "Creating tag $NEW_VERSION..."
git tag -a "$NEW_VERSION" -m "Release $NEW_VERSION"

echo "Pushing tag to origin..."
git push origin "$NEW_VERSION"

echo ""
echo "✅ Released $NEW_VERSION"
echo "   GitHub Actions will now build and push Docker images."
