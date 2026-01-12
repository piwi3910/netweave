# Architecture Decision Records (ADRs)

This document tracks major architecture and design decisions for netweave.

## Table of Contents

- [ADR Format](#adr-format)
- [Active ADRs](#active-adrs)
- [Superseded ADRs](#superseded-adrs)

## ADR Format

Each ADR follows this structure:

```
# ADR-NNN: Title

**Status:** Proposed | Accepted | Deprecated | Superseded by ADR-XXX
**Date:** YYYY-MM-DD
**Deciders:** Names or roles
**Tags:** architecture, performance, security, etc.

## Context

What is the issue that we're seeing motivating this decision or change?

## Decision

What is the change that we're proposing and/or doing?

## Consequences

### Positive
- What becomes easier or better?

### Negative
- What becomes more difficult or worse?

### Neutral
- What stays the same or is neither good nor bad?

## Alternatives Considered

### Alternative 1: Name
- Description
- Why not chosen

### Alternative 2: Name
- Description
- Why not chosen

## Implementation Notes

Key points for implementing this decision.

## References

- Links to relevant documentation, discussions, or PRs
```

---

## Active ADRs

### ADR-001: Use Go 1.25.0 as Minimum Version

**Status:** Accepted
**Date:** 2026-01-06
**Deciders:** Core team
**Tags:** dependencies, compatibility

#### Context

The project requires Kubernetes client libraries (k8s.io/client-go) for infrastructure management. Recent versions have strict Go version requirements and dependency constraints.

#### Decision

Set minimum Go version to 1.25.0 and use k8s.io/client-go v0.35.0.

#### Consequences

**Positive:**
- Resolves yaml.v3 module path conflicts present in k8s.io v0.31-v0.34
- Eliminates golang.org/x/net version conflicts
- Provides latest security patches and performance improvements
- Future-proofs against upcoming K8s API changes

**Negative:**
- Requires developers to upgrade to Go 1.25.0+
- May cause compatibility issues with older tooling
- Increases minimum system requirements

**Neutral:**
- Go 1.25.0 released in 2026, widely available
- Toolchain auto-management handles version switching

#### Alternatives Considered

**Alternative 1: Use Go 1.23 with k8s.io v0.31**
- Would have lower system requirements
- Not chosen due to yaml.v3 type mismatch errors and missing bug fixes

**Alternative 2: Use Go 1.24 with k8s.io v0.34**
- Would work but still has some dependency conflicts
- Not chosen because it's a halfway solution with no clear benefits

#### Implementation Notes

- Update go.mod: `go 1.25.0`
- Update CI workflows to use Go 1.25.0
- Update documentation with new requirements
- Add version check to Makefile

#### References

- [Issue #42](https://github.com/piwi3910/netweave/issues/42)
- [k8s.io/client-go release notes](https://github.com/kubernetes/client-go/releases)

---

### ADR-002: Zero-Tolerance Linting Policy

**Status:** Accepted
**Date:** 2026-01-05
**Deciders:** Core team
**Tags:** quality, standards, developer-experience

#### Context

Code quality issues accumulate over time if not strictly enforced. Linter warnings are often ignored with `//nolint` comments, leading to technical debt.

#### Decision

Enforce zero-tolerance linting policy: ALL code must pass ALL linters with ZERO warnings. No `//nolint` directives allowed except in rare, documented cases.

#### Consequences

**Positive:**
- Consistent, high-quality codebase
- Catches bugs early (null checks, error handling)
- Forces developers to fix root causes, not suppress warnings
- Builds good habits and code patterns
- Easier code reviews (less subjective feedback)

**Negative:**
- Steeper learning curve for new contributors
- Can slow initial development
- Requires more time fixing complex linting issues
- May feel restrictive to experienced developers

**Neutral:**
- 50+ linters enforced via golangci-lint
- Pre-commit hooks auto-enforce standards

#### Alternatives Considered

**Alternative 1: Allow `//nolint` with required justification**
- More flexible for edge cases
- Not chosen because justifications become excuses over time

**Alternative 2: Only enforce critical linters (gosec, govet)**
- Easier to adopt initially
- Not chosen because leads to inconsistent style and quality

#### Implementation Notes

- Configure golangci-lint with all desired linters
- Add pre-commit hook to run linters
- CI pipeline fails on ANY linting warning
- Documentation on how to fix common linting issues

#### References

- [CLAUDE.md Code Quality Standards](../../CLAUDE.md#code-quality-standards)
- [golangci-lint configuration](.golangci.yml)

---

### ADR-003: Redis for State and Caching

**Status:** Accepted
**Date:** 2025-12-20
**Deciders:** Architecture team
**Tags:** storage, performance, scalability

#### Context

Gateway needs distributed state storage for:
- Subscriptions (persistent)
- Performance caching (ephemeral)
- Rate limiting (distributed counters)
- Inter-pod coordination (pub/sub)

Multiple backends support stateless gateway pods.

#### Decision

Use Redis OSS 7.4+ with Sentinel for HA as the primary state store and cache.

#### Consequences

**Positive:**
- Proven, battle-tested technology
- Sub-millisecond latency for cache operations
- Built-in pub/sub for event coordination
- Sentinel provides automatic failover
- Rich data structures (strings, hashes, sorted sets)
- Easy to operate and monitor

**Negative:**
- Single point of failure (mitigated by Sentinel)
- Memory-bound (all data in RAM)
- Additional operational complexity
- Not ACID compliant (eventual consistency)

**Neutral:**
- Redis Cluster for horizontal scaling (future)
- Disk persistence optional (AOF/RDB)

#### Alternatives Considered

**Alternative 1: etcd**
- Strong consistency guarantees
- Not chosen due to higher latency and operational complexity

**Alternative 2: PostgreSQL**
- Full ACID compliance
- Not chosen due to slower performance for caching use cases

**Alternative 3: In-memory (no external storage)**
- Simplest deployment
- Not chosen because doesn't work with multiple gateway pods

#### Implementation Notes

- Deploy Redis with Sentinel (3 sentinels, 1 master, 2 replicas)
- Use separate Redis databases for different concerns:
  - DB 0: Subscriptions
  - DB 1: Performance cache
  - DB 2: Rate limiting
- Configure TTL appropriately per use case
- Monitor with redis_exporter for Prometheus

#### References

- [Architecture Documentation](../architecture.md)
- [Redis Sentinel Documentation](https://redis.io/topics/sentinel)

---

### ADR-004: Adapter Pattern for Multi-Backend Support

**Status:** Accepted
**Date:** 2025-12-15
**Deciders:** Architecture team
**Tags:** architecture, extensibility

#### Context

Gateway needs to support multiple infrastructure backends (Kubernetes, OpenStack, AWS, Azure, GCP, VMware, bare-metal) with a unified O2-IMS API interface.

#### Decision

Implement adapter pattern with well-defined interfaces for each O2 API type (O2-IMS, O2-DMS, O2-SMO).

#### Consequences

**Positive:**
- Clean separation of concerns
- Easy to add new backends
- Testable in isolation (mock adapters)
- Follows O-RAN specification intent
- Enables vendor-agnostic API

**Negative:**
- Additional abstraction layer
- Performance overhead (minimal)
- Adapters must handle differences in backend capabilities
- More code to maintain (25+ adapters)

**Neutral:**
- Each adapter is independently versioned
- Adapter registration via plugin system

#### Alternatives Considered

**Alternative 1: Direct backend integration**
- Simpler initially
- Not chosen due to tight coupling and no multi-backend support

**Alternative 2: Proxy pattern with pass-through**
- Less code
- Not chosen because doesn't abstract differences between backends

#### Implementation Notes

- Define clear adapter interfaces (22 methods for O2-IMS)
- Each adapter in separate package
- Common adapter utilities in shared package
- Comprehensive adapter tests (unit + integration)
- Capability negotiation per adapter

#### References

- [Adapter Interface](../../internal/adapter/adapter.go)
- [Backend Plugins Documentation](../backend-plugins.md)

---

### ADR-005: Table-Driven Tests as Standard

**Status:** Accepted
**Date:** 2025-12-10
**Deciders:** Core team
**Tags:** testing, standards

#### Context

Test code makes up ~40% of the codebase. Inconsistent test patterns make tests harder to read and maintain.

#### Decision

Mandate table-driven tests for all business logic testing. Use AAA pattern (Arrange, Act, Assert) for test structure.

#### Consequences

**Positive:**
- Easy to add new test cases
- Clear, readable test structure
- Encourages comprehensive coverage (happy path + edge cases)
- Self-documenting test scenarios
- Parallel execution with t.Run()

**Negative:**
- More verbose for simple tests
- Requires discipline to maintain structure
- Initial learning curve for contributors

**Neutral:**
- Works well with subtests
- Compatible with all testing libraries

#### Alternatives Considered

**Alternative 1: Individual test functions per scenario**
- More flexibility
- Not chosen due to code duplication and inconsistency

**Alternative 2: BDD-style tests (Ginkgo/Gomega)**
- More expressive syntax
- Not chosen to avoid additional dependencies

#### Implementation Notes

- Template in testing guidelines
- All new tests must use table-driven pattern
- Refactor existing tests over time
- Use testdata package for fixtures

#### References

- [Testing Guidelines](testing.md#table-driven-tests)
- [Go Wiki: Table Driven Tests](https://github.com/golang/go/wiki/TableDrivenTests)

---

## Superseded ADRs

### ADR-000: Example Superseded Decision

**Status:** Superseded by ADR-XXX
**Date:** 2025-01-01

*Brief explanation of why superseded and link to replacement.*

---

## How to Propose an ADR

1. **Create GitHub Issue:**
   ```bash
   gh issue create --title "ADR: Proposal title" --label "adr"
   ```

2. **Draft ADR:**
   - Copy ADR template
   - Fill in all sections
   - Get feedback in issue

3. **Discussion:**
   - Present in team meeting or PR
   - Address feedback
   - Reach consensus

4. **Merge:**
   - Update this file with new ADR
   - Mark as "Accepted"
   - Reference in commit message

5. **Implement:**
   - Follow decision
   - Update docs
   - Reference ADR in related code

## ADR Guidelines

**When to create an ADR:**
- Significant architectural changes
- Technology selection decisions
- Major breaking changes
- New patterns or standards
- Reversing previous decisions

**When NOT to create an ADR:**
- Bug fixes
- Feature implementations (unless architecturally significant)
- Routine maintenance
- Minor refactoring

**ADR Numbering:**
- Start at ADR-001
- Sequential numbering
- Never reuse numbers
- Superseded ADRs keep their number

---

**For questions about ADRs or proposing a new decision, create a GitHub issue with the `adr` label.**
