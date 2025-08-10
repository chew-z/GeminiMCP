set -euo pipefail

# Pre-flight checks
echo "Performing pre-flight checks..."
gh auth status
if ! git diff-index --quiet HEAD --; then
    echo "Error: Uncommitted changes detected. Aborting."
    exit 1
fi
echo "Checks passed."

# Versioning
new_tag="v0.1.0"
echo "New version: ${new_tag}"

# Build
echo "Building project..."
go build -o ./bin/mcp-gemini .
assets=("./bin/mcp-gemini")
echo "Build complete."

# Create Git tag and push
echo "Creating and pushing Git tag..."
git tag -a "${new_tag}" -m "Release ${new_tag}"
git push origin "${new_tag}"
echo "Tag pushed."

# Create GitHub Release
echo "Creating GitHub release..."
repo_nwo="chew-z/GeminiMCP"
if [ "$(gh repo view "${repo_nwo}" --json hasDiscussionsEnabled --jq .hasDiscussionsEnabled)" = "true" ]; then
    gh release create "${new_tag}" "${assets[@]}" --title="${new_tag}" --notes-file RELEASE_NOTES.md --latest --discussion-category="Releases" --verify-tag
else
    gh release create "${new_tag}" "${assets[@]}" --title="${new_tag}" --notes-file RELEASE_NOTES.md --latest --verify-tag
fi

# Cleanup
rm RELEASE_NOTES.md

echo "Release ${new_tag} created successfully!"