package handlers_test

// regression_audit_test.go — the two CI-enforced audits that keep
// the regression seed honest. Both functions live in this single file.
//
// Test names start with TestRegressionSeedAudit so a CI workflow can
// gate via `go test ./handlers/ -run "TestRegressionSeedAudit"`.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/redscaresu/fakegcp/handlers"
)

// TestRegressionSeedAuditManifestMatchesHandlers walks the manifest
// (handlers/regression_manifest.go::LandedServices) and the
// handlers/ directory; asserts:
//
//	(a) every id in LandedServices is satisfied by ≥1 handlers/<id>*.go
//	    file (so "compute" is satisfied by handlers/compute.go OR any
//	    aliased file like handlers/network.go that maps to it); and
//	(b) every service prefix in handlers/ has a manifest entry — the
//	    audit groups files by their before-first-`_`-or-`.go` prefix
//	    (modulo the servicePrefixAliases map) and asserts every
//	    prefix appears in LandedServices.
//
// Files that are not service handlers (handlers.go itself, admin.go,
// regression_manifest.go, *_test.go) are excluded from the prefix
// check via the knownNonServiceFiles list.
func TestRegressionSeedAuditManifestMatchesHandlers(t *testing.T) {
	dir := handlersDir(t)

	// (a) Every LandedServices id has ≥1 satisfying file.
	for _, id := range handlers.LandedServices {
		if !matchHandlerFile(t, dir, id) {
			t.Errorf("LandedServices entry %q has no matching handlers/%s*.go file (manifest sorted: %v)", id, id, debugSortedServices())
		}
	}

	// (b) Every service prefix in handlers/ is in LandedServices.
	prefixes := serviceFilePrefixes(t, dir)
	for prefix := range prefixes {
		if !sliceContains(handlers.LandedServices, prefix) {
			t.Errorf("handlers/ has files with prefix %q but LandedServices is missing it; either add to manifest, rename the file, or add an entry to servicePrefixAliases", prefix)
		}
	}
}

// TestRegressionSeedAuditNoVacuousPasses parses every test file in
// handlers/ via go/ast and asserts no test function body contains
// both a requireHandlerImplemented(...) call AND an assert./require.
// or t.Error/t.Fatal/t.Fail call. That combination is a vacuous-pass
// smell — the manifest-gated skip should fire BEFORE the test reaches
// its assertions, so the two should never coexist in the same func
// body. Bare t.Skip / t.Skipf / t.SkipNow outside the helper is
// likewise the exact vacuous-pass pattern the audit must catch.
//
// When a service flips to landed in LandedServices,
// requireHandlerImplemented stops skipping and the assertions run;
// this audit catches the case where someone forgot to remove the
// requireHandlerImplemented call after lighting up the assertions.
func TestRegressionSeedAuditNoVacuousPasses(t *testing.T) {
	dir := handlersDir(t)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read handlers/: %v", err)
	}
	fset := token.NewFileSet()
	for _, ent := range entries {
		if !strings.HasSuffix(ent.Name(), "_test.go") {
			continue
		}
		// Skip the audit file itself.
		if ent.Name() == "regression_audit_test.go" {
			continue
		}
		path := filepath.Join(dir, ent.Name())
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				return true
			}
			// Only audit functions that look like Test* funcs.
			if fn.Name == nil || !strings.HasPrefix(fn.Name.Name, "Test") {
				return true
			}
			hasRequire := false
			hasAssert := false
			hasBareSkip := false
			ast.Inspect(fn.Body, func(inner ast.Node) bool {
				call, ok := inner.(*ast.CallExpr)
				if !ok {
					return true
				}
				if name, ok := callIdentName(call); ok {
					if name == "requireHandlerImplemented" {
						hasRequire = true
					}
				}
				if pkg, sel, ok := callSelectorName(call); ok {
					if pkg == "assert" || pkg == "require" {
						hasAssert = true
					}
					// Real assertions on this suite go through plain
					// testing.T methods (t.Error, t.Errorf, t.Fatal,
					// t.Fatalf, t.Fail, t.FailNow) in addition to
					// testify. Counting only assert./require. would
					// leave landed tests keeping
					// requireHandlerImplemented next to a plain
					// t.Errorf silently green.
					if pkg == "t" && (sel == "Error" || sel == "Errorf" ||
						sel == "Fatal" || sel == "Fatalf" ||
						sel == "Fail" || sel == "FailNow") {
						hasAssert = true
					}
					// Bare t.Skip / t.Skipf outside
					// requireHandlerImplemented is the exact vacuous-
					// pass pattern the audit was supposed to catch.
					// Flag any direct skip on the testing-T receiver.
					if pkg == "t" && (sel == "Skip" || sel == "Skipf" || sel == "SkipNow") {
						hasBareSkip = true
					}
				}
				return true
			})
			if hasRequire && hasAssert {
				t.Errorf("vacuous-pass risk in %s::%s — requireHandlerImplemented coexists with assert./require. (or t.Error*/t.Fatal*/t.Fail*) in the same func body. Either remove the requireHandlerImplemented call (service has landed) or remove the assertion (this is a stub).",
					ent.Name(), fn.Name.Name)
			}
			if hasBareSkip {
				t.Errorf("vacuous-pass risk in %s::%s — direct t.Skip/Skipf/SkipNow call. Use requireHandlerImplemented(t, service, slice, pattern) so the audit can track it; bare skips violate the manifest-gated contract.",
					ent.Name(), fn.Name.Name)
			}
			return true
		})
	}
}

