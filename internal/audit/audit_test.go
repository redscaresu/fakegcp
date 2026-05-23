// Package audit holds the machine-verifiable coverage audit.
//
// TestFullCoverageAudit walks fakegcp/coverage_matrix.yaml and asserts
// four invariants per entry:
//
//   (a) Integration test exists in handlers/*_test.go matching
//       integration_test_func_name.
//   (b) examples/working/<working_dir_name>/ exists OR working_exempt: true
//       with non-empty working_exempt_reason.
//   (c) examples/misconfigured/<misconfigured_dir_name>/ same contract.
//   (d) examples/updates/<updates_dir_name>/ same contract.
//
// The fakeaws sibling audit also enforces a (e) scenario-anchor
// invariant; fakegcp omits it because the GCP scenarios don't carry
// per-resource anchors in the same way.
//
// Coverage entries land per service-bundle PR. The matrix can ship
// empty (audit passes vacuously) and gains real assertions as
// services land.
package audit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// CoverageMatrixEntry mirrors the YAML schema documented in
// coverage_matrix.yaml. Field names are cloud-neutral (no aws_ prefix);
// gcp_resource_type holds the terraform-provider-google resource id
// (e.g., "google_compute_network", "google_sql_database_instance").
type CoverageMatrixEntry struct {
	Service                 string `yaml:"service"`
	GCPResourceType         string `yaml:"gcp_resource_type"`
	ScenarioResourceType    string `yaml:"scenario_resource_type,omitempty"`
	WorkingExempt           bool   `yaml:"working_exempt,omitempty"`
	WorkingExemptReason     string `yaml:"working_exempt_reason,omitempty"`
	MisconfiguredExempt     bool   `yaml:"misconfigured_exempt,omitempty"`
	MisconfiguredReason     string `yaml:"misconfigured_exempt_reason,omitempty"`
	UpdatesExempt           bool   `yaml:"updates_exempt,omitempty"`
	UpdatesReason           string `yaml:"updates_exempt_reason,omitempty"`
	WorkingDirName          string `yaml:"working_dir_name,omitempty"`
	MisconfiguredDirName    string `yaml:"misconfigured_dir_name,omitempty"`
	UpdatesDirName          string `yaml:"updates_dir_name,omitempty"`
	IntegrationTestFuncName string `yaml:"integration_test_func_name,omitempty"`
}

// CoverageMatrix is the top-level YAML shape.
type CoverageMatrix struct {
	Entries []CoverageMatrixEntry `yaml:"entries"`
}

func TestFullCoverageAudit(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("locate fakegcp repo root: %v", err)
	}
	matrix, err := loadCoverageMatrix(filepath.Join(root, "coverage_matrix.yaml"))
	if err != nil {
		t.Fatalf("load coverage_matrix.yaml: %v", err)
	}
	if len(matrix.Entries) == 0 {
		// Matrix is empty at seed time. Audit passes vacuously.
		// Service bundles add entries one by one.
		t.Logf("coverage_matrix.yaml has 0 entries; audit will run real assertions as services land")
		return
	}

	for _, entry := range matrix.Entries {
		t.Run(entry.GCPResourceType, func(t *testing.T) {
			assertHandlerTestExists(t, root, entry)
			assertExampleDirOrExempt(t, root, "working", entry.WorkingDirName, entry.GCPResourceType,
				entry.WorkingExempt, entry.WorkingExemptReason)
			assertExampleDirOrExempt(t, root, "misconfigured", entry.MisconfiguredDirName, entry.GCPResourceType,
				entry.MisconfiguredExempt, entry.MisconfiguredReason)
			assertExampleDirOrExempt(t, root, "updates", entry.UpdatesDirName, entry.GCPResourceType,
				entry.UpdatesExempt, entry.UpdatesReason)
		})
	}
}

// ----- helpers -----

func loadCoverageMatrix(path string) (*CoverageMatrix, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m CoverageMatrix
	if err := yaml.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	return &m, nil
}

// repoRoot walks up from the audit test source file's location to
// locate the fakegcp repo root (where coverage_matrix.yaml lives).
// Tests are typically run from the package directory; we walk up
// looking for the file.
func repoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 6; i++ { // bound the walk
		if _, err := os.Stat(filepath.Join(dir, "coverage_matrix.yaml")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("coverage_matrix.yaml not found")
}

func assertHandlerTestExists(t *testing.T, root string, e CoverageMatrixEntry) {
	t.Helper()
	pattern := e.IntegrationTestFuncName
	if pattern == "" {
		// Default: ^Test.*<gcp_resource_type-camel-cased>.*
		camel := camelCase(strings.TrimPrefix(e.GCPResourceType, "google_"))
		pattern = "^Test.*" + camel + ".*"
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("invalid integration_test_func_name regex %q: %v", pattern, err)
	}
	// fakegcp splits tests across multiple _test.go files in handlers/.
	matched := false
	entries, _ := os.ReadDir(filepath.Join(root, "handlers"))
	for _, ent := range entries {
		if !strings.HasSuffix(ent.Name(), "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, "handlers", ent.Name()))
		if err != nil {
			continue
		}
		if hasFuncMatching(b, re) {
			matched = true
			break
		}
	}
	if !matched {
		t.Errorf("no Test func matching %q found in handlers/*_test.go", pattern)
	}
}

var funcDeclRE = regexp.MustCompile(`(?m)^func\s+(\w+)\s*\(`)

func hasFuncMatching(body []byte, re *regexp.Regexp) bool {
	matches := funcDeclRE.FindAllSubmatch(body, -1)
	for _, m := range matches {
		if re.Match(m[1]) {
			return true
		}
	}
	return false
}

func camelCase(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}

func assertExampleDirOrExempt(t *testing.T, root, tree, dirName, resourceType string, exempt bool, reason string) {
	t.Helper()
	if exempt {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("%s tree: %s_exempt is true but reason is empty (%s)", tree, tree, resourceType)
		}
		return
	}
	if dirName == "" {
		dirName = strings.TrimPrefix(resourceType, "google_")
	}
	dir := filepath.Join(root, "examples", tree, dirName)
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("%s tree: examples/%s/%s does not exist (resource_type=%s)", tree, tree, dirName, resourceType)
	}
}
