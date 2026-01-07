# O-RAN Compliance Checker

Automated O-RAN Alliance specification compliance validation for netweave gateway.

## Overview

This tool validates netweave's compliance with O-RAN specifications by testing API endpoints and generating compliance reports and badges.

## Quick Start

```bash
# Build the compliance checker
go build -o ../../build/compliance ./cmd/compliance

# Run compliance check (requires running gateway)
./build/compliance -url http://localhost:8080

# Generate compliance badges for README
make compliance-badges

# Update README.md with compliance badges
make compliance-update-readme
```

## What It Tests

### O2-IMS v3.0.0 (Infrastructure Management Services)

- ✅ Subscription management (GET/POST/DELETE `/o2ims/v1/subscriptions`)
- ✅ Resource pool management (GET `/o2ims/v1/resourcePools`)
- ✅ Resource management (GET `/o2ims/v1/resources`)
- ✅ Resource type discovery (GET `/o2ims/v1/resourceTypes`)
- ✅ Deployment manager metadata (GET `/o2ims/v1/deploymentManagers`)
- ✅ O-Cloud information (GET `/o2ims/v1/oCloudInfrastructure`)

### O2-DMS v3.0.0 (Deployment Management Services)

- 📋 Deployment package management
- 📋 Deployment lifecycle (create, update, delete)
- 📋 Scaling and rollback operations
- 📋 Deployment status and logs

### O2-SMO v3.0.0 (Service Management & Orchestration)

- ✅ Unified subscription system
- ✅ Webhook notification delivery
- ✅ Event filtering
- ✅ API discovery endpoints

## Usage

### Run Compliance Check

```bash
# Against local gateway
./build/compliance -url http://localhost:8080

# Against remote gateway
./build/compliance -url https://netweave.example.com

# With verbose output
./build/compliance -url http://localhost:8080 -v
```

### Generate Reports

```bash
# Text report (default)
./build/compliance -url http://localhost:8080 -output text

# JSON report
./build/compliance -url http://localhost:8080 -output json > report.json

# Badges for README
./build/compliance -url http://localhost:8080 -output badges
```

### Update README

```bash
# Automatically update README.md with compliance badges
./build/compliance -url http://localhost:8080 -update-readme

# Specify custom README path
./build/compliance -url http://localhost:8080 -update-readme -readme ./docs/README.md
```

## Output Formats

### Text Report

```
O-RAN Specification Compliance Report
=====================================

## O2-IMS v3.0.0

Specification URL: https://specifications.o-ran.org/...
Compliance Level: full
Compliance Score: 100.0%
Endpoints Tested: 15
Endpoints Passed: 15
Endpoints Failed: 0
Tested At: 2026-01-06 12:00:00 UTC
```

### JSON Report

```json
[
  {
    "specName": "O2-IMS",
    "specVersion": "v3.0.0",
    "specUrl": "https://specifications.o-ran.org/...",
    "complianceLevel": "full",
    "complianceScore": 100.0,
    "totalEndpoints": 15,
    "passedEndpoints": 15,
    "failedEndpoints": 0,
    "missingFeatures": [],
    "testedAt": "2026-01-06T12:00:00Z"
  }
]
```

### Badge Format

Generates shields.io badges with automatic color coding:

- **Green**: 100% compliance (full)
- **Yellow**: 80-99% compliance (partial)
- **Red**: <80% compliance (none)

Example badge:
```
[![O-RAN O2-IMS v3.0.0 Compliance](https://img.shields.io/badge/O--RAN__O2--IMS-v3.0.0__compliant-brightgreen)](https://specifications.o-ran.org/...)
```

## Makefile Targets

Convenient make targets are available in the project root:

```bash
make compliance-check          # Run compliance check
make compliance-badges          # Generate badges
make compliance-json            # Generate JSON report
make compliance-update-readme   # Update README with badges
```

## CI/CD Integration

### GitHub Actions

Add to your workflow:

```yaml
- name: Run Compliance Check
  run: |
    make build
    ./build/netweave &
    sleep 5
    make compliance-check

- name: Generate Compliance Report
  run: make compliance-json

- name: Upload Report
  uses: actions/upload-artifact@v4
  with:
    name: compliance-report
    path: build/reports/compliance.json
```

### Pre-Commit Hook

Add to `.pre-commit-config.yaml`:

```yaml
- repo: local
  hooks:
    - id: compliance-check
      name: O-RAN Compliance
      entry: make compliance-check
      language: system
      pass_filenames: false
```

## Development

### Adding New Endpoint Tests

Edit `checker.go` to add new endpoint tests:

```go
endpoints := []EndpointTest{
    {
        Method:         "GET",
        Path:           "/o2ims/v1/newEndpoint",
        RequiredStatus: http.StatusOK,
    },
}
```

### Testing the Compliance Checker

```bash
# Run unit tests
go test -v ./tools/compliance/...

# Run with coverage
go test -v -coverprofile=coverage.out ./tools/compliance/...
go tool cover -html=coverage.out
```

## Architecture

```
cmd/compliance/main.go          # CLI entry point
tools/compliance/
├── checker.go                   # Core compliance validation logic
├── badge.go                     # Badge generation
├── checker_test.go              # Unit tests
└── badge_test.go                # Badge tests
```

## Compliance Levels

The tool assigns compliance levels based on endpoint coverage:

| Level | Coverage | Badge Color |
|-------|----------|-------------|
| Full | 100% | Green |
| Partial | 80-99% | Yellow |
| None | <80% | Red |

## Troubleshooting

### Gateway Not Running

**Error:** `connection refused`

**Solution:** Start the gateway before running compliance check:
```bash
make build
./build/netweave &
make compliance-check
```

### Endpoint Not Found (404)

**Expected Behavior:** GET endpoints returning 404 for non-existent resources is acceptable and indicates the endpoint is implemented.

**Actual Problem:** If ALL endpoints return 404, the gateway may not be configured correctly.

### Low Compliance Score

1. Check which endpoints failed: `./build/compliance -v`
2. Review missing features in the output
3. Verify gateway is fully started and healthy
4. Check [docs/api-mapping.md](../../docs/api-mapping.md) for endpoint status

## References

- [Compliance Documentation](../../docs/compliance.md)
- [O-RAN Specifications](https://specifications.o-ran.org/)
- [API Mapping Guide](../../docs/api-mapping.md)
- [Architecture Overview](../../docs/architecture.md)

## License

Apache License 2.0 - See [LICENSE](../../LICENSE) for details.
