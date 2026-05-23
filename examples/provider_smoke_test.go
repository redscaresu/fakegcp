// Package examples — auto-discovered provider smoke harness for fakegcp.
//
// For every example dir under examples/{working,misconfigured,updates}/
// this test starts a fresh `fakegcp` binary on a free port (no shared
// mock state across dirs), copies the example into a temp dir, rewrites
// `localhost:8080` in `providers.tf` to point at the per-test port, and
// runs the per-tree contract:
//
//   working/      apply → plan -detailed-exitcode (no diff) → destroy
//   misconfigured/ apply MUST fail (and if expected.txt is present, the
//                  output MUST contain that error fragment)
//   updates/      apply -var-file=v1.tfvars → plan no-op
//                 → apply -var-file=v2.tfvars → plan no-op → destroy
//
// Adding a directory to ANY of the three trees auto-registers — no
// per-example test wiring. Each subdir is its own t.Run sub-test.
//
// Gating:
//   - Gated by FAKEGCP_ENABLE_E2E=1 because it shells out to `tofu` and
//     builds + spawns the fakegcp binary. Without the env var, the
//     test t.Skip's with a clear message.
//
// known_broken.yaml allowlist:
//   - examples/known_broken.yaml lists dirs whose idempotency gate is
//     currently expected to fail (each entry references a tracking
//     ticket in infrafactory/BACKLOG.md). Entries skip the drift
//     assertion but still run apply + destroy. If a known-broken dir
//     starts passing idempotency, the test FAILS with
//     "congratulations, remove this entry" — ratchet-only-tighten.
//
// This package is `examples_test` so the auto-discovery walks the repo
// via runtime.Caller (mirror of fakeaws/examples/provider_smoke_test.go
// and mockway/e2e/provider_smoke_test.go).
package examples_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	gateEnvVar          = "FAKEGCP_ENABLE_E2E"
	defaultFakegcpHost  = "127.0.0.1"
	bootBudget          = 5 * time.Second
	commandBudget       = 8 * time.Minute
	knownBrokenFileName = "known_broken.yaml"
)

// brokenEntry mirrors examples/known_broken.yaml schema.
type brokenEntry struct {
	Dir     string `yaml:"dir"`
	Symptom string `yaml:"symptom"`
	Ticket  string `yaml:"ticket"`
}

type brokenList struct {
	Entries []brokenEntry `yaml:"entries"`
}

// brokenIndex maps "<tree>/<name>" → entry for fast lookup.
type brokenIndex map[string]brokenEntry

// fakegcpBinaryOnce builds the fakegcp binary one time per test run.
var (
	fakegcpBinaryOnce sync.Once
	fakegcpBinaryPath string
	fakegcpBinaryErr  error
)

// TestKnownBrokenAllowlistSummary prints a CI-greppable summary of every
// entry in examples/known_broken.yaml. Always runs (no gate, no skip)
// so the ratchet's current state is visible on every PR run, not just
// when -v is set and a broken dir fires. Closes S53-T3.
//
// Output shape:
//   known_broken summary: 2 entries
//   known_broken: working/iam — ... (ticket M46)
//   known_broken: working/basic_instance — ... (ticket M48)
//
// Empty allowlist prints "known_broken summary: 0 entries" so a clean
// state is unambiguous.
func TestKnownBrokenAllowlistSummary(t *testing.T) {
	root := repoRoot(t)
	broken := loadKnownBroken(t, root)
	t.Logf("known_broken summary: %d entries", len(broken))
	keys := make([]string, 0, len(broken))
	for k := range broken {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		e := broken[k]
		t.Logf("known_broken: %s — %s (ticket %s)", k, e.Symptom, e.Ticket)
	}
}

// TestProviderSmokeWorking walks examples/working/<svc>/ and asserts
// idempotency: apply → plan -detailed-exitcode (must be no-diff) → destroy.
// Known-broken entries skip the drift assertion but still run apply + destroy.
func TestProviderSmokeWorking(t *testing.T) {
	requireE2EGate(t)
	requireTofu(t)
	root := repoRoot(t)
	bin := buildFakegcpOnce(t, root)
	broken := loadKnownBroken(t, root)
	dir := filepath.Join(root, "examples", "working")
	walkExamplesAndRun(t, dir, "working", broken, bin, runWorkingExample)
}

