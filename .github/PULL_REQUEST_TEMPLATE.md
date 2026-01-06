# Pull Request

## Description

<!-- Provide a clear and concise description of what this PR does -->

## Related Issue

<!-- Link to the GitHub issue this PR addresses -->
Resolves #

## Type of Change

<!-- Mark the relevant option with an "x" -->

- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Documentation update
- [ ] Performance improvement
- [ ] Refactoring (no functional changes)
- [ ] Configuration change
- [ ] Dependency update

## Changes Made

<!-- List the key changes in this PR -->

-
-
-

## Testing Performed

<!-- Describe the testing you've done -->

### Unit Tests
- [ ] Added new unit tests
- [ ] Updated existing unit tests
- [ ] All unit tests pass (`make test`)

### Integration Tests
- [ ] Added new integration tests
- [ ] Updated existing integration tests
- [ ] All integration tests pass (`make test-integration`)

### Manual Testing
<!-- Describe any manual testing performed -->

-

## Performance Impact

<!-- Describe any performance implications -->

- [ ] No performance impact
- [ ] Performance improvement (describe below)
- [ ] Potential performance regression (justify below and provide mitigation)

Details:

## Security Considerations

<!-- Describe any security implications -->

- [ ] No security impact
- [ ] Security improvement (describe below)
- [ ] Security scan passed (`make security-scan`)
- [ ] No secrets or credentials committed

Details:

## Documentation

- [ ] Code is self-documenting with clear function/type names
- [ ] Added/updated function/type documentation
- [ ] Updated README.md (if applicable)
- [ ] Updated architecture docs (if applicable)
- [ ] Updated API documentation (if applicable)
- [ ] Added runbook/operational documentation (if applicable)

## Screenshots

<!-- If UI changes, add before/after screenshots -->

## Breaking Changes

<!-- If this is a breaking change, describe the impact and migration path -->

- [ ] No breaking changes
- [ ] Breaking changes (describe below)

Details:

## Deployment Notes

<!-- Any special deployment considerations -->

- [ ] No special deployment requirements
- [ ] Database migration required
- [ ] Configuration changes required (describe below)
- [ ] New dependencies added (list below)

Details:

## Checklist

<!-- All items must be checked before PR can be merged -->

### Code Quality
- [ ] Code follows project style guidelines (see CLAUDE.md)
- [ ] `make fmt` run and all code is formatted
- [ ] `make lint` passes with zero warnings
- [ ] `make test` passes (coverage ≥80%)
- [ ] `make security-scan` passes
- [ ] No `//nolint` directives added
- [ ] No TODO/FIXME without linked GitHub issues

### Testing
- [ ] Added tests for new functionality
- [ ] All existing tests still pass
- [ ] Edge cases are covered
- [ ] Error cases are tested

### Documentation
- [ ] Updated relevant documentation
- [ ] Added code comments for complex logic
- [ ] Exported functions/types are documented

### Security
- [ ] No hardcoded secrets or credentials
- [ ] No security vulnerabilities introduced
- [ ] Input validation added where needed
- [ ] Error messages don't expose sensitive information

### Git
- [ ] Commits follow conventional commit format
- [ ] Commit messages are clear and descriptive
- [ ] Branch is up to date with main
- [ ] No merge conflicts

### Review
- [ ] Self-reviewed the code
- [ ] Checked for common mistakes (see CLAUDE.md)
- [ ] Ready for team review

## Additional Context

<!-- Add any other context about the PR here -->

---

**By submitting this PR, I confirm that:**
- All quality checks have passed locally
- The code is ready for production deployment
- I have followed all guidelines in CLAUDE.md
- I am available to address review comments promptly