// TestRegressionSeedAuditHasPatterns is the M85 vacuous-audit guard.
// The two audits above are necessary but not sufficient: they enforce
// shape constraints on patterns that EXIST. They do not enforce that
// patterns exist at all. fakegcp shipped this audit scaffolding with
// ZERO TestRegression* functions for ~12 months — M79 closed the gap
// by porting fakeaws's catalogue (13 patterns), but the audit passed
// CI the whole time prior because there was nothing to validate.
// This guard asserts a minimum pattern count proportional to the
// LandedServices manifest so the gap cannot reopen.
//
// Threshold: min(len(LandedServices), 8). A floor of 8 keeps small
// repos from being forced into thin patterns; the per-service cap
// makes the guard tighten as the surface grows.
//
// Counting rule: functions named ^TestRegression but NOT
// ^TestRegressionSeedAudit (the audit tests themselves don't count).
func TestRegressionSeedAuditHasPatterns(t *testing.T) {
	dir := handlersDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read handlers/: %v", err)
	}
	fset := token.NewFileSet()
	count := 0
	for _, ent := range entries {
		if !strings.HasSuffix(ent.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, ent.Name())
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Name == nil {
				return true
			}
			name := fn.Name.Name
			if strings.HasPrefix(name, "TestRegression") && !strings.HasPrefix(name, "TestRegressionSeedAudit") {
				count++
			}
			return true
		})
	}
	threshold := len(handlers.LandedServices)
	if threshold > 8 {
		threshold = 8
	}
	if count < threshold {
		t.Fatalf("vacuous-audit guard: found only %d TestRegression* patterns, need ≥%d (LandedServices has %d entries). Add patterns to handlers/regression_test.go that exercise FK rejection, cascade ordering, wire-shape contracts, etc. See M75/M79 for the pattern catalogue.", count, threshold, len(handlers.LandedServices))
	}
}

// ----- helpers (local to the audit suite) -----

func handlersDir(t *testing.T) string {
	t.Helper()
	// Tests run from the package directory; handlers/ is the package itself.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return wd
}

func matchHandlerFile(t *testing.T, dir, id string) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read handlers/: %v", err)
	}
	for _, ent := range entries {
		name := ent.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		base := strings.TrimSuffix(name, ".go")
		// An aliased file (e.g., network.go → compute) also satisfies
		// the alias target.
		if alias, ok := servicePrefixAliases[base]; ok && alias == id {
			return true
		}
		// Exact match (e.g., "iam.go" → id "iam") or prefix-with-_
		// (e.g., "iam_bindings.go" → id "iam").
		if base == id || strings.HasPrefix(base, id+"_") {
			return true
		}
	}
	return false
}

// knownNonServiceFiles lists handler-package files that aren't
// service handlers — they're shared infrastructure (router wiring,
// admin endpoints, audit tests, etc.) and shouldn't appear in
// LandedServices.
var knownNonServiceFiles = map[string]bool{
	"handlers.go":            true,
	"admin.go":               true,
	"regression_manifest.go": true,
}

// servicePrefixAliases maps non-conformant filename prefixes to their
// logical service id. handlers/network.go is really compute_network
// (CreateNetwork / CreateSubnetwork are GCP Compute API operations);
// handlers/router.go is really compute_router (CreateRouter etc).
// Without the alias map the audit's reverse-check would flag
// "network" and "router" as services missing from LandedServices.
// Renaming the files would lose git blame; aliasing is one
// declarative line per indirection.
var servicePrefixAliases = map[string]string{
	"network": "compute",
	"router":  "compute",
}

func serviceFilePrefixes(t *testing.T, dir string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read handlers/: %v", err)
	}
	out := map[string]bool{}
	for _, ent := range entries {
		name := ent.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if knownNonServiceFiles[name] {
			continue
		}
		base := strings.TrimSuffix(name, ".go")
		// Service prefix: everything before the first '_' (so
		// iam_bindings.go → "iam"; cloudrun.go → "cloudrun").
		prefix := base
		if i := strings.Index(base, "_"); i > 0 {
			prefix = base[:i]
		}
		// Apply the alias map BEFORE inserting into out, so the
		// reverse-check measures logical services, not filename
		// prefixes.
		if aliased, ok := servicePrefixAliases[prefix]; ok {
			prefix = aliased
		}
		out[prefix] = true
	}
	return out
}

func sliceContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func callIdentName(call *ast.CallExpr) (string, bool) {
	if id, ok := call.Fun.(*ast.Ident); ok {
		return id.Name, true
	}
	return "", false
}

func callSelectorName(call *ast.CallExpr) (string, string, bool) {
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if pkg, ok := sel.X.(*ast.Ident); ok {
			return pkg.Name, sel.Sel.Name, true
		}
	}
	return "", "", false
}

// debugSortedServices returns the manifest sorted alphabetically; used
// only by failure messages.
func debugSortedServices() []string {
	out := append([]string(nil), handlers.LandedServices...)
	sort.Strings(out)
	return out
}

var _ = debugSortedServices // referenced to silence "unused" if no failure
