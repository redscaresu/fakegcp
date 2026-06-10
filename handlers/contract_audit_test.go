package handlers_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestAllContractsHaveTests is the durable, CI-enforced enforcement of
// the CRITICAL[<id>]: / TestContract_<id> wire-shape convention shared
// across the fakegenesys / mockway / fakegcp / fakeaws sibling-fake
// family.
//
// Goal: a wire-shape invariant the consuming Terraform provider depends
// on must NOT live as a comment alone. Every CRITICAL[<id>]: or
// MUST[<id>]: docstring in handlers/*.go MUST have a paired
// TestContract_<id> in the same package (kebab-case → snake_case), and
// every TestContract_<id> MUST have at least one source [<id>] tag.
// Drift becomes a failed `go test`, not a missed code review.
//
// Adding a contract:
//
//  1. Add `// CRITICAL[<kebab-case-id>]: <invariant + why it matters>`
//     above the handler (or `MUST[<id>]:` if the constraint is in a
//     specific code path rather than the function preamble).
//  2. Add `func TestContract_<id_with_underscores>(t *testing.T)` to a
//     test file in this package. The test must assert the invariant
//     (revert the fix → test fails).
//
// Convention origin: fakegenesys S123 introduced the convention; S127
// rolled it out across all 4 siblings. See item 14 in
// feedback_oss_mature_day_one.md.
//
// The audit is empty-contracts-safe: a package with zero CRITICAL[id]
// docstrings AND zero TestContract_ tests passes trivially. This lets
// new sibling fakes adopt the file before they've fully swept their
// existing CRITICAL: notes — permission-to-use without imposing
// immediate inventory.

const (
	handlersGlob = "*.go"      // non-test source
	testFileGlob = "*_test.go" // test files
	contractIDRe = `(?:CRITICAL|MUST)\[([a-z0-9][a-z0-9-]*)\]`
	testFuncRe   = `func\s+(TestContract_[A-Za-z0-9_]+)\s*\(`
)

var (
	contractRe = regexp.MustCompile(contractIDRe)
	testRe     = regexp.MustCompile(testFuncRe)
)

func TestAllContractsHaveTests(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	sourceIDs, sourceLocs := scanContractIDs(t, dir)
	testIDs, testLocs := scanTestContractFuncs(t, dir)

	if len(sourceIDs) == 0 && len(testIDs) == 0 {
		t.Log("contract audit: zero contracts in this package — pass (empty-state is the bootstrap case)")
		return
	}

	missingTests := setDiff(sourceIDs, testIDs)
	orphanTests := setDiff(testIDs, sourceIDs)

	if len(missingTests) == 0 && len(orphanTests) == 0 {
		t.Logf("contract audit: %d contracts, all paired with tests", len(sourceIDs))
		return
	}

	if len(missingTests) > 0 {
		t.Errorf("contract audit: %d contract docstring(s) lack a paired TestContract_<id>:", len(missingTests))
		for _, id := range sortedKeys(missingTests) {
			t.Errorf("  - %s\n    declared at: %s\n    expected test: func TestContract_%s(t *testing.T)",
				fmt.Sprintf("CRITICAL[%s] / MUST[%s]", id, id),
				strings.Join(sourceLocs[id], ", "),
				kebabToSnake(id))
		}
	}
	if len(orphanTests) > 0 {
		t.Errorf("contract audit: %d test(s) lack a paired CRITICAL[<id>]: or MUST[<id>]: docstring:", len(orphanTests))
		for _, id := range sortedKeys(orphanTests) {
			t.Errorf("  - test: TestContract_%s\n    declared at: %s\n    expected docstring: CRITICAL[%s]: or MUST[%s]: in a handler",
				kebabToSnake(id),
				strings.Join(testLocs[id], ", "),
				id, id)
		}
	}

	t.Log("To fix: add the missing test(s) AND/OR docstring(s), OR demote the orphan to a plain comment.")
}

func scanContractIDs(t *testing.T, dir string) (map[string]struct{}, map[string][]string) {
	t.Helper()
	files := globGo(t, dir, handlersGlob, true)
	ids := map[string]struct{}{}
	locs := map[string][]string{}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		base := filepath.Base(path)
		for line, text := range strings.Split(string(raw), "\n") {
			for _, m := range contractRe.FindAllStringSubmatch(text, -1) {
				id := m[1]
				ids[id] = struct{}{}
				locs[id] = append(locs[id], fmt.Sprintf("%s:%d", base, line+1))
			}
		}
	}
	return ids, locs
}

