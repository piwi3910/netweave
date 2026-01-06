# Branch Protection Rules for netweave

This document describes the required branch protection rules for the `main` branch.

## GitHub Repository Settings

### Required Settings for `main` Branch

Navigate to: **Settings → Branches → Branch protection rules → Add rule**

#### Branch name pattern
```
main
```

#### Protect matching branches

**Require a pull request before merging:**
- ✅ Enabled
- Required approvals: **1**
- ✅ Dismiss stale pull request approvals when new commits are pushed
- ✅ Require review from Code Owners (if CODEOWNERS file exists)
- ❌ Restrict who can dismiss pull request reviews (allow maintainers)
- ✅ Allow specified actors to bypass pull request requirements (ONLY for hotfixes by maintainers)
- ✅ Require approval of the most recent reviewable push

**Require status checks to pass before merging:**
- ✅ Enabled
- ✅ Require branches to be up to date before merging

Required status checks:
- `Lint Code`
- `Security Scan`
- `Unit Tests`
- `Integration Tests`
- `Build Binary`
- `Build Docker Image`
- `Status Check`

**Require conversation resolution before merging:**
- ✅ Enabled (all PR comments must be resolved)

**Require signed commits:**
- ✅ Enabled (all commits must be GPG signed)

**Require linear history:**
- ✅ Enabled (enforce squash or rebase merge, no merge commits)

**Require deployments to succeed before merging:**
- ❌ Disabled (for now, enable when staging environment is ready)

**Lock branch:**
- ❌ Disabled (allow pushes via PR)

**Do not allow bypassing the above settings:**
- ✅ Enabled (admins must follow same rules)

**Restrict who can push to matching branches:**
- ✅ Enabled
- Allowed actors: **None** (all changes via PR only)
- Include administrators: ✅ Yes

**Allow force pushes:**
- ❌ Disabled

**Allow deletions:**
- ❌ Disabled

## Rulesets (Advanced - GitHub Enterprise)

If using GitHub Enterprise, create a ruleset instead:

### Ruleset Configuration

**Name:** `Main Branch Protection`

**Enforcement status:** Active

**Bypass list:**
- ONLY in emergencies with multi-maintainer approval

**Target branches:**
- Include: `main`

**Rules:**

1. **Restrict deletions** ✅
2. **Restrict force pushes** ✅
3. **Require linear history** ✅
4. **Require signed commits** ✅
5. **Require pull request**:
   - Required approvals: 1
   - Dismiss stale reviews: ✅
   - Require code owner review: ✅
   - Require approval of most recent push: ✅
6. **Require status checks**:
   - Require branches to be up to date: ✅
   - Status checks:
     - `Lint Code`
     - `Security Scan`
     - `Unit Tests`
     - `Integration Tests`
     - `Build Binary`
     - `Build Docker Image`
     - `Status Check`
7. **Block force pushes** ✅
8. **Require conversation resolution** ✅

## Setting Up via GitHub CLI

```bash
# Install GitHub CLI if not already installed
# brew install gh  # macOS
# or download from https://cli.github.com/

# Authenticate
gh auth login

# Create branch protection rule
gh api repos/{owner}/netweave/branches/main/protection \
  --method PUT \
  --field required_status_checks='{"strict":true,"contexts":["Lint Code","Security Scan","Unit Tests","Integration Tests","Build Binary","Build Docker Image","Status Check"]}' \
  --field enforce_admins=true \
  --field required_pull_request_reviews='{"dismissal_restrictions":{},"dismiss_stale_reviews":true,"require_code_owner_reviews":true,"required_approving_review_count":1,"require_last_push_approval":true}' \
  --field restrictions=null \
  --field required_linear_history=true \
  --field allow_force_pushes=false \
  --field allow_deletions=false \
  --field block_creations=false \
  --field required_conversation_resolution=true \
  --field lock_branch=false \
  --field allow_fork_syncing=true
```

## Setting Up via Terraform (Infrastructure as Code)

```hcl
# terraform/github.tf

resource "github_branch_protection" "main" {
  repository_id = github_repository.netweave.node_id
  pattern       = "main"

  required_status_checks {
    strict   = true
    contexts = [
      "Lint Code",
      "Security Scan",
      "Unit Tests",
      "Integration Tests",
      "Build Binary",
      "Build Docker Image",
      "Status Check"
    ]
  }

  required_pull_request_reviews {
    dismiss_stale_reviews           = true
    require_code_owner_reviews      = true
    required_approving_review_count = 1
    require_last_push_approval      = true
  }

  enforce_admins                  = true
  require_signed_commits          = true
  require_linear_history          = true
  require_conversation_resolution = true
  allow_force_pushes              = false
  allow_deletions                 = false
}
```

## CODEOWNERS File

Create a `.github/CODEOWNERS` file to require reviews from specific teams/people:

```
# Default owners for everything in the repo
* @netweave-team/core-maintainers

# Specific ownership
/docs/ @netweave-team/documentation
/.github/ @netweave-team/devops
/deployments/ @netweave-team/devops
/internal/security/ @netweave-team/security
/internal/o2ims/ @netweave-team/api-developers
```

## Verifying Branch Protection

After setting up, verify the protection is active:

```bash
# Via GitHub CLI
gh api repos/{owner}/netweave/branches/main/protection

# Via web UI
# Navigate to Settings → Branches and verify all checkmarks
```

## Exception Process (Hotfixes)

For critical production hotfixes ONLY:

1. Create a hotfix branch from main: `hotfix/critical-issue-description`
2. Make minimal changes to fix the issue
3. Create PR with `[HOTFIX]` prefix in title
4. Require 2 maintainer approvals (instead of 1)
5. All CI checks must still pass (no exceptions)
6. After merge, create post-mortem issue explaining why protection bypass was needed

## Monitoring

Set up alerts for:
- Branch protection changes (should rarely happen)
- Failed status checks
- Force push attempts (should be blocked)
- Protection bypass attempts

Use GitHub's audit log:
```
Settings → Security → Audit log → Filter: "protected_branch"
```

## Updating This Configuration

When CI checks change:
1. Update `.github/workflows/ci.yml` first
2. Test on a feature branch
3. Update this document
4. Update branch protection rules
5. Verify all checks still pass

## Common Issues

### "Required status check is not present"
- The job name in the workflow must exactly match the status check name
- Run a test PR to see which checks are reporting
- Update the required checks list

### "Branch is not up to date"
- Rebase your branch on latest main: `git rebase origin/main`
- Or use GitHub's "Update branch" button in the PR

### "Commit is not signed"
- Set up GPG signing: https://docs.github.com/en/authentication/managing-commit-signature-verification
- Configure git: `git config --global commit.gpgsign true`

## References

- [GitHub Branch Protection Documentation](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches)
- [GitHub Rulesets](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/about-rulesets)
- [Signing Commits](https://docs.github.com/en/authentication/managing-commit-signature-verification/signing-commits)
