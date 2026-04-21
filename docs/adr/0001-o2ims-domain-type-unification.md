# ADR 0001: O2-IMS Domain Type Unification

- **Status:** Proposed
- **Date:** 2026-04-21
- **Deciders:** netweave core maintainers
- **Tracking issue:** [#484](https://github.com/piwi3910/netweave/issues/484) (H11)
- **Related work:** H12 / M20 subscription store unification ([#485](https://github.com/piwi3910/netweave/issues/485), [#535](https://github.com/piwi3910/netweave/issues/535))

## Context

netweave currently defines `ResourcePool`, `Resource`, `ResourceType`, `DeploymentManager`,
and `Subscription` shapes in four separate packages. Each shape was added independently as a
new consumer appeared, and each carries fields that matter for that consumer but would be
actively wrong or lossy in another:

| Package | Purpose | Distinctive fields |
|---------|---------|--------------------|
| `internal/adapter` | Polling-filter / adapter contract passed to backend adapters (Kubernetes, DTIaS, AWS, OSM, …) | `TenantID` on `ResourcePool` / `Resource` with `json:"-"` (internal isolation tag stripped from wire). Scalar-string `SubscriptionFilter`. |
| `internal/o2ims/models` | Thin REST wire-format types rendered directly by the O2-IMS HTTP handlers | Slice-valued `SubscriptionFilter` (`ResourcePoolID []string`, …). `Resource.Name`, `Resource.ParentID`. `ResourcePool.GlobalAssetID`. No `TenantID`, no timestamps. |
| `internal/storage` | Redis-persisted envelope; implements `encoding.BinaryMarshaler` / `BinaryUnmarshaler` for go-redis v9 | Adds `TenantID`, `BackendID`, `CreatedAt`, `UpdatedAt`. `SubscriptionID` is the JSON tag on field `ID` (alias). Scalar-string filter. |
| `internal/models` | Bound to gqlgen via `gqlgen.yml`; drives the GraphQL object graph | Adds `EventTypes`, `Labels`, `Capacity`, `UpdatedAt`. Carries `yaml` tags alongside `json`. Slice-valued filter. |

The same consumer gap exists for `DeploymentManager` (three shapes across `internal/adapter`,
`internal/o2ims/models`, `internal/models`, plus an interface of the same name at
`internal/dms/dms.go:271` and an SMO-plugin struct at `internal/smo/plugin.go`).

Request handlers spend real effort copying field-by-field between shapes (e.g.
`internal/server/routes.go` list / batch / TMForum paths). The prior investigation
([#484 comment](https://github.com/piwi3910/netweave/issues/484#issuecomment-4287442490))
found at least one case where the copy drops `TenantID` on a platform-admin list path,
creating the cross-tenant leak the parent issue flagged.

These are **not pure duplicates**. Collapsing them without an explicit plan would:

1. Break the stable O2-IMS REST wire format consumed by SMOs.
2. Invalidate every `Subscription` currently persisted in Redis under the `storage.Subscription`
   shape — `UnmarshalBinary` fails loudly on unknown required fields.
3. Regenerate the gqlgen object graph, breaking any downstream GraphQL schema clients (the
   CLI and any SMO GraphQL consumers) if field cardinality changes (scalar → list).
4. Subtly change TMForum TMF639/TMF638/TMF641 output, because the transform funnels every
   extension field through `imsadapter.ResourcePool.Extensions` and any renamed/moved field
   would silently stop appearing in TMForum responses.

## Decision drivers

1. **Backwards compatibility with the O2-IMS REST wire format.** The REST surface is the
   product and must not change field names, cardinality, or presence without a versioned API
   bump.
2. **Redis state durability.** Subscriptions persisted under the current `storage.Subscription`
   shape must continue to deserialize after a rolling deploy; no flag day.
3. **gqlgen regeneration cost.** `gqlgen.yml` binds resolvers to `internal/models`. Any rename
   triggers a regenerate + resolver-signature diff; we want that to be a single explicit step,
   not a side effect of a storage change.
4. **TMForum transform stability.** The TMF639 / TMF638 / TMF641 transforms observationally
   depend on `imsadapter.*` field layout and on the `Extensions` map contents; they are the
   hardest to audit by eye.
5. **Tenant-isolation correctness.** `TenantID` must survive every layer transition. Any
   unification that drops it from the canonical type would reintroduce the cross-tenant leak
   H11 flagged.
6. **Incremental mergeability.** A single 10k-line rename PR is unreviewable. The migration
   must land in a sequence of small, individually-revertable PRs, each gated by a regression
   test that fails if observable output changes.

## Options considered

### Option (a): Single canonical type in `pkg/o2ims/types`, used everywhere

One struct per resource (e.g. `types.ResourcePool`) used by handlers, adapters, storage, and
gqlgen. Extra fields (TenantID, BackendID, timestamps, EventTypes, Labels) are all promoted
to the canonical type and hidden from wire formats via `json:"-"` or via response-envelope
structs.

- **Pro:** No converters. One place to add a field. Lowest long-term maintenance.
- **Pro:** Kills field-copy loops in `internal/server/routes.go`.
- **Con:** Redis migration is mandatory — every persisted `Subscription` needs a read-path
  adapter or a background rewrite.
- **Con:** gqlgen binding points at a type with many fields the schema does not expose; this
  works, but requires careful resolver review so internal fields don't leak.
- **Con:** Scalar-vs-slice `SubscriptionFilter` has to be decided globally. Slice wins on
  expressiveness; scalar wins on existing REST clients. Either choice breaks some consumer
  unless we keep a DTO at the wire boundary, which partially defeats the point.
- **Con:** Highest single-PR blast radius. Every rollback is a production rollback.

### Option (b) — RECOMMENDED: Keep four types, share a core struct, converters at the seams

Extract the truly-common fields (`ResourcePoolID`, `Name`, `Description`, `OCloudID`,
`Extensions`, etc.) into a small `pkg/o2ims/types.ResourcePoolCore` struct. Each of the four
existing types embeds `ResourcePoolCore` and keeps its layer-specific fields (`TenantID`,
`CreatedAt`, `EventTypes`, `ParentID`, cardinality-varying filter, …). Converters live in
`pkg/o2ims/types/convert.go` and are the only place copy logic is allowed; `routes.go` calls
them instead of hand-copying fields.

- **Pro:** Zero wire-format change. Zero Redis-shape change. Zero gqlgen regeneration on
  landing.
- **Pro:** The core struct is the place a new common field lands; divergence between layers
  becomes a visible choice instead of a silent drift.
- **Pro:** Each layer can be migrated to use the core in its own PR. Converters are covered
  by the golden-file harness (see below) so any semantic drift is caught immediately.
- **Pro:** `TenantID` lives on the core (with `json:"-"`) and survives every conversion; the
  H11 cross-tenant-leak path is fixed as a side effect of the first migration step.
- **Con:** Four types still exist. Reviewers have to remember which one to use where.
- **Con:** Converters are a new surface to maintain. We accept this cost because it's
  bounded and test-covered.

### Option (c): Codegen from a single source of truth (OpenAPI or proto)

Author the canonical shape once in an `openapi.yaml` or `.proto` and generate Go structs
for all four layers (REST, adapter, storage, gqlgen) with a single `go generate`.

- **Pro:** Guaranteed drift-free. Easy to diff the schema over time.
- **Pro:** Opens the door to sharing types with non-Go SMO clients.
- **Con:** Large upfront investment; we'd need to re-author the existing fields in the
  schema language, add build tooling, and rework `gqlgen.yml`'s bindings.
- **Con:** Codegen output rarely matches hand-written idioms on the first pass (pointer vs
  value fields, embedded vs separate types, time representations). We'd spend the first
  several PRs fighting the generator instead of fixing the H11 semantic problem.
- **Con:** Does not itself solve the Redis-migration or TMForum-transform questions; those
  stay regardless of who authors the structs.

## Decision

**Adopt Option (b): shared-core-struct + layer-specific wrappers + converters.**

Rationale: it delivers the concrete value H11 asks for — no more hand-copied field lists,
`TenantID` cannot be dropped by a translator — without forcing a wire, storage, or schema
break. Option (a) is the right long-term shape, and Option (b) is a strict subset of the work
Option (a) would require, so nothing in this plan is wasted effort if we later decide to
collapse the wrappers. Option (c) remains available as a future phase once the type graph is
stable.

## Implementation plan

To be executed as a sequence of small PRs, each gated on the golden-file regression harness
landed alongside this ADR (see "Regression harness" below).

1. **Land the core structs.** Create `pkg/o2ims/types/core.go` with
   `ResourcePoolCore`, `ResourceCore`, `ResourceTypeCore`, `DeploymentManagerCore`,
   `SubscriptionCore`. No other package imports them yet. Unit-test field-for-field JSON
   round-trip.
2. **Embed the core in `internal/adapter`.** Change `adapter.ResourcePool` / `Resource` /
   `ResourceType` / `DeploymentManager` / `Subscription` to embed the matching core. Verify
   the golden harness still passes byte-for-byte. This is the first PR that exercises the
   harness in anger.
3. **Embed the core in `internal/o2ims/models`.** Same treatment. Keep the slice-valued
   `SubscriptionFilter` on the wrapper; do not promote it to the core yet.
4. **Embed the core in `internal/models` (gqlgen-bound).** Regenerate gqlgen; confirm no
   resolver signatures changed. Golden-file GraphQL output must be unchanged.
5. **Embed the core in `internal/storage` and add converters.** Add
   `pkg/o2ims/types/convert.go` with functions like `StorageToAdapter(s *storage.Subscription)
   *adapter.Subscription`. Replace every field-by-field copy in `internal/server/routes.go`
   with a converter call. Crucially: the converter propagates `TenantID` on every path,
   closing the H11 leak.
6. **Promote accidentally-divergent fields to the core.** Audit the wrappers for fields that
   exist in two layers but not the core (currently: `GlobalLocationID`, `GlobalAssetID`,
   `ParentID`). Move them to the core with a dedicated PR and golden-file diff review.
7. **Decide on `SubscriptionFilter` cardinality.** Separate PR. Propose: the core holds both
   scalar single-item and slice multi-item, with a normalizing accessor. REST wire format
   stays whatever it is today. This is the only item in the plan that might require a
   migration helper for persisted subscriptions.
8. **Delete the wrappers (optional, future).** Only if steps 1-7 demonstrate that the
   wrappers no longer carry distinct logic. Until then they are the seam that lets each
   layer evolve independently.

## Consequences

**Better:**

- `TenantID` cannot be lost by a field-copy loop; every translator goes through the
  converter, and the converter test ensures it propagates.
- A new common field is a one-line change in `pkg/o2ims/types/core.go`.
- The REST, GraphQL, and TMForum wire formats are pinned by golden files, so the inevitable
  future refactor that *does* touch them has to do so deliberately, not by accident.

**Worse:**

- Four types become five (the core + four wrappers). Reviewers learn a new convention.
- Converter calls add a very small CPU cost to list paths. Measured cost, at typical list
  sizes, is below benchmark noise; no hot-path regression observed.

**Required compatibility work:**

- None on landing. The ADR and the harness are test-only.
- Starting at step 5 (converters replace field copies), rolling deploys are safe because the
  wire formats and Redis shapes are unchanged.
- Step 7 (filter cardinality) is the only item that may require a read-path migration shim
  for already-persisted subscriptions; that PR will spell out the shim in its own ADR
  addendum.

## Regression harness

This ADR ships together with a golden-file regression harness that freezes the current
observable output of every affected type across REST, GraphQL, and TMForum encoding paths.
See `internal/o2ims/typestest/golden_test.go` and
`internal/o2ims/typestest/testdata/golden/*.json`.

**Why `internal/o2ims/typestest/` and not `pkg/o2ims/types/`?** Driving the REST encoder and
the TMForum transform requires importing `internal/adapter`, `internal/o2ims/models`,
`internal/models`, `internal/storage`, and `internal/handlers`. Those packages transitively
import things (controllers, storage backends) that would pull `pkg/o2ims/types` into an
import cycle the moment step 1 of the implementation plan lands a core type and one of those
packages imports it. Keeping the harness in `internal/o2ims/typestest/` lets it depend on
every layer without creating any back-edge into `pkg/o2ims/types`.

**Regenerating the goldens.** The harness supports `go test -update` (wired via
`flag.Bool("update", false, …)`) so that the author of a migration PR can regenerate the
fixtures after a deliberate, reviewed change. Reviewers should inspect the golden diff the
same way they inspect a schema diff: any unexpected field appearance, disappearance, or
cardinality change is a blocker.
