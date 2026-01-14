# Release vX.Y.Z

**Release Date**: YYYY-MM-DD

## Overview

[Brief 2-3 sentence description of this release, highlighting the most important changes]

## Highlights

- ✨ **[Major new feature]** - Brief description
- 🐛 **[Important bug fix]** - What was fixed
- 📈 **[Performance improvement]** - What improved
- 🔒 **[Security enhancement]** - Security improvement

---

## Breaking Changes

⚠️ **Important**: This release contains breaking changes. Please review carefully before upgrading.

### API Changes

**Changed:** `[endpoint/field]`
- **Before**: `old_behavior`
- **After**: `new_behavior`
- **Impact**: Who is affected and how
- **Migration**: Step-by-step migration instructions

**Removed:** `[endpoint/field]`
- **Reason**: Why it was removed
- **Alternative**: What to use instead
- **Migration**: How to migrate existing code

### Configuration Changes

**Changed:** `[config_option]`
- **Before**: `old_value_format`
- **After**: `new_value_format`
- **Migration**: Update configuration files:
  ```yaml
  # Before
  old_config: value

  # After
  new_config: new_value
  ```

### Behavioral Changes

- **[Component]**: Behavior changed from X to Y
  - **Impact**: Effects on existing deployments
  - **Action Required**: What users need to do

---

## New Features

### Feature Category 1

