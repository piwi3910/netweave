# Contributing to netweave

Thank you for your interest in contributing to netweave! This document provides guidelines and instructions for contributing to the project.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Code Quality Standards](#code-quality-standards)
- [Pull Request Process](#pull-request-process)
- [Commit Message Guidelines](#commit-message-guidelines)
- [Testing Guidelines](#testing-guidelines)
- [Documentation Guidelines](#documentation-guidelines)

## Code of Conduct

This project adheres to a code of professional conduct:

- **Be respectful** - Treat all contributors with respect
- **Be constructive** - Provide helpful feedback
- **Be collaborative** - Work together to improve the project
- **Be professional** - Maintain a professional tone in all communications

## Getting Started

### Prerequisites

- Go 1.23 or later
- Docker (for container builds)
- Kubernetes cluster access (for integration tests)
- Git with GPG signing configured
- make

### Initial Setup

1. **Fork the repository** on GitHub

2. **Clone your fork**:
   ```bash
   git clone https://github.com/YOUR_USERNAME/netweave.git
   cd netweave
   ```

3. **Add upstream remote**:
   ```bash
   git remote add upstream https://github.com/yourorg/netweave.git
   ```

4. **Install development tools**:
   ```bash
   make install-tools
   ```

5. **Install git hooks**:
   ```bash
   make install-hooks
   ```

6. **Verify setup**:
   ```bash
   make verify-setup
   ```

### Configure Git Signing

All commits must be GPG signed:

```bash
# Generate GPG key if you don't have one
gpg --full-generate-key

# List your GPG keys
gpg --list-secret-keys --keyid-format LONG

# Configure git to use your key
git config --global user.signingkey YOUR_KEY_ID
git config --global commit.gpgsign true

# Add GPG key to GitHub
gpg --armor --export YOUR_KEY_ID
# Copy output and add to GitHub: Settings → SSH and GPG keys → New GPG key
```

## Development Workflow

### 1. Create a Feature Branch

Always create a new branch for your work:

```bash
# Update your local main
git checkout main
git pull upstream main

# Create feature branch
git checkout -b feature/issue-42-add-subscription-filter
```

Branch naming conventions:
- `feature/issue-NUM-description` - New features
- `fix/issue-NUM-description` - Bug fixes
- `docs/issue-NUM-description` - Documentation only
- `refactor/issue-NUM-description` - Code refactoring
- `test/issue-NUM-description` - Test additions/improvements

### 2. Make Your Changes

Follow the coding standards in [CLAUDE.md](CLAUDE.md):

- Write clean, readable, well-documented code
- Follow Go best practices and idioms
- Add tests for new functionality
- Update documentation as needed

### 3. Run Quality Checks

Before committing, ensure all quality checks pass:

```bash
# Format code
make fmt

# Run linters
make lint

# Run tests
make test

# Run all quality checks
make quality
```

### 4. Commit Your Changes

Write clear, descriptive commit messages following [Conventional Commits](#commit-message-guidelines):

```bash
git add .
git commit -m "feat(subscription): add resource type filter

Adds ability to filter subscriptions by resource type.
Includes unit and integration tests.

Resolves #42"
```

**Pre-commit hooks will automatically run** to verify:
- Code formatting
- Linting
- Security scans
- No secrets committed

### 5. Push and Create PR

```bash
# Push your branch
git push origin feature/issue-42-add-subscription-filter

# Create pull request via GitHub UI or gh CLI
gh pr create --title "feat(subscription): add resource type filter" \
  --body "Resolves #42"
```

## Code Quality Standards

### Zero-Tolerance Policy

**ALL code must pass these checks** before merging:

1. ✅ **Formatting**: `make fmt` produces no changes
2. ✅ **Linting**: `make lint` passes with zero warnings
3. ✅ **Tests**: `make test` passes (≥80% coverage)
4. ✅ **Security**: `make security-scan` passes
5. ✅ **Integration**: Integration tests pass
6. ✅ **Build**: `make build` succeeds

### What NOT to Do

❌ **NEVER** disable linters with `//nolint` comments
❌ **NEVER** commit code with linting warnings
❌ **NEVER** ignore security vulnerabilities
❌ **NEVER** commit secrets or credentials
❌ **NEVER** skip writing tests
❌ **NEVER** bypass pre-commit hooks
❌ **NEVER** force-push to shared branches

### Code Review Expectations

When your PR is reviewed, expect feedback on:

- Code correctness and logic
- Test coverage and quality
- Documentation completeness
- Performance implications
- Security considerations
- Adherence to project patterns

**Respond to review comments promptly** and professionally.

## Pull Request Process

### Before Creating a PR

- [ ] Read [CLAUDE.md](CLAUDE.md) thoroughly
- [ ] Create/update GitHub issue describing the change
- [ ] Run `make quality` and ensure it passes
- [ ] Write/update tests (≥80% coverage)
- [ ] Update documentation if needed
- [ ] Rebase on latest main: `git rebase upstream/main`

### PR Requirements

All PRs must:

1. **Link to an issue**: `Resolves #NUM` in description
2. **Pass all CI checks**: See [.github/workflows/ci.yml](.github/workflows/ci.yml)
3. **Have ≥1 approval**: From a project maintainer
4. **Be up-to-date**: Rebased on latest main
5. **Be signed**: All commits GPG signed
6. **Have tests**: New code must have tests
7. **Be documented**: Public APIs must be documented

### PR Template

When creating a PR, fill out the [PR template](.github/PULL_REQUEST_TEMPLATE.md) completely:

- Clear description of changes
- Testing performed
- Security considerations
- Documentation updates
- Deployment notes (if applicable)

### Review Process

1. **Automated checks run** (linting, tests, security)
2. **Code review** by maintainer(s)
3. **Address feedback** via new commits
4. **Re-review** if significant changes
5. **Squash and merge** when approved

### Merge Requirements

PRs can only be merged when:

- ✅ All CI checks pass
- ✅ ≥1 approval from maintainer
- ✅ All review comments resolved
- ✅ Branch is up-to-date with main
- ✅ No merge conflicts

## Commit Message Guidelines

We follow [Conventional Commits](https://www.conventionalcommits.org/):

### Format

```
<type>(<scope>): <short summary>

<detailed description>

<footer>
```

### Types

- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, no logic change)
- `refactor`: Code refactoring (no functional change)
- `perf`: Performance improvements
- `test`: Adding or updating tests
- `build`: Build system changes
- `ci`: CI/CD changes
- `chore`: Maintenance tasks

### Scope

The scope indicates what part of the codebase is affected:

- `adapter`: Adapter layer
- `cache`: Caching logic
- `storage`: Storage layer
- `subscription`: Subscription management
- `o2ims`: O2-IMS implementation
- `k8s`: Kubernetes integration
- `redis`: Redis integration
- `security`: Security features
- `deps`: Dependency updates

### Examples

```bash
# New feature
feat(subscription): add webhook retry logic

Implements exponential backoff for webhook delivery failures.
Retries up to 3 times with 1s, 2s, 4s delays.

Resolves #123

# Bug fix
fix(cache): prevent race condition in invalidation

Use mutex to protect concurrent cache updates.

Resolves #456

# Documentation
docs(api): update O2-IMS subscription API examples

Add examples for filtering by resource type.

# Breaking change
feat(adapter)!: change adapter interface to support pagination

BREAKING CHANGE: All adapter implementations must update
the List() method signature to include pagination parameters.

Resolves #789
```

## Testing Guidelines

### Test Coverage Requirements

- **Unit tests**: ≥80% coverage for all packages
- **Integration tests**: All API endpoints
- **E2E tests**: Critical user workflows

### Writing Tests

```go
// Use table-driven tests
func TestSubscriptionStore_Create(t *testing.T) {
    tests := []struct {
        name    string
        sub     *O2Subscription
        wantErr bool
    }{
        {
            name: "valid subscription",
            sub:  validSubscription(),
            wantErr: false,
        },
        {
            name: "invalid callback URL",
            sub:  invalidSubscription(),
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := store.Create(context.Background(), tt.sub)
            if tt.wantErr {
                require.Error(t, err)
            } else {
                require.NoError(t, err)
            }
        })
    }
}
```

### Running Tests

```bash
# Unit tests
make test

# Integration tests (requires Redis and Kubernetes)
make test-integration

# E2E tests
make test-e2e

# Watch mode (for development)
make test-watch

# Coverage report
make test-coverage
```

## Documentation Guidelines

### Code Documentation

- Document all exported functions, types, and packages
- Use godoc format
- Include examples for complex APIs
- Explain WHY, not just WHAT

```go
// SubscriptionStore manages O2-IMS subscriptions.
// It provides thread-safe CRUD operations with Redis persistence
// and automatic webhook notification on resource changes.
type SubscriptionStore interface {
    // Create creates a new subscription.
    // Returns ErrSubscriptionExists if a subscription with the same ID exists.
    Create(ctx context.Context, sub *O2Subscription) error

    // Get retrieves a subscription by ID.
    // Returns ErrSubscriptionNotFound if the subscription doesn't exist.
    Get(ctx context.Context, id string) (*O2Subscription, error)
}
```

### Documentation Files

When changing functionality, update relevant documentation:

- `README.md` - Project overview and quickstart
- `docs/architecture.md` - Architecture details
- `docs/api-mapping.md` - O2-IMS ↔ Kubernetes mappings
- `docs/deployment.md` - Deployment instructions
- `docs/operations.md` - Operational procedures

## Common Tasks

### Adding a New Feature

1. Create GitHub issue describing the feature
2. Discuss design in the issue
3. Create feature branch
4. Implement feature with tests
5. Update documentation
6. Run `make quality`
7. Create PR

### Fixing a Bug

1. Create GitHub issue with reproduction steps
2. Write failing test demonstrating the bug
3. Fix the bug
4. Ensure test passes
5. Run `make quality`
6. Create PR

### Updating Dependencies

```bash
# Update all dependencies
make deps-upgrade

# Run tests to verify compatibility
make test

# Run integration tests
make test-integration

# Create PR with dependency updates
```

## Getting Help

- **Questions**: Open a GitHub Discussion
- **Bugs**: Open a GitHub Issue
- **Security**: Email security@example.com (DO NOT open public issue)
- **Chat**: Join our Slack/Discord (if available)

## Recognition

Contributors will be recognized in:
- `CONTRIBUTORS.md` file
- Release notes
- GitHub contributors page

## License

By contributing, you agree that your contributions will be licensed under the same license as the project (see [LICENSE](LICENSE)).

---

**Thank you for contributing to netweave!** 🎉

We appreciate your effort to make this project better. Every contribution, no matter how small, makes a difference.