func scanTestContractFuncs(t *testing.T, dir string) (map[string]struct{}, map[string][]string) {
	t.Helper()
	files := globGo(t, dir, testFileGlob, false)
	ids := map[string]struct{}{}
	locs := map[string][]string{}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		base := filepath.Base(path)
		for line, text := range strings.Split(string(raw), "\n") {
			for _, m := range testRe.FindAllStringSubmatch(text, -1) {
				funcName := m[1]
				kebab := snakeToKebab(strings.TrimPrefix(funcName, "TestContract_"))
				ids[kebab] = struct{}{}
				locs[kebab] = append(locs[kebab], fmt.Sprintf("%s:%d", base, line+1))
			}
		}
	}
	return ids, locs
}

func globGo(t *testing.T, dir, pattern string, excludeTest bool) []string {
	t.Helper()
	all, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		t.Fatalf("glob %q: %v", pattern, err)
	}
	out := make([]string, 0, len(all))
	for _, p := range all {
		base := filepath.Base(p)
		if isAuditMachineryFile(base) {
			continue
		}
		isTest := strings.HasSuffix(base, "_test.go")
		if excludeTest && isTest {
			continue
		}
		if !excludeTest && !isTest {
			continue
		}
		out = append(out, p)
	}
	return out
}

func isAuditMachineryFile(base string) bool {
	return base == "contract_audit_test.go"
}

func setDiff(a, b map[string]struct{}) map[string]struct{} {
	out := map[string]struct{}{}
	for k := range a {
		if _, ok := b[k]; !ok {
			out[k] = struct{}{}
		}
	}
	return out
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func kebabToSnake(s string) string { return strings.ReplaceAll(s, "-", "_") }
func snakeToKebab(s string) string { return strings.ReplaceAll(s, "_", "-") }

// TestContractAuditTest_Self validates the audit's own logic with
// known-good and known-bad fixtures. Without this, a bug in the audit
// could silently let real contracts drift.
func TestContractAuditTest_Self(t *testing.T) {
	tmp := t.TempDir()
	mustWrite(t, filepath.Join(tmp, "src.go"),
		"package x\n// CRITICAL[foo-bar]: invariant\nfunc handle() {}\n")
	mustWrite(t, filepath.Join(tmp, "src_test.go"),
		"package x\nimport \"testing\"\nfunc TestContract_foo_bar(t *testing.T) {}\n")

	src, _ := scanContractIDs(t, tmp)
	tst, _ := scanTestContractFuncs(t, tmp)
	if got := setDiff(src, tst); len(got) != 0 {
		t.Errorf("known-good: setDiff(src, tst) = %v, want empty", got)
	}
	if got := setDiff(tst, src); len(got) != 0 {
		t.Errorf("known-good: setDiff(tst, src) = %v, want empty", got)
	}

	tmp2 := t.TempDir()
	mustWrite(t, filepath.Join(tmp2, "src.go"),
		"package x\n// CRITICAL[orphan-doc]: invariant\nfunc handle() {}\n")
	src2, _ := scanContractIDs(t, tmp2)
	tst2, _ := scanTestContractFuncs(t, tmp2)
	if diff := setDiff(src2, tst2); len(diff) != 1 {
		t.Errorf("known-bad-missing-test: setDiff = %v, want exactly 1 missing", diff)
	}

	tmp3 := t.TempDir()
	mustWrite(t, filepath.Join(tmp3, "src_test.go"),
		"package x\nimport \"testing\"\nfunc TestContract_orphan_test(t *testing.T) {}\n")
	src3, _ := scanContractIDs(t, tmp3)
	tst3, _ := scanTestContractFuncs(t, tmp3)
	if diff := setDiff(tst3, src3); len(diff) != 1 {
		t.Errorf("known-bad-orphan-test: setDiff(tst, src) = %v, want exactly 1 orphan", diff)
	}

	if got := snakeToKebab(kebabToSnake("a-b-c-d")); got != "a-b-c-d" {
		t.Errorf("kebab/snake round-trip: %q", got)
	}
}

func mustWrite(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
