// Package typestest contains a golden-file regression harness that pins the
// current observable JSON output of the four parallel domain-type shapes used
// by netweave: the adapter contract, the O2-IMS REST wire format, the
// Redis-persisted storage envelope, and the gqlgen-bound GraphQL object graph.
//
// The harness also pins the TMForum TMF639 transform output produced by
// internal/handlers/tmforum_transform.go.
//
// The purpose of this package is to support the type-unification work tracked
// by issue #484 (H11) and documented in docs/adr/0001-o2ims-domain-type-unification.md.
// Any migration that intentionally changes observable output must regenerate
// the golden files with:
//
//	go test ./internal/o2ims/typestest -update
//
// and the resulting diff must be reviewed as carefully as a schema change.
//
// This package lives under internal/ (not pkg/) so that it can import every
// layer — adapter, storage, models, handlers — without creating an import
// cycle with the forthcoming pkg/o2ims/types package. See the ADR for the
// full rationale.
package typestest