// TestProviderSmokeMisconfigured walks examples/misconfigured/<svc>/.
// `tofu apply` MUST fail. If expected.txt is present, the failure output
// MUST contain that string fragment.
func TestProviderSmokeMisconfigured(t *testing.T) {
	requireE2EGate(t)
	requireTofu(t)
	root := repoRoot(t)
	bin := buildFakegcpOnce(t, root)
	broken := loadKnownBroken(t, root)
	dir := filepath.Join(root, "examples", "misconfigured")
	walkExamplesAndRun(t, dir, "misconfigured", broken, bin, runMisconfiguredExample)
}

// TestProviderSmokeUpdates walks examples/updates/<svc>/, applies v1, asserts
// plan is clean, applies v2, asserts plan is clean, destroys. Each updates/
// directory MUST contain v1.tfvars + v2.tfvars.
func TestProviderSmokeUpdates(t *testing.T) {
	requireE2EGate(t)
	requireTofu(t)
	root := repoRoot(t)
	bin := buildFakegcpOnce(t, root)
	broken := loadKnownBroken(t, root)
	dir := filepath.Join(root, "examples", "updates")
	walkExamplesAndRun(t, dir, "updates", broken, bin, runUpdatesExample)
}

// ----- discovery -----

type exampleCtx struct {
	srcDir  string // original example dir (read-only)
	workDir string // per-test temp copy with rewritten providers.tf
	port    int    // port for this run's fakegcp instance
	broken  *brokenEntry
}

type exampleRunner func(t *testing.T, ec exampleCtx)

func walkExamplesAndRun(
	t *testing.T,
	parent string,
	tree string,
	broken brokenIndex,
	bin string,
	run exampleRunner,
) {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.Logf("skipping %s — directory does not exist", parent)
			return
		}
		t.Fatalf("read %s: %v", parent, err)
	}
	any := false
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		any = true
		name := ent.Name()
		srcDir := filepath.Join(parent, name)
		key := tree + "/" + name
		brokenEntry, isBroken := broken[key]
		t.Run(name, func(t *testing.T) {
			ec := setupExample(t, srcDir, bin)
			if isBroken {
				e := brokenEntry
				ec.broken = &e
				t.Logf("known_broken: %s — %s (ticket %s)", key, e.Symptom, e.Ticket)
			}
			run(t, ec)
		})
	}
	if !any {
		t.Logf("no example subdirectories under %s", parent)
	}
}

// setupExample picks a free port, copies srcDir → temp, rewrites
// `localhost:8080` in *.tf to `localhost:<port>`, and starts a fresh
// fakegcp instance bound to that port. Cleanup tears down the server.
func setupExample(t *testing.T, srcDir, bin string) exampleCtx {
	t.Helper()
	port := pickFreePort(t)
	work := t.TempDir()
	if err := copyDir(srcDir, work); err != nil {
		t.Fatalf("copy %s → %s: %v", srcDir, work, err)
	}
	if err := rewriteEndpointPort(work, port); err != nil {
		t.Fatalf("rewrite providers.tf in %s: %v", work, err)
	}
	startFakegcp(t, bin, port)
	return exampleCtx{srcDir: srcDir, workDir: work, port: port}
}

// ----- per-tree contracts -----

func runWorkingExample(t *testing.T, ec exampleCtx) {
	t.Helper()
	env := tofuEnv()
	tofuInit(t, ec.workDir, env)

	// For known_broken dirs we tolerate failure at any of the apply / plan
	// / destroy steps — the symptom text in known_broken.yaml documents
	// which step is currently failing. If ALL steps pass cleanly, the
	// entry should be removed (ratchet-only-tighten).
	if ec.broken != nil {
		applyOK := tofuApplyTolerant(t, ec.workDir, env, nil)
		if !applyOK {
			t.Logf("known_broken: apply failed as expected (ticket %s)", ec.broken.Ticket)
			return
		}
		planExit := tofuPlanDetailed(t, ec.workDir, env, nil)
		if planExit == 0 {
			t.Fatalf("known_broken dir %q PASSED idempotency — congratulations, remove this entry from %s (ticket %s)",
				ec.broken.Dir, knownBrokenFileName, ec.broken.Ticket)
		}
		t.Logf("known_broken: drift detected as expected (exit %d, ticket %s)", planExit, ec.broken.Ticket)
		// Best-effort cleanup. Destroy may also fail for known-broken
		// dirs; that's fine — t.TempDir cleans up the workdir and the
		// per-test fakegcp instance is killed by t.Cleanup.
		_ = tofuDestroyTolerant(t, ec.workDir, env, nil)
		return
	}

	tofuApply(t, ec.workDir, env, nil)
	if planExit := tofuPlanDetailed(t, ec.workDir, env, nil); planExit != 0 {
		t.Fatalf("second apply is not idempotent: plan -detailed-exitcode exit=%d (drift detected)", planExit)
	}
	tofuDestroy(t, ec.workDir, env, nil)
}

