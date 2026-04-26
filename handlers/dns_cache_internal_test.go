package handlers

import "testing"

// TestResetDNSChangesNilsSnapshot pins the consistency fix from
// pass 18: resetDNSChanges must clear both the live cache and
// the snapshot baseline. Otherwise a later restoreDNSChanges
// could resurrect stale pre-reset state. This is a tiny internal
// test so it can read the unexported fields directly; the public
// admin lifecycle (/mock/reset → /mock/restore) can't reach this
// path because the repo's Reset() removes its .snapshot file,
// making /mock/restore fail before the in-memory restore runs.
func TestResetDNSChangesNilsSnapshot(t *testing.T) {
	app := &Application{
		dnsChanges: map[string]map[string]any{
			"p/z/before": {"id": "before"},
		},
	}
	app.snapshotDNSChanges()
	if app.dnsChangesSnapshot == nil {
		t.Fatal("snapshot did not capture baseline")
	}

	app.resetDNSChanges()

	if len(app.dnsChanges) != 0 {
		t.Errorf("live cache not cleared: %v", app.dnsChanges)
	}
	if app.dnsChangesSnapshot != nil {
		t.Errorf("snapshot baseline not cleared: %v", app.dnsChangesSnapshot)
	}

	// Re-snapshot after reset must capture the new (empty) state,
	// not the pre-reset baseline.
	app.snapshotDNSChanges()
	if len(app.dnsChangesSnapshot) != 0 {
		t.Errorf("re-snapshot did not capture empty post-reset state: %v", app.dnsChangesSnapshot)
	}

	// And restore from that empty baseline must leave the cache
	// empty, not resurrect the pre-reset entry.
	app.dnsChanges["p/z/after"] = map[string]any{"id": "after"}
	app.restoreDNSChanges()
	if len(app.dnsChanges) != 0 {
		t.Errorf("restore did not roll cache back to empty: %v", app.dnsChanges)
	}
}
