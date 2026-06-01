package handlers_test

import (
	"net/http"
	"testing"

	"github.com/redscaresu/fakegcp/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNodePoolCreateAddsServerSideDefaults pins the N12 NodePool fix.
// The v5 terraform-provider-google nodePool reader derefs nested
// fields (management.autoUpgrade, upgradeSettings.maxSurge,
// maxPodsConstraint.maxPodsPerNode, config.metadata) without nil
// guards. Without the server-side defaults, the provider panics
// with "Plugin did not respond" on ApplyResourceChange. Surfaced
// in gcp-gke-cluster + gcp-full-stack 2026-06-02 sweeps.
func TestNodePoolCreateAddsServerSideDefaults(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	// Pre-create the cluster the pool references.
	clusterPath := "/v1/projects/" + project + "/locations/us-central1/clusters"
	resp, _ := testutil.DoCreate(t, srv, clusterPath, map[string]any{
		"cluster": map[string]any{
			"name":             "test-cluster",
			"initialNodeCount": 1,
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	poolPath := "/v1/projects/" + project + "/locations/us-central1/clusters/test-cluster/nodePools"
	resp, _ = testutil.DoCreate(t, srv, poolPath, map[string]any{
		"nodePool": map[string]any{
			"name":             "test-pool",
			"initialNodeCount": 1,
			"config": map[string]any{
				"machineType": "e2-medium",
			},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// GET the pool back and assert the defaults landed.
	getPath := poolPath + "/test-pool"
	resp, body := testutil.DoGet(t, srv, getPath)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	mgmt, ok := body["management"].(map[string]any)
	require.True(t, ok, "management default missing")
	assert.Equal(t, true, mgmt["autoUpgrade"])
	assert.Equal(t, true, mgmt["autoRepair"])

	upgrade, ok := body["upgradeSettings"].(map[string]any)
	require.True(t, ok, "upgradeSettings default missing")
	assert.Equal(t, "SURGE", upgrade["strategy"])
	assert.NotNil(t, upgrade["maxSurge"])
	assert.NotNil(t, upgrade["maxUnavailable"])

	mpc, ok := body["maxPodsConstraint"].(map[string]any)
	require.True(t, ok, "maxPodsConstraint default missing")
	assert.NotNil(t, mpc["maxPodsPerNode"])

	cfg, ok := body["config"].(map[string]any)
	require.True(t, ok, "config missing")
	_, hasMetadata := cfg["metadata"]
	assert.True(t, hasMetadata, "config.metadata default missing")
	_, hasLabels := cfg["labels"]
	assert.True(t, hasLabels, "config.labels default missing")
	scopes, _ := cfg["oauthScopes"].([]any)
	assert.NotEmpty(t, scopes, "config.oauthScopes default missing")
	tags, hasTags := cfg["tags"].([]any)
	assert.True(t, hasTags && tags != nil, "config.tags default missing")
	taints, hasTaints := cfg["taints"].([]any)
	assert.True(t, hasTaints && taints != nil, "config.taints default missing")
	assert.NotEmpty(t, cfg["serviceAccount"], "config.serviceAccount default missing")
	assert.NotEmpty(t, cfg["imageType"], "config.imageType default missing")

	shielded, ok := cfg["shieldedInstanceConfig"].(map[string]any)
	require.True(t, ok, "config.shieldedInstanceConfig default missing")
	assert.NotNil(t, shielded["enableSecureBoot"])

	nc, ok := body["networkConfig"].(map[string]any)
	require.True(t, ok, "networkConfig default missing")
	assert.NotEmpty(t, nc["podIpv4CidrBlock"])
}

// TestNodePoolCreatePreservesCallerProvidedFields confirms the
// defaults are additive — if the caller sends management/upgradeSettings
// the response echoes those values rather than the defaults.
func TestNodePoolCreatePreservesCallerProvidedFields(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	clusterPath := "/v1/projects/" + project + "/locations/us-central1/clusters"
	_, _ = testutil.DoCreate(t, srv, clusterPath, map[string]any{
		"cluster": map[string]any{"name": "c2", "initialNodeCount": 1},
	})

	poolPath := "/v1/projects/" + project + "/locations/us-central1/clusters/c2/nodePools"
	resp, _ := testutil.DoCreate(t, srv, poolPath, map[string]any{
		"nodePool": map[string]any{
			"name": "p2",
			"management": map[string]any{
				"autoUpgrade": false,
				"autoRepair":  false,
			},
			"upgradeSettings": map[string]any{
				"maxSurge":       float64(3),
				"maxUnavailable": float64(1),
				"strategy":       "BLUE_GREEN",
			},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, body := testutil.DoGet(t, srv, poolPath+"/p2")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	mgmt := body["management"].(map[string]any)
	assert.Equal(t, false, mgmt["autoUpgrade"], "caller value should be preserved")
	assert.Equal(t, false, mgmt["autoRepair"], "caller value should be preserved")

	upgrade := body["upgradeSettings"].(map[string]any)
	assert.Equal(t, "BLUE_GREEN", upgrade["strategy"], "caller strategy preserved")
}
