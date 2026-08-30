// Package profile exposes Fledge-managed agent profiles and adapts their
// instructions to the native startup arguments of supported harnesses.
//
// Files:
//   - registry.go              managed profile contracts, lookup, and enumeration
//   - delivery.go              native harness instruction delivery and conflict checks
//   - fledge-orchestrator.md   embedded behavior of the managed root profile
package profile
