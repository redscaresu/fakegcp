// Package handlers — regression-seed manifest.
//
// LandedServices is the tracked list of service-level ids that have a
// fully-implemented handler set. requireHandlerImplemented(t, id)
// checks against this list to decide whether a regression test
// proceeds with real assertions or skips with a structured TODO
// message until the corresponding handler lands.
//
// Service-level ids only (no subservice slashes). Adding a service to
// the manifest is the last step of the per-bundle PR (handler + tests
// + examples + coverage_matrix entry + manifest flip, all together).
package handlers

import (
	"slices"
	"testing"
)

// LandedServices lists the service-level ids whose handlers are fully
// implemented in fakegcp today. Each id is a single lowercase token
// (no hyphens, no subservice slashes).
//
// Note: `network` and `router` are NOT in this list — they're file-
// name aliases for the `compute` service (CreateNetwork /
// CreateSubnetwork / CreateRouter are GCP Compute API operations).
// The audit's serviceFilePrefixes() helper maps those filename
// prefixes to `compute` via servicePrefixAliases so the reverse-check
// (every file prefix has a LandedServices entry) doesn't false-fail.
//
// Service tickets append to this list in their landing PR. The audit
// (TestRegressionSeedAuditManifestMatchesHandlers) asserts every entry
// here corresponds to ≥1 handlers/<id>*.go file AND every service
// prefix in handlers/ has a manifest entry (after aliasing).
var LandedServices = []string{
	"cloudrun",
	"compute",
	"container",
	"dns",
	"iam",
	"loadbalancer",
	"memorystore",
	"pubsub",
	"secretmanager",
	"serviceusage",
	"sql",
	"storage",
}

// requireHandlerImplemented is the manifest-gated skip helper. Tests
// for not-yet-landed services call this — the test calls t.Skipf with
// a structured TODO marker so CI can grep+count outstanding work
// without silent green-lights.
//
// Bare t.Skip() is forbidden; this helper is the *only* sanctioned
// skip path. The audit (TestRegressionSeedAuditNoVacuousPasses) parses
// test bodies via go/ast and fails CI if any test func contains both
// requireHandlerImplemented(...) AND a passing assert./require. call
// — that combination is the vacuous-pass smell.
//
// id is one of the LandedServices values; pattern is a short
// human-readable name for the standing pattern this test pins.
func requireHandlerImplemented(t *testing.T, id, slice, pattern string) {
	t.Helper()
	if slices.Contains(LandedServices, id) {
		return
	}
	t.Skipf("TODO(slice=%s,service=%s,pattern=%s) regression awaits handler — flip handlers/regression_manifest.go::LandedServices when the service lands",
		slice, id, pattern)
}

// RequireHandlerImplementedForTest is the public re-export for the
// handlers_test package's regression suite. Internal handler tests
// can use the package-private requireHandlerImplemented directly.
func RequireHandlerImplementedForTest(t *testing.T, id, slice, pattern string) {
	t.Helper()
	requireHandlerImplemented(t, id, slice, pattern)
}