func runMisconfiguredExample(t *testing.T, ec exampleCtx) {
	t.Helper()
	env := tofuEnv()
	tofuInit(t, ec.workDir, env)
	out, err := tofuApplyExpectingFailure(t, ec.workDir, env)
	if err == nil {
		t.Fatalf("misconfigured example: tofu apply UNEXPECTEDLY succeeded\noutput:\n%s", out)
	}
	// expected.txt is optional in fakegcp (most misconfigured dirs document
	// the expected error in README.md). If present, enforce the fragment.
	expectedPath := filepath.Join(ec.srcDir, "expected.txt")
	if data, readErr := os.ReadFile(expectedPath); readErr == nil {
		expected := strings.TrimSpace(string(data))
		if expected != "" && !strings.Contains(out, expected) {
			t.Fatalf("misconfigured example: apply failed but output does not contain expected error %q\noutput:\n%s",
				expected, out)
		}
	}
}

func runUpdatesExample(t *testing.T, ec exampleCtx) {
	t.Helper()
	v1 := filepath.Join(ec.workDir, "v1.tfvars")
	v2 := filepath.Join(ec.workDir, "v2.tfvars")
	for _, p := range []string{v1, v2} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("updates example missing %s: %v", p, err)
		}
	}

	env := tofuEnv()
	tofuInit(t, ec.workDir, env)

	// Known_broken entries tolerate failure at any step. We track whether
	// the run reached the end clean so that, if it did, the test fails
	// with "congratulations, remove this entry" — ratchet-only-tighten.
	if ec.broken != nil {
		if !tofuApplyTolerant(t, ec.workDir, env, []string{"-var-file=" + v1}) {
			t.Logf("known_broken: apply v1 failed as expected (ticket %s)", ec.broken.Ticket)
			return
		}
		v1Exit := tofuPlanDetailed(t, ec.workDir, env, []string{"-var-file=" + v1})
		if v1Exit != 0 {
			t.Logf("known_broken: drift at v1 detected as expected (exit %d, ticket %s)", v1Exit, ec.broken.Ticket)
			_ = tofuDestroyTolerant(t, ec.workDir, env, []string{"-var-file=" + v1})
			return
		}
		if !tofuApplyTolerant(t, ec.workDir, env, []string{"-var-file=" + v2}) {
			t.Logf("known_broken: apply v2 failed as expected (ticket %s)", ec.broken.Ticket)
			_ = tofuDestroyTolerant(t, ec.workDir, env, []string{"-var-file=" + v1})
			return
		}
		v2Exit := tofuPlanDetailed(t, ec.workDir, env, []string{"-var-file=" + v2})
		if v2Exit != 0 {
			t.Logf("known_broken: drift at v2 detected as expected (exit %d, ticket %s)", v2Exit, ec.broken.Ticket)
			_ = tofuDestroyTolerant(t, ec.workDir, env, []string{"-var-file=" + v2})
			return
		}
		t.Fatalf("known_broken updates dir %q PASSED full v1+v2 idempotency — congratulations, remove this entry from %s (ticket %s)",
			ec.broken.Dir, knownBrokenFileName, ec.broken.Ticket)
		return
	}

	tofuApply(t, ec.workDir, env, []string{"-var-file=" + v1})
	if exit := tofuPlanDetailed(t, ec.workDir, env, []string{"-var-file=" + v1}); exit != 0 {
		t.Fatalf("apply at v1 is not idempotent: plan -detailed-exitcode exit=%d", exit)
	}

	tofuApply(t, ec.workDir, env, []string{"-var-file=" + v2})
	if exit := tofuPlanDetailed(t, ec.workDir, env, []string{"-var-file=" + v2}); exit != 0 {
		t.Fatalf("apply at v2 is not idempotent: plan -detailed-exitcode exit=%d", exit)
	}

	tofuDestroy(t, ec.workDir, env, []string{"-var-file=" + v2})
}

