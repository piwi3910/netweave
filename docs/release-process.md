# Release Process

This document describes the release process for netweave O2 Gateway.

## Table of Contents

- [Release Types](#release-types)
- [Release Schedule](#release-schedule)
- [Pre-Release Checklist](#pre-release-checklist)
- [Release Steps](#release-steps)
- [Hotfix Process](#hotfix-process)
- [Version Numbering](#version-numbering)
- [Automation](#automation)

---

## Release Types

### Major Release (x.0.0)

**When to Release:**
- Breaking API changes
- Major architectural changes
- Removal of deprecated features
- Significant O-RAN spec version updates

**Examples:**
- v1.0.0 → v2.0.0: Breaking API changes, new architecture
- v2.0.0 → v3.0.0: Major O-RAN spec compliance update

### Minor Release (1.x.0)

**When to Release:**
- New features (backward compatible)
- New backend adapters
- Performance improvements
- Non-breaking API additions

**Examples:**
- v1.0.0 → v1.1.0: Add Azure adapter
- v1.1.0 → v1.2.0: Add batch operations API

### Patch Release (1.0.x)

**When to Release:**
- Bug fixes
- Security patches
- Documentation updates
- Dependency updates (non-breaking)

**Examples:**
- v1.0.0 → v1.0.1: Fix subscription delivery bug
- v1.0.1 → v1.0.2: Security patch for CVE

---

## Release Schedule

### Regular Releases

- **Patch releases**: As needed for critical bugs or security issues
- **Minor releases**: Monthly or quarterly (based on feature readiness)
- **Major releases**: Annually or when breaking changes are necessary

### Support Policy

- **Current major version**: Full support (features, bugs, security)
- **Previous major version**: Security and critical bugs only (12 months)
- **Older versions**: No support (upgrade recommended)

**Example:**
- v2.x.x: Full support
- v1.x.x: Security and critical bugs (until v2.0 + 12 months)
- v0.x.x: No support

---

## Pre-Release Checklist

### Code Quality

- [ ] All CI checks passing (lint, test, security scan)
- [ ] Test coverage ≥80%
- [ ] No critical security vulnerabilities
- [ ] No known memory leaks or race conditions
- [ ] Performance benchmarks meet targets

### Documentation

- [ ] CHANGELOG.md updated with all changes
- [ ] API documentation updated (if API changed)
- [ ] Migration guide created (for breaking changes)
- [ ] README.md version references updated
- [ ] Compliance status document updated
- [ ] Architecture diagrams updated (if architecture changed)

### Versioning

- [ ] Version bumped in:
  - [ ] `internal/version/version.go`
  - [ ] `charts/netweave/Chart.yaml`
  - [ ] `charts/netweave/values.yaml`
  - [ ] `docs/IMPLEMENTATION_STATUS.md`
- [ ] Git tag format validated: `v1.2.3` (not `1.2.3`)

### Testing

- [ ] Unit tests: `make test`
- [ ] Integration tests: `make test-integration`
- [ ] E2E tests: `make test-e2e`
- [ ] Upgrade test from previous version
- [ ] Downgrade/rollback tested
- [ ] Load test completed (performance validation)

### Compliance

- [ ] O-RAN compliance check: `make compliance-check`
- [ ] OpenAPI spec validated
- [ ] No API regressions
- [ ] Backward compatibility verified (for minor/patch)

### Security

- [ ] Security scan: `make security-scan`
- [ ] Dependencies scanned: `govulncheck ./...`
- [ ] Container image scanned: `trivy image`
- [ ] SBOM generated: `syft . -o json > sbom.json`
- [ ] Security advisories reviewed

### Artifacts

- [ ] Binaries built for all platforms (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64)
- [ ] Container images built and tagged
- [ ] Helm chart packaged
- [ ] Checksums generated (SHA256)
- [ ] SBOM attached to release

---

## Release Steps

### Step 1: Prepare Release Branch

```bash
# Ensure main is up to date
git checkout main
git pull origin main

# Create release branch
git checkout -b release-v1.2.0

# Update version in code
./scripts/update-version.sh v1.2.0

# Update CHANGELOG.md
vim CHANGELOG.md
# Add release notes using template from .github/RELEASE_TEMPLATE.md

# Commit changes
git add .
git commit -m "[Release] Prepare v1.2.0

- Update version to v1.2.0
- Update CHANGELOG.md
- Update documentation references"

# Push release branch
git push origin release-v1.2.0
```

### Step 2: Create Pull Request

```bash
# Create PR for release branch
gh pr create \
  --title "Release v1.2.0" \
  --body "$(cat .github/RELEASE_TEMPLATE.md)" \
  --base main

# Wait for CI to pass
# Get at least one approval
# Address any review feedback
```

### Step 3: Merge and Tag

```bash
# Merge PR (use squash merge)
gh pr merge --squash

# Switch to main and pull
git checkout main
git pull origin main

# Create annotated tag
git tag -a v1.2.0 -m "Release v1.2.0

$(cat CHANGELOG.md | sed -n '/^## v1.2.0/,/^## v/p' | head -n -1)"

# Push tag (triggers release CI)
git push origin v1.2.0
```

### Step 4: Build Release Artifacts

The CI pipeline automatically builds artifacts when a tag is pushed. Manual build if needed:

```bash
# Build binaries for all platforms
make release VERSION=v1.2.0

# Build and push container images
make docker-build docker-push VERSION=v1.2.0

# Package Helm chart
make helm-package VERSION=v1.2.0

# Generate checksums
cd dist/
sha256sum * > checksums.txt
```

### Step 5: Create GitHub Release

```bash
# Create GitHub release (automated via CI, or manual)
gh release create v1.2.0 \
  --title "Release v1.2.0" \
  --notes-file .github/release-notes/v1.2.0.md \
  --draft  # Review before publishing

# Upload artifacts
gh release upload v1.2.0 \
  dist/netweave-linux-amd64.tar.gz \
  dist/netweave-linux-arm64.tar.gz \
  dist/netweave-darwin-amd64.tar.gz \
  dist/netweave-darwin-arm64.tar.gz \
  dist/checksums.txt \
  dist/sbom.json

# Publish release
gh release edit v1.2.0 --draft=false
```

### Step 6: Post-Release Tasks

```bash
# Update Helm repository index
cd charts/
helm repo index . --url https://piwi3910.github.io/netweave
git add index.yaml
git commit -m "[Release] Update Helm repo index for v1.2.0"
git push

# Announce release
# - GitHub Discussions
# - Slack/Discord channels
# - Mailing list
# - Social media

# Update documentation site
# - Deploy updated docs to GitHub Pages
# - Update version selector

# Create next development branch (optional)
git checkout -b dev-v1.3.0
```

---

## Hotfix Process

For critical bugs in production that cannot wait for the next regular release.

### Step 1: Create Hotfix Branch

```bash
# Branch from the affected release tag
git checkout v1.2.0
git checkout -b hotfix-v1.2.1

# Apply minimal fix
# DO NOT include unrelated changes
git commit -m "[Hotfix] Fix critical subscription bug

Resolves #456"
```

### Step 2: Test Thoroughly

```bash
# Run all tests
make test test-integration test-e2e

# Test upgrade from v1.2.0 → v1.2.1
# Test rollback from v1.2.1 → v1.2.0

# Verify fix in production-like environment
```

### Step 3: Release Hotfix

```bash
# Update version
./scripts/update-version.sh v1.2.1

# Update CHANGELOG
vim CHANGELOG.md

# Commit and tag
git commit -am "[Release] Hotfix v1.2.1"
git tag -a v1.2.1 -m "Hotfix v1.2.1"
git push origin hotfix-v1.2.1
git push origin v1.2.1

# Create GitHub release (expedited)
gh release create v1.2.1 \
  --title "Hotfix v1.2.1" \
  --notes "Critical bug fix for subscription delivery issue (#456)" \
  --prerelease=false
```

### Step 4: Backport to Main

```bash
# Cherry-pick fix to main
git checkout main
git cherry-pick <commit-hash>
git push origin main

# Or merge hotfix branch if no conflicts
git merge hotfix-v1.2.1
```

---

## Version Numbering

### Format

**Stable releases:**
```
vMAJOR.MINOR.PATCH
```

**Pre-releases:**
```
vMAJOR.MINOR.PATCH-rc.N      # Release candidate
vMAJOR.MINOR.PATCH-beta.N    # Beta
vMAJOR.MINOR.PATCH-alpha.N   # Alpha
```

**Development:**
```
vMAJOR.MINOR.PATCH-dev       # Development builds
```

### Examples

- `v1.0.0` - First stable release
- `v1.1.0` - Minor release with new features
- `v1.1.1` - Patch release with bug fixes
- `v2.0.0-rc.1` - Release candidate for major version
- `v1.2.0-beta.1` - Beta release
- `v1.3.0-dev` - Development build

### Semantic Versioning Rules

1. **MAJOR** version increments when:
   - Breaking API changes
   - Incompatible configuration changes
   - Removal of deprecated features
   - Database schema changes requiring migration

2. **MINOR** version increments when:
   - New features (backward compatible)
   - New backend adapters
   - Performance improvements
   - Deprecation of features (with migration path)

3. **PATCH** version increments when:
   - Bug fixes
   - Security patches
   - Documentation updates
   - Dependency updates (non-breaking)

---

## Automation

### GitHub Actions Workflows

**Release Workflow** (`.github/workflows/release.yml`):
- Triggered on: Tag push matching `v*`
- Actions:
  1. Run full test suite
  2. Build binaries for all platforms
  3. Build and push Docker images
  4. Package Helm chart
  5. Generate SBOM
  6. Create GitHub release
  7. Upload artifacts
  8. Update Helm repository

**Nightly Build** (`.github/workflows/nightly.yml`):
- Triggered on: Daily schedule
- Actions:
  1. Build from main branch
  2. Tag as `vX.Y.Z-dev`
  3. Push to Docker registry (dev tag)

### Release Scripts

**`scripts/update-version.sh`**:
```bash
#!/bin/bash
# Updates version in all relevant files

NEW_VERSION=$1

sed -i '' "s/Version = \".*\"/Version = \"$NEW_VERSION\"/" internal/version/version.go
sed -i '' "s/version: .*/version: $NEW_VERSION/" charts/netweave/Chart.yaml
sed -i '' "s/appVersion: .*/appVersion: $NEW_VERSION/" charts/netweave/Chart.yaml
```

**`scripts/generate-changelog.sh`**:
```bash
#!/bin/bash
# Generates changelog from git commits since last tag

LAST_TAG=$(git describe --tags --abbrev=0)
git log $LAST_TAG..HEAD --pretty=format:"- %s (#%h)" > CHANGELOG.new
```

### Release Checklist Bot

Consider using a GitHub bot to track release checklist:
- [ ] Tests passing
- [ ] Docs updated
- [ ] CHANGELOG updated
- [ ] Version bumped
- [ ] Security scan passed

---

## Troubleshooting

### Release Build Failed

**Issue**: CI fails to build release artifacts

**Solutions:**
1. Check build logs: `gh run view <run-id> --log`
2. Verify dependencies are vendored: `go mod vendor`
3. Test build locally: `make release VERSION=vX.Y.Z`
4. Check for platform-specific issues

### Tag Already Exists

**Issue**: Cannot push tag because it already exists

**Solutions:**
```bash
# Delete local tag
git tag -d v1.2.0

# Delete remote tag (if really needed)
git push --delete origin v1.2.0

# Recreate tag with correct commit
git tag -a v1.2.0 -m "Release v1.2.0"
git push origin v1.2.0
```

### Helm Chart Fails to Install

**Issue**: Helm chart installation fails after release

**Solutions:**
1. Validate chart: `helm lint charts/netweave/`
2. Dry run: `helm install netweave charts/netweave/ --dry-run`
3. Check template rendering: `helm template netweave charts/netweave/`
4. Verify chart version matches release

### Docker Image Not Found

**Issue**: Docker image for new release not available

**Solutions:**
1. Check CI logs for docker push step
2. Verify image tags: `docker pull ghcr.io/piwi3910/netweave:v1.2.0`
3. Check registry permissions
4. Retry push manually if needed

---

## See Also

- **[CHANGELOG.md](../CHANGELOG.md)** - Release history
- **[CONTRIBUTING.md](../CONTRIBUTING.md)** - Contribution guidelines
- **[Upgrade Guide](operations/upgrades.md)** - Version upgrade procedures
- **[CI/CD Documentation](.github/workflows/README.md)** - CI pipeline details
