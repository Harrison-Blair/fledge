// Package profile exposes Fledge-managed agent profiles and adapts their
// instructions to the native startup arguments of supported harnesses.
//
// Files:
//   - registry.go              managed profile contracts, lookup, and enumeration
//   - delivery.go              native harness instruction delivery and conflict checks
//   - composition.go           deterministic assembly of managed-profile instructions
//   - fledge-orchestrator.md   embedded behavior of the managed root profile
//   - fledge-core.md           role-neutral session identity and trust fragment
//   - fledge-general.md        role-neutral managed-worker behavior fragment
//   - fledge-worker-report.md  managed-worker callback and report protocol fragment
package profile