// ----- tofu wrappers -----

func tofuEnv() []string {
	return append(os.Environ(),
		"GOOGLE_PROJECT=fake-project",
		"GOOGLE_REGION=us-central1",
		"GOOGLE_ZONE=us-central1-a",
		"GOOGLE_OAUTH_ACCESS_TOKEN=fake-token",
		"GOOGLE_CREDENTIALS=",
		"TF_IN_AUTOMATION=1",
	)
}

func tofuInit(t *testing.T, dir string, env []string) {
	t.Helper()
	runTofu(t, dir, env, "init", "-input=false", "-no-color", "-reconfigure")
}

func tofuApply(t *testing.T, dir string, env, extraArgs []string) {
	t.Helper()
	args := append([]string{"apply", "-auto-approve", "-input=false", "-no-color"}, extraArgs...)
	runTofu(t, dir, env, args...)
}

// tofuApplyTolerant runs apply but returns false instead of failing the
// test when apply errors. Used for known_broken entries where the
// failure mode is documented + tracked.
func tofuApplyTolerant(t *testing.T, dir string, env, extraArgs []string) bool {
	t.Helper()
	args := append([]string{"apply", "-auto-approve", "-input=false", "-no-color"}, extraArgs...)
	ctx, cancel := context.WithTimeout(context.Background(), commandBudget)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tofu", args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("tofu %v timed out\n%s", args, out)
	}
	if err != nil {
		t.Logf("tofu %v failed (tolerated): %v\n%s", args, err, out)
		return false
	}
	return true
}

// tofuDestroyTolerant is the best-effort variant of tofuDestroy used as
// cleanup for known_broken entries (which may have partial state).
func tofuDestroyTolerant(t *testing.T, dir string, env, extraArgs []string) bool {
	t.Helper()
	args := append([]string{"destroy", "-auto-approve", "-input=false", "-no-color"}, extraArgs...)
	ctx, cancel := context.WithTimeout(context.Background(), commandBudget)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tofu", args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("tofu %v failed (tolerated): %v\n%s", args, err, out)
		return false
	}
	return true
}

func tofuApplyExpectingFailure(t *testing.T, dir string, env []string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandBudget)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tofu", "apply", "-auto-approve", "-input=false", "-no-color")
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// tofuPlanDetailed runs `tofu plan -detailed-exitcode` and returns:
//   0 = no changes (idempotent)
//   2 = drift detected (NOT idempotent)
//   any other → t.Fatalf (plan errored)
func tofuPlanDetailed(t *testing.T, dir string, env, extraArgs []string) int {
	t.Helper()
	args := append([]string{"plan", "-detailed-exitcode", "-input=false", "-no-color"}, extraArgs...)
	ctx, cancel := context.WithTimeout(context.Background(), commandBudget)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tofu", args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("tofu plan timed out\n%s", out)
	}
	if err == nil {
		return 0
	}
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("tofu plan: %v\n%s", err, out)
	}
	code := ee.ExitCode()
	if code == 2 {
		// caller (runWorkingExample/runUpdatesExample) decides whether
		// drift is a failure based on known_broken status.
		return 2
	}
	t.Fatalf("tofu plan -detailed-exitcode unexpected exit=%d: %v\n%s", code, err, out)
	return code
}

func tofuDestroy(t *testing.T, dir string, env, extraArgs []string) {
	t.Helper()
	args := append([]string{"destroy", "-auto-approve", "-input=false", "-no-color"}, extraArgs...)
	runTofu(t, dir, env, args...)
}

func runTofu(t *testing.T, dir string, env []string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandBudget)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tofu", args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("tofu %v timed out\n%s", args, out)
	}
	if err != nil {
		t.Fatalf("tofu %v: %v\n%s", args, err, out)
	}
}

// ----- fakegcp lifecycle -----

