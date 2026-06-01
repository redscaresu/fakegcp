package handlers_test

import (
	"net/http"
	"testing"

	"github.com/redscaresu/fakegcp/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSQLInstanceCreateAddsServerSideDefaults pins the N12 SQL fix.
// The v5 terraform-provider-google sqlInstance reader derefs nested
// settings fields (activationPolicy, backupConfiguration, ipConfiguration,
// maintenanceWindow) without nil guards. Without these defaults the
// provider panics with "Plugin did not respond" on ApplyResourceChange.
func TestSQLInstanceCreateAddsServerSideDefaults(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoCreate(t, srv,
		"/sql/v1beta4/projects/"+project+"/instances",
		map[string]any{
			"name":             "minimal-db",
			"databaseVersion":  "POSTGRES_14",
			"region":           "us-central1",
			"settings": map[string]any{
				"tier": "db-f1-micro",
			},
		})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, body := testutil.DoGet(t, srv, "/sql/v1beta4/projects/"+project+"/instances/minimal-db")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	settings, ok := body["settings"].(map[string]any)
	require.True(t, ok, "settings missing")
	assert.Equal(t, "ALWAYS", settings["activationPolicy"])
	assert.Equal(t, "PER_USE", settings["pricingPlan"])

	bc, ok := settings["backupConfiguration"].(map[string]any)
	require.True(t, ok, "backupConfiguration default missing")
	assert.NotNil(t, bc["enabled"])

	ipc, ok := settings["ipConfiguration"].(map[string]any)
	require.True(t, ok, "ipConfiguration default missing")
	_, hasAN := ipc["authorizedNetworks"]
	assert.True(t, hasAN, "ipConfiguration.authorizedNetworks default missing")

	mw, ok := settings["maintenanceWindow"].(map[string]any)
	require.True(t, ok, "maintenanceWindow default missing")
	assert.NotNil(t, mw["day"])
	assert.NotNil(t, mw["hour"])

	assert.NotEmpty(t, settings["dataDiskType"])
	assert.NotEmpty(t, settings["settingsVersion"])

	if _, ok := body["serverCaCert"].(map[string]any); !ok {
		t.Errorf("serverCaCert default missing: %v", body["serverCaCert"])
	}
	assert.NotEmpty(t, body["serviceAccountEmailAddress"])
}

// TestComputeInstanceCreateAddsServerSideDefaults pins the N12 Compute
// fix. The v5 terraform-provider-google compute_instance reader derefs
// networkInterfaces[].fingerprint, metadata.fingerprint, tags.fingerprint
// and scheduling.* without nil guards.
func TestComputeInstanceCreateAddsServerSideDefaults(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	// Pre-create the network the instance references; CreateInstance
	// enforces a FK on networkInterfaces[].network.
	_, _ = testutil.DoCreate(t, srv,
		testutil.ComputePath(project, "global", "networks"),
		map[string]any{"name": "test-net", "autoCreateSubnetworks": false})

	resp, _ := testutil.DoCreate(t, srv,
		testutil.ComputePath(project, "zones", "us-central1-a", "instances"),
		map[string]any{
			"name":        "test-vm",
			"machineType": "zones/us-central1-a/machineTypes/e2-small",
			"networkInterfaces": []any{
				map[string]any{"network": "projects/" + project + "/global/networks/test-net"},
			},
			"disks": []any{
				map[string]any{"initializeParams": map[string]any{"sourceImage": "img"}},
			},
		})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, body := testutil.DoGet(t, srv,
		testutil.ComputePath(project, "zones", "us-central1-a", "instances", "test-vm"))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	assert.NotEmpty(t, body["fingerprint"], "instance fingerprint missing")
	assert.NotEmpty(t, body["labelFingerprint"], "labelFingerprint missing")
	assert.NotEmpty(t, body["cpuPlatform"], "cpuPlatform missing")
	assert.NotNil(t, body["params"], "params missing")

	meta, ok := body["metadata"].(map[string]any)
	require.True(t, ok, "metadata default missing")
	assert.NotEmpty(t, meta["fingerprint"])

	tags, ok := body["tags"].(map[string]any)
	require.True(t, ok, "tags default missing")
	assert.NotEmpty(t, tags["fingerprint"])

	sched, ok := body["scheduling"].(map[string]any)
	require.True(t, ok, "scheduling default missing")
	assert.NotNil(t, sched["automaticRestart"])
	assert.NotEmpty(t, sched["onHostMaintenance"])

	_, ok = body["shieldedInstanceConfig"].(map[string]any)
	assert.True(t, ok, "shieldedInstanceConfig default missing")

	nics, ok := body["networkInterfaces"].([]any)
	require.True(t, ok && len(nics) == 1)
	nic0 := nics[0].(map[string]any)
	assert.NotEmpty(t, nic0["fingerprint"], "networkInterface fingerprint missing")
	assert.NotEmpty(t, nic0["kind"])

	disks, ok := body["disks"].([]any)
	require.True(t, ok && len(disks) == 1)
	disk0 := disks[0].(map[string]any)
	assert.NotEmpty(t, disk0["kind"])
	assert.Equal(t, "READ_WRITE", disk0["mode"])
	assert.Equal(t, true, disk0["boot"])
}