- **[Feature Name]** (#123) - Description of what it does
  - Subfeature or detail
  - Configuration example if applicable

### Feature Category 2

- **[Feature Name]** (#124) - Description
- **[Feature Name]** (#125) - Description

---

## Enhancements

### Performance Improvements

- **[Component]** - Improvement description (#126)
  - Benchmark: Before: X ops/s, After: Y ops/s
- **Caching optimization** - Reduced memory usage by 30% (#127)

### API Enhancements

- **[Endpoint]** - Added support for X (#128)
- **[Endpoint]** - Improved response time (#129)

### Backend Adapters

- **Kubernetes adapter** - Added support for K8s 1.31 (#130)
- **AWS adapter** - Improved EKS cluster detection (#131)

### Developer Experience

- **CLI improvements** - Better error messages (#132)
- **Documentation** - Added comprehensive guides (#133)

---

## Bug Fixes

### Critical Fixes

- **[Component]** - Fixed critical issue that caused X (#134)
- **Subscription delivery** - Fixed race condition in webhook delivery (#135)

### General Fixes

- **[Component]** - Fixed minor issue with Y (#136)
- **[Component]** - Corrected Z behavior (#137)
- **Error handling** - Better error messages for invalid input (#138)

### Security Fixes

- **Authentication** - Fixed potential auth bypass (CVE-YYYY-XXXXX) (#139)
- **Input validation** - Enhanced XSS protection (#140)

---

## Performance Improvements

- **API latency** - Reduced p95 latency from 150ms to 80ms (#141)
- **Memory usage** - Reduced baseline memory by 25% (#142)
- **Cache hit ratio** - Improved from 85% to 92% with new caching strategy (#143)
- **Subscription processing** - 3x faster event delivery (#144)

**Benchmarks:**

| Metric | v1.1.0 | v1.2.0 | Improvement |
|--------|--------|--------|-------------|
| Requests/sec | 1000 | 1500 | +50% |
| p95 latency | 150ms | 80ms | -47% |
| Memory usage | 800MB | 600MB | -25% |
| Cache hit ratio | 85% | 92% | +8% |

---

## Security

### Security Fixes

- **CVE-YYYY-XXXXX** - Fixed authentication bypass vulnerability (#145)
  - **Severity**: Critical
  - **Impact**: All versions < v1.2.0
  - **Action**: Upgrade immediately

### Security Enhancements

- **TLS 1.3** - Now enforced by default (#146)
- **Webhook signatures** - Added HMAC-SHA256 signing (#147)
- **Rate limiting** - Enhanced DDoS protection (#148)
- **Secrets management** - Integration with external secret managers (#149)

### Security Scanning

- ✅ No critical vulnerabilities (gosec, govulncheck)
- ✅ Container image scanned (Trivy)
- ✅ SBOM attached to release
- ✅ Dependency review completed

---

## Dependencies

### Updated

- **k8s.io/client-go**: v0.34.0 → v0.35.0
  - Adds support for Kubernetes 1.31
  - Breaking: Requires Go 1.25+
- **go.uber.org/zap**: v1.26.0 → v1.27.0
  - Performance improvements
- **redis/go-redis**: v9.0.5 → v9.1.0
  - Bug fixes

### Added

- **github.com/example/newlib**: v1.5.0
  - Required for new feature X

### Removed

- **github.com/deprecated/oldlib**: Removed unused dependency

---

## Deprecations

⚠️ **Deprecated features will be removed in v2.0.0**

### API Deprecations

- **Endpoint**: `GET /api/v1/old-endpoint`
  - **Deprecated Since**: v1.2.0
  - **Removal**: v2.0.0
  - **Alternative**: Use `GET /api/v2/new-endpoint`
  - **Migration Guide**: [Link to docs]

### Configuration Deprecations

- **Config option**: `old_config_field`
  - **Deprecated Since**: v1.2.0
  - **Removal**: v2.0.0
  - **Alternative**: `new_config_field`

---

## Known Issues

### Critical Issues

None

### Non-Critical Issues

- **[Component]** - Known issue with X under specific conditions (#150)
  - **Impact**: Only affects Y scenario
  - **Workaround**: Do Z
  - **Fix**: Planned for v1.2.1

- **[Component]** - Minor UI glitch in Z (#151)
  - **Impact**: Visual only, no functional impact
  - **Fix**: Planned for v1.3.0

---

## O-RAN Compliance

### Compliance Status

- **O2-IMS v3.0.0**: 95% compliant (unchanged)
- **O2-DMS v3.0.0**: 95% compliant (+5% from v1.1.0)
- **O2-SMO v3.0.0**: 90% compliant (unchanged)

### New Compliance Features

- Implemented O2-DMS endpoint: `POST /deployments/{id}/scale` (#152)
- Added O2-IMS filter support for complex queries (#153)

See [Implementation Status](docs/IMPLEMENTATION_STATUS.md) for details.

---

## Upgrade Guide

### Upgrade Path

**Supported Upgrades:**
- ✅ v1.1.x → v1.2.0 (tested)
- ✅ v1.0.x → v1.2.0 (tested)
- ⚠️ v0.x.x → v1.2.0 (not recommended, upgrade to v1.1.0 first)

### Prerequisites

**Before upgrading:**
- [ ] Backup Redis data
- [ ] Backup configuration
- [ ] Review breaking changes above
- [ ] Test upgrade in non-production environment

**Minimum Requirements:**
- Go 1.25.0+ (⚠️ **Changed from 1.24**)
- Kubernetes 1.28+
- Redis 7.4+

### Step-by-Step Upgrade

#### From v1.1.x

**1. Update Configuration**

```yaml
# config.yaml - Required changes
redis:
  # NEW: Connection pool configuration
  pool_size: 100
  min_idle_conns: 10

server:
  # CHANGED: TLS now requires minimum version
  tls_min_version: "1.3"
```

**2. Update Kubernetes Manifests**

```yaml
# Update image version
image: ghcr.io/piwi3910/netweave:v1.2.0

# Update resource limits (recommended)
resources:
  limits:
    memory: 2Gi  # Increased from 1Gi
```

**3. Apply Migration**

```bash
# No database migrations required for this release
```

**4. Rolling Update**

```bash
# Helm upgrade
helm upgrade netweave netweave/netweave --version 1.2.0

# Or kubectl
kubectl set image deployment/netweave-gateway \
  gateway=ghcr.io/piwi3910/netweave:v1.2.0

# Monitor rollout
kubectl rollout status deployment/netweave-gateway
```

**5. Verify Upgrade**

```bash
# Check version
kubectl exec deployment/netweave-gateway -- gateway version

# Check health
curl https://gateway:8443/healthz

# Verify API functionality
curl https://gateway:8443/o2ims/v1/resourcePools
```

#### From v1.0.x

Follow the v1.1.0 upgrade guide first, then upgrade to v1.2.0.

See [Upgrade Guide](docs/operations/upgrades.md) for detailed instructions.

---

## Rollback Procedure

If issues occur after upgrade:

```bash
# Helm rollback
helm rollback netweave

# Or kubectl rollback
kubectl rollout undo deployment/netweave-gateway

# Verify rollback
kubectl rollout status deployment/netweave-gateway
```

**Note**: Rollback is safe for v1.1.x → v1.2.0 (no database schema changes)

---

## Documentation

### New Documentation

- [Performance Tuning Guide](docs/operations/performance.md)
- [FAQ](docs/reference/faq.md)
- [Release Process](docs/release-process.md)

### Updated Documentation

- [API Reference](docs/api/README.md) - Added v2 endpoints
- [Configuration Guide](docs/configuration/README.md) - New options
- [Troubleshooting Guide](docs/operations/troubleshooting.md) - New scenarios

---

## Contributors

Thank you to all contributors who made this release possible:

- @contributor1 - Feature X (#123)
- @contributor2 - Bug fix Y (#135)
- @contributor3 - Documentation improvements (#133)

**Full Changelog**: https://github.com/piwi3910/netweave/compare/v1.1.0...v1.2.0

---

## Downloads

### Binaries

- [Linux AMD64](https://github.com/piwi3910/netweave/releases/download/v1.2.0/netweave-linux-amd64.tar.gz)
- [Linux ARM64](https://github.com/piwi3910/netweave/releases/download/v1.2.0/netweave-linux-arm64.tar.gz)
- [macOS AMD64](https://github.com/piwi3910/netweave/releases/download/v1.2.0/netweave-darwin-amd64.tar.gz)
- [macOS ARM64](https://github.com/piwi3910/netweave/releases/download/v1.2.0/netweave-darwin-arm64.tar.gz)
- [Checksums](https://github.com/piwi3910/netweave/releases/download/v1.2.0/checksums.txt)

### Container Images

```bash
docker pull ghcr.io/piwi3910/netweave:v1.2.0
docker pull ghcr.io/piwi3910/netweave:v1.2
docker pull ghcr.io/piwi3910/netweave:v1
docker pull ghcr.io/piwi3910/netweave:latest
```

### Helm Chart

```bash
helm repo add netweave https://piwi3910.github.io/netweave
helm repo update
helm install netweave netweave/netweave --version 1.2.0
```

### SBOM

- [Software Bill of Materials (JSON)](https://github.com/piwi3910/netweave/releases/download/v1.2.0/sbom.json)

---

## Support

- 📖 [Documentation](https://github.com/piwi3910/netweave/tree/main/docs)
- 💬 [GitHub Discussions](https://github.com/piwi3910/netweave/discussions)
- 🐛 [Report Issues](https://github.com/piwi3910/netweave/issues)
- 📧 [Security Issues](mailto:security@example.com)