// buildFakegcpOnce builds cmd/fakegcp once per test process and caches
// the binary path. Returns the path to the built binary.
func buildFakegcpOnce(t *testing.T, repoRoot string) string {
	t.Helper()
	fakegcpBinaryOnce.Do(func() {
		out := filepath.Join(os.TempDir(), fmt.Sprintf("fakegcp-e2e-%d", os.Getpid()))
		cmd := exec.Command("go", "build", "-o", out, "./cmd/fakegcp")
		cmd.Dir = repoRoot
		if combined, err := cmd.CombinedOutput(); err != nil {
			fakegcpBinaryErr = fmt.Errorf("build fakegcp: %v\n%s", err, combined)
			return
		}
		fakegcpBinaryPath = out
	})
	if fakegcpBinaryErr != nil {
		t.Fatalf("%v", fakegcpBinaryErr)
	}
	return fakegcpBinaryPath
}

// startFakegcp launches the fakegcp binary on the given port and registers
// a t.Cleanup that kills the process at test end. Blocks until /mock/reset
// responds (or bootBudget elapses).
func startFakegcp(t *testing.T, bin string, port int) {
	t.Helper()
	cmd := exec.Command(bin, "--port", fmt.Sprintf("%d", port))
	// Discard stdout/stderr — the binary's logger is noisy and we only care
	// about HTTP responses. If a test fails we have tofu output in the
	// fatal message which is more useful for debugging.
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fakegcp: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	})
	if err := waitForFakegcp(port, bootBudget); err != nil {
		t.Fatalf("fakegcp on port %d did not become ready: %v", port, err)
	}
}

// waitForFakegcp polls POST /mock/reset until it returns 2xx or budget
// elapses. /mock/reset is the cheapest admin endpoint to probe with.
func waitForFakegcp(port int, budget time.Duration) error {
	url := fmt.Sprintf("http://%s:%d/mock/reset", defaultFakegcpHost, port)
	deadline := time.Now().Add(budget)
	var lastErr error
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodPost, url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return nil
			}
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	return lastErr
}

// pickFreePort asks the kernel for an ephemeral port and immediately closes
// the listener so fakegcp can bind it. There's a TOCTOU window but it's
// acceptable for a test harness.
func pickFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pick free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

// ----- file/yaml helpers -----

// copyDir copies all regular files from src into dst (non-recursive: all
// fakegcp examples are flat — main.tf, providers.tf, *.tfvars).
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

// rewriteEndpointPort rewrites every `localhost:8080` substring in *.tf
// under dir to `localhost:<port>`. Matches the substitution pattern in
// scripts/e2e.sh; intentionally simple (string replace, not HCL parse)
// because every providers.tf in this repo uses that exact substring.
func rewriteEndpointPort(dir string, port int) error {
	target := fmt.Sprintf("localhost:%d", port)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tf") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		updated := strings.ReplaceAll(string(data), "localhost:8080", target)
		if updated == string(data) {
			continue
		}
		if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// loadKnownBroken parses examples/known_broken.yaml and returns a lookup
// keyed by "<tree>/<name>". Missing file → empty index (no-op).
func loadKnownBroken(t *testing.T, root string) brokenIndex {
	t.Helper()
	path := filepath.Join(root, "examples", knownBrokenFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return brokenIndex{}
		}
		t.Fatalf("read %s: %v", path, err)
	}
	var bl brokenList
	if err := yaml.Unmarshal(data, &bl); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	idx := make(brokenIndex, len(bl.Entries))
	for _, e := range bl.Entries {
		if e.Dir == "" {
			t.Fatalf("%s entry missing `dir`: %+v", knownBrokenFileName, e)
		}
		if e.Ticket == "" {
			t.Fatalf("%s entry %q missing `ticket`", knownBrokenFileName, e.Dir)
		}
		idx[e.Dir] = e
	}
	return idx
}

// ----- gating helpers -----

func requireE2EGate(t *testing.T) {
	t.Helper()
	if os.Getenv(gateEnvVar) != "1" {
		t.Skipf("set %s=1 to run example smoke tests (requires tofu + builds fakegcp from ./cmd/fakegcp)", gateEnvVar)
	}
}

func requireTofu(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Skipf("tofu not on PATH: %v", err)
	}
}

// repoRoot walks upward from this file until it finds go.mod. Mirror of
// fakeaws/examples/provider_smoke_test.go's helper.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate fakegcp repo root from %s", file)
	return ""
}
