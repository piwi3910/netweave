# Task Completion Checklist

## MANDATORY Steps Before Completing Any Task

### 1. Code Quality (ZERO TOLERANCE)

```bash
# Run formatting
make fmt

# Run linters (MUST pass with ZERO warnings)
make lint

# NEVER use //nolint comments to suppress warnings
# FIX THE CODE, NOT THE RULES
```

### 2. Testing Requirements

```bash
# Run unit tests (MUST pass)
make test

# Check coverage (MUST be ≥80%)
make test-coverage

# Run integration tests if applicable
make test-integration

# Run E2E tests for critical flows
make test-e2e
```

### 3. Security Validation

```bash
# Run security scans (MUST pass)
make security-scan

# Check for secrets
make check-secrets
```

### 4. Documentation Updates

- Update GoDoc comments for all new/modified exported functions and types
- Update docs/api-mapping.md for API changes
- Update docs/architecture.md for architecture changes
- Update README.md for feature/tech stack changes
- Ensure all code examples in docs are tested and working
- Run make lint-docs to verify documentation formatting

### 5. All-in-One Quality Gate

```bash
# Run complete quality check (REQUIRED before PR)
make quality

# This checks:
# ✅ Code formatting
# ✅ All linters pass (zero warnings)
# ✅ All tests pass
# ✅ Coverage ≥80%
# ✅ Security scans pass
```

### 6. Git Workflow

- Create feature branch: issue-NUM-description
- Write meaningful commit messages
- Reference issue numbers in commits
- Run pre-commit hooks automatically
- Never commit without passing quality checks

## Violations That BLOCK Merge

❌ Linting errors or warnings
❌ Test failures
❌ Coverage below 80%
❌ Security vulnerabilities
❌ Missing GoDoc for exported functions
❌ API changes without updating docs/api-mapping.md
❌ Architecture changes without updating docs/architecture.md
❌ Outdated code examples in documentation
❌ Using //nolint directives
❌ Using any or interface{} without justification
❌ Ignoring errors with _ operator
❌ Hardcoded secrets or credentials
❌ Missing unit tests for new code
❌ Test coverage below 80%

## Success Criteria

✅ make quality passes
✅ All documentation updated and synchronized
✅ GoDoc complete for all exported symbols
✅ Commit message references issue number
✅ Feature branch follows naming convention
✅ No secrets or sensitive data committed
✅ Pre-commit hooks installed and passing

## Remember

**Quality is not optional. This is production code for critical telecom infrastructure.**

When in doubt, prioritize:
1. Security - Never compromise security
2. Quality - Code must be maintainable
3. Performance - Must scale to production loads
4. Observability - Must be debuggable in production
