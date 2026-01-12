# Reference Documentation

Technical reference materials, specifications, and lookup tables for netweave development and operations.

## Overview

This section contains:
- O-RAN specification compliance details
- Glossary of terms and acronyms
- Error code reference with solutions
- Technical specifications

## Contents

### [O-RAN Compliance](compliance.md)

Detailed compliance status with O-RAN Alliance specifications:
- O2-IMS v3.0.0 compliance (100%)
- O2-DMS v3.0.0 compliance status
- O2-SMO v3.0.0 compliance (100%)
- Compliance validation tools
- API endpoint coverage

**Use this when:**
- Verifying O-RAN spec compliance
- Understanding which endpoints are implemented
- Running compliance checks
- Validating against standards

### [Glossary](glossary.md)

Comprehensive glossary of terms, acronyms, and concepts:
- O-RAN terminology
- Kubernetes concepts
- netweave-specific terms
- Industry acronyms
- Technical definitions

**Use this when:**
- Learning O-RAN concepts
- Understanding unfamiliar acronyms
- Writing documentation
- Onboarding new team members

### [Error Code Reference](error-codes.md)

Detailed error codes with troubleshooting guidance:
- HTTP status codes
- Application error codes
- Adapter-specific errors
- Common issues and solutions
- Debugging strategies

**Use this when:**
- Troubleshooting errors
- Understanding error messages
- Debugging API failures
- Writing error handling code

## Quick Reference

### Common O-RAN Terms

| Term | Definition |
|------|------------|
| **O-Cloud** | Cloud infrastructure hosting O-RAN functions |
| **O-DU** | O-RAN Distributed Unit |
| **O-CU** | O-RAN Centralized Unit |
| **SMO** | Service Management & Orchestration |
| **O2-IMS** | O2 Infrastructure Management Services |
| **O2-DMS** | O2 Deployment Management Services |

See [Glossary](glossary.md) for complete definitions.

### Common Error Codes

| Code | Meaning | Solution |
|------|---------|----------|
| **404** | Resource not found | Verify resource ID exists |
| **409** | Conflict (duplicate) | Use different identifier |
| **500** | Internal server error | Check logs for details |
| **503** | Service unavailable | Check backend connectivity |

See [Error Codes](error-codes.md) for complete reference.

### API Compliance Status

| API | Status | Coverage |
|-----|--------|----------|
| **O2-IMS** | ✅ Full | 100% |
| **O2-DMS** | 🟡 Partial | ~30% |
| **O2-SMO** | ✅ Full | 100% |

See [Compliance](compliance.md) for detailed breakdown.

## Using This Section

### For Developers

When developing:
1. **Check compliance** before implementing features
2. **Use glossary** for consistent terminology
3. **Reference error codes** when handling errors
4. **Verify against specs** during code review

### For Operators

When troubleshooting:
1. **Look up error codes** in reference
2. **Check compliance status** for feature availability
3. **Use glossary** to understand log messages
4. **Consult specs** for expected behavior

### For Architects

When designing:
1. **Review compliance** for feature planning
2. **Check terminology** for consistency
3. **Understand error patterns** for error handling design
4. **Validate against specs** for standards alignment

## Contributing to Reference

### Adding New Terms

1. Check glossary for existing definition
2. Add alphabetically in appropriate section
3. Include acronym expansion
4. Provide clear, concise definition
5. Link to related terms

### Updating Error Codes

1. Document in error-codes.md
2. Include HTTP status code
3. Provide clear description
4. Add troubleshooting steps
5. Link to related documentation

### Compliance Updates

1. Run compliance checker
2. Update compliance.md
3. Regenerate badges
4. Update README.md
5. Commit with verification

## External References

### O-RAN Alliance

- **[O-RAN Specifications](https://specifications.o-ran.org/)** - Official specs
- **[O-RAN ALLIANCE](https://www.o-ran.org/)** - Alliance website
- **[O-RAN Software Community](https://o-ran-sc.org/)** - Open source projects

### Kubernetes

- **[Kubernetes Docs](https://kubernetes.io/docs/)** - Official documentation
- **[API Reference](https://kubernetes.io/docs/reference/kubernetes-api/)** - K8s API
- **[client-go](https://github.com/kubernetes/client-go)** - Go client library

### Standards

- **[RFC 7807](https://tools.ietf.org/html/rfc7807)** - Problem Details for HTTP APIs
- **[OpenAPI 3.0](https://swagger.io/specification/)** - API specification format
- **[JSON:API](https://jsonapi.org/)** - JSON API specification

## Staying Current

Reference documentation is updated:
- **Compliance:** After each release
- **Glossary:** As new terms are introduced
- **Error Codes:** When new errors are added
- **Specifications:** When O-RAN releases new versions

Check [CHANGELOG.md](../../CHANGELOG.md) for recent updates.

---

**For questions or corrections, please [open an issue](https://github.com/piwi3910/netweave/issues/new).**
