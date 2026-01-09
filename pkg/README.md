# Public Packages (pkg/)

This directory is reserved for **public, reusable libraries** that external projects can safely import and use.

## Purpose

The `pkg/` directory follows Go's standard project layout convention for packages intended to be consumed by external applications. Code placed here represents a **stable public API** with backward compatibility guarantees.

## Current Status

**Empty** - Currently, all functionality is internal to the netweave gateway. No public libraries have been extracted yet.

## When to Add Packages Here

Add packages to `pkg/` when:
1. **External consumers need the code** - Other projects will import these packages
2. **API is stable** - You're ready to commit to backward compatibility
3. **Well-documented** - Public packages require comprehensive documentation
4. **Standalone functionality** - The package can work independently of netweave internals

## Examples of Potential Public Packages

Future packages that might belong here:
- `pkg/o2ims/client` - O2-IMS API client library for SMO integrations
- `pkg/filters` - O2-IMS filter parsing and validation library
- `pkg/webhook` - Webhook notification client/server utilities
- `pkg/metrics` - Prometheus metrics helpers for O2-IMS adapters

## Guidelines

### Before Adding a Package:
1. Ensure it's needed by external projects (not just netweave internal code)
2. Design a clean, minimal API surface
3. Write comprehensive documentation with examples
4. Add thorough unit tests (100% coverage recommended)
5. Follow semantic versioning for breaking changes

### Package Structure:
```
pkg/
├── example/
│   ├── client.go        # Main implementation
│   ├── client_test.go   # Comprehensive tests
│   ├── README.md        # Package documentation
│   └── examples/        # Usage examples
│       └── basic/
│           └── main.go
```

### Testing Requirements:
- **100% coverage** - Public packages must be thoroughly tested
- **Example tests** - Include `Example*` functions for godoc
- **Integration tests** - Test real-world usage scenarios
- **Fuzz tests** - For parsers and input validation

## Alternative: Internal Packages

If code is only needed within netweave, use `internal/` instead:
- `internal/` packages cannot be imported by external projects
- Provides encapsulation and freedom to refactor
- Most netweave code belongs in `internal/`

## References

- [Go Project Layout](https://github.com/golang-standards/project-layout)
- [Go Package Documentation](https://go.dev/doc/effective_go#package-names)
- [Semantic Versioning](https://semver.org/)

---

**Status**: This directory is intentionally empty and reserved for future public APIs.
