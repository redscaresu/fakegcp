package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	require.NotNil(t, app.dnsChangesSnapshot, "snapshot did not capture baseline")

	app.resetDNSChanges()

	assert.Empty(t, app.dnsChanges, "live cache not cleared")
	assert.Nil(t, app.dnsChangesSnapshot, "snapshot baseline not cleared")

	// Re-snapshot after reset must capture the new (empty) state,
	// not the pre-reset baseline.
	app.snapshotDNSChanges()
	assert.Empty(t, app.dnsChangesSnapshot, "re-snapshot did not capture empty post-reset state")

	// And restore from that empty baseline must leave the cache
	// empty, not resurrect the pre-reset entry.
	app.dnsChanges["p/z/after"] = map[string]any{"id": "after"}
	app.restoreDNSChanges()
	assert.Empty(t, app.dnsChanges, "restore did not roll cache back to empty")
}
