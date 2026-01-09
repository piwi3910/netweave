# Supply Chain Security

This document describes the supply chain security measures implemented in the netweave O2-IMS Gateway project.

## Overview

Supply chain security ensures that all dependencies, third-party libraries, and build artifacts are free from known vulnerabilities and comply with licensing requirements. This project implements multiple layers of automated security scanning and compliance checking.

## Components

### 1. Dependency Vulnerability Scanning

We use multiple tools to detect known vulnerabilities in our dependencies:

#### Snyk
- **Purpose**: Commercial-grade vulnerability database with detailed remediation advice
- **Frequency**: Weekly scheduled scans + on every PR
- **Configuration**: `.github/workflows/dependency-scan.yml`
- **Severity Threshold**: HIGH and above
- **Setup**: Requires `SNYK_TOKEN` secret in GitHub repository settings

#### Nancy (Sonatype)
- **Purpose**: OSS Index vulnerability scanner
- **Frequency**: Weekly scheduled scans + on every PR
- **Configuration**: `.github/workflows/dependency-scan.yml`
- **Database**: Sonatype OSS Index
- **No authentication required**

#### govulncheck
- **Purpose**: Official Go vulnerability database scanner
- **Frequency**: Weekly scheduled scans + on every PR + existing CI pipeline
- **Configuration**: `.github/workflows/dependency-scan.yml` and `.github/workflows/ci.yml`
- **Database**: Go vulnerability database (https://vuln.go.dev)
- **No authentication required**

#### Trivy
- **Purpose**: Comprehensive vulnerability scanner for containers and filesystems
- **Frequency**: Weekly scheduled scans + on every PR
- **Configuration**: `.github/workflows/dependency-scan.yml`
- **Severity**: CRITICAL, HIGH, MEDIUM
- **No authentication required**

### 2. License Compliance

Ensures all dependencies comply with our license policy.

#### Approved Licenses

The following licenses are approved for use:

- **Apache-2.0** - Permissive, widely used, patent grant
- **MIT** - Permissive, simple
- **BSD-2-Clause** - Permissive, simple
- **BSD-3-Clause** - Permissive with advertising clause
- **ISC** - Permissive, functionally equivalent to MIT
- **MPL-2.0** - Weak copyleft, file-level

#### Forbidden Licenses

The following licenses are **STRICTLY FORBIDDEN**:

- **GPL-2.0, GPL-3.0** - Strong copyleft (derivative works must be GPL)
- **LGPL-2.0, LGPL-2.1, LGPL-3.0** - Weak copyleft (linking restrictions)
- **AGPL-3.0** - Network copyleft (SaaS copyleft)

Any dependency using a forbidden license will cause the CI pipeline to fail.

#### go-licenses Tool

We use Google's `go-licenses` tool to:
- Check all dependencies against the license policy
- Generate license reports (CSV and JSON formats)
- Extract and save all third-party license files
- Generate NOTICE file for legal attribution

**Configuration**: `.github/workflows/license-compliance.yml`

### 3. Software Bill of Materials (SBOM)

SBOMs provide a complete inventory of all software components, dependencies, and metadata.

#### SBOM Formats

We generate SBOMs in three industry-standard formats:

1. **SPDX JSON** (`sbom-spdx.json`)
   - SPDX 2.3 specification
   - Machine-readable JSON format
   - Used by Grype for vulnerability scanning

2. **CycloneDX JSON** (`sbom-cyclonedx.json`)
   - CycloneDX 1.4 specification
   - Machine-readable JSON format
   - Widely supported by enterprise tools

3. **SPDX Tag-Value** (`sbom.spdx`)
   - SPDX 2.3 specification
   - Human-readable text format
   - Suitable for manual review

#### SBOM Generation with Syft

We use Anchore Syft to generate SBOMs:
- **Frequency**: Weekly scheduled + on every release
- **Configuration**: `.github/workflows/sbom-generation.yml`
- **Artifacts**: Attached to GitHub releases for all tagged versions

#### SBOM Vulnerability Scanning with Grype

After generating SBOMs, we scan them with Anchore Grype:
- Detects vulnerabilities in SBOM components
- Generates SARIF reports for GitHub Security tab
- Categorizes by severity (Critical, High, Medium, Low)
- **Configuration**: `.github/workflows/sbom-generation.yml`

### 4. Dependency Updates

#### Dependabot

Automated dependency updates via GitHub Dependabot:

**Configuration**: `.github/dependabot.yml`

**Update Schedule**: Weekly on Mondays at 6:00 UTC

**Managed Ecosystems**:
- Go modules (`go.mod`)
- GitHub Actions workflows
- Docker base images
- Helm chart dependencies

**Grouping Strategy**:
- Kubernetes packages grouped together
- AWS SDK packages grouped together
- Azure SDK packages grouped together
- Testing tools grouped together
- Observability tools grouped together

**Benefits**:
- Automatic PRs for dependency updates
- Security vulnerability alerts
- Release notes and changelogs
- Configurable auto-merge for patch updates

#### Dependency Review

For pull requests, we use GitHub's Dependency Review action:
- Compares dependencies between base and PR branches
- Blocks PRs introducing HIGH severity vulnerabilities
- Checks license compliance
- Adds summary comment to PR

**Configuration**: `.github/workflows/dependency-scan.yml`

### 5. Outdated Dependency Monitoring

Weekly checks identify dependencies with available updates:
- Compares current versions with latest available
- Generates report of outdated dependencies
- Available as workflow artifact
- **Configuration**: `.github/workflows/dependency-scan.yml`

## Local Development

### Running Scans Locally

All supply chain security checks can be run locally using Make targets:

```bash
# Run all vulnerability scanners
make deps-scan

# Individual scanners
make deps-scan-govulncheck
make deps-scan-nancy
make deps-scan-trivy

# License compliance
make license-check
make license-report
make license-save
make license-notice

# SBOM generation and scanning
make sbom-generate
make sbom-scan

# Check for outdated dependencies
make deps-outdated

# Run ALL supply chain security checks
make supply-chain-all
```

### Tool Installation

Required tools for local development:

```bash
# Go vulnerability checker (built-in to Go 1.18+)
go install golang.org/x/vuln/cmd/govulncheck@latest

# Nancy (Sonatype OSS Index)
# Download from: https://github.com/sonatype-nexus-community/nancy/releases

# Trivy
# Install from: https://trivy.dev/

# go-licenses
go install github.com/google/go-licenses@latest

# Syft (SBOM generator)
# Install from: https://github.com/anchore/syft

# Grype (SBOM scanner)
# Install from: https://github.com/anchore/grype
```

## CI/CD Integration

### Workflows

Three dedicated workflows handle supply chain security:

1. **dependency-scan.yml**
   - Runs multiple vulnerability scanners
   - Checks for outdated dependencies
   - Creates security summary
   - **Triggers**: Push to main/develop, PRs, weekly schedule

2. **license-compliance.yml**
   - Validates license policy compliance
   - Generates license reports
   - Creates NOTICE file for attribution
   - **Triggers**: Push to main/develop, PRs, weekly schedule

3. **sbom-generation.yml**
   - Generates SBOMs in multiple formats
   - Scans SBOMs for vulnerabilities
   - Attaches SBOMs to releases
   - **Triggers**: Push to main, releases, weekly schedule

### GitHub Security Integration

Results are integrated with GitHub Security:

- **Security tab**: View all vulnerability alerts
- **Dependabot alerts**: Automated security advisories
- **SARIF uploads**: Snyk, Trivy, and Grype results
- **Dependency graph**: Full dependency tree visualization

## Secrets Management

Required secrets in GitHub repository settings:

| Secret | Purpose | Required For |
|--------|---------|--------------|
| `SNYK_TOKEN` | Snyk authentication | Snyk vulnerability scanning |
| `GITHUB_TOKEN` | Built-in GitHub token | Dependency review, SBOM publishing |

## Compliance Reports

### Available Reports

All scans generate downloadable artifacts:

1. **Vulnerability Reports**
   - `snyk-report.json` - Snyk vulnerability data
   - `nancy-report.json` - Nancy vulnerability data
   - `govulncheck-report.json` - Go vulncheck results
   - `trivy-report.json` - Trivy scan results
   - `grype-report.json` - Grype SBOM scan results

2. **License Reports**
   - `licenses.csv` - License compliance report (CSV)
   - `licenses.json` - License compliance report (JSON)
   - `NOTICE` - Legal attribution file
   - `third-party-licenses/` - All license files

3. **SBOM Artifacts**
   - `sbom-spdx.json` - SPDX JSON SBOM
   - `sbom-cyclonedx.json` - CycloneDX JSON SBOM
   - `sbom.spdx` - SPDX tag-value SBOM

4. **Dependency Information**
   - `outdated.txt` - List of outdated dependencies
   - `dependency-graph.md` - Dependency tree

### Retention

- Vulnerability reports: 30 days
- License reports: 30 days
- SBOM artifacts: 90 days
- NOTICE file: 90 days
- Outdated dependencies: 7 days

## Response Process

### Critical Vulnerability Found

1. **Automated Detection**: Workflow identifies vulnerability
2. **Security Alert**: GitHub creates security advisory
3. **Assessment**: Team evaluates severity and impact
4. **Remediation**:
   - Update dependency if patch available
   - Apply workaround if no patch exists
   - Document mitigation strategy
5. **Verification**: Re-run scans to confirm fix
6. **Documentation**: Update CHANGELOG and security advisories

### License Violation Found

1. **Automated Detection**: License compliance check fails
2. **Investigation**: Identify offending dependency
3. **Options**:
   - Find alternative dependency with acceptable license
   - Request license exception (rare, requires legal review)
   - Implement functionality ourselves
4. **Update**: Remove or replace dependency
5. **Verification**: Re-run license compliance checks

## Best Practices

### For Developers

1. **Before Adding Dependencies**:
   - Check license compatibility
   - Review security history
   - Verify active maintenance
   - Run `make license-check` locally

2. **Dependency Updates**:
   - Review Dependabot PRs promptly
   - Test updates in development environment
   - Check for breaking changes
   - Update documentation if needed

3. **Security Alerts**:
   - Address high/critical alerts within 7 days
   - Document mitigation if no fix available
   - Test fixes before merging

### For Maintainers

1. **Weekly Reviews**:
   - Review scheduled scan results
   - Triage new vulnerability alerts
   - Update dependency policy if needed

2. **Release Process**:
   - Verify all scans pass
   - Include SBOM with release
   - Document known issues in release notes

3. **Policy Updates**:
   - Review license policy quarterly
   - Update approved/forbidden lists as needed
   - Communicate policy changes to team

## Resources

### Documentation

- [SPDX Specification](https://spdx.dev/specifications/)
- [CycloneDX Specification](https://cyclonedx.org/specification/overview/)
- [Go Vulnerability Database](https://vuln.go.dev/)
- [Sonatype OSS Index](https://ossindex.sonatype.org/)
- [Snyk Vulnerability Database](https://security.snyk.io/)

### Tools

- [Syft](https://github.com/anchore/syft) - SBOM generator
- [Grype](https://github.com/anchore/grype) - SBOM vulnerability scanner
- [Trivy](https://trivy.dev/) - Universal security scanner
- [go-licenses](https://github.com/google/go-licenses) - License compliance tool
- [Nancy](https://github.com/sonatype-nexus-community/nancy) - Dependency vulnerability scanner

### Standards

- [OpenSSF Best Practices](https://bestpractices.coreinfrastructure.org/)
- [SLSA Framework](https://slsa.dev/)
- [NIST Secure Software Development Framework](https://csrc.nist.gov/projects/ssdf)

## Continuous Improvement

This supply chain security program is continuously evolving. We regularly:

- Evaluate new security tools and practices
- Update policies based on industry standards
- Incorporate feedback from security audits
- Enhance automation and reporting

For questions or suggestions, please open a GitHub issue or contact the security team.
