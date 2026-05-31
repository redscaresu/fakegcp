package handlers_test

import (
	"net/http"
	"testing"

	"github.com/redscaresu/fakegcp/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdminSnapshotState_OK confirms /mock/snapshot returns success and
// can be invoked with no auth (admin endpoints are unauthenticated by
// design — see fakegcp AGENTS.md). Surfaced as a regression boundary
// for infrafactory's incremental run mode that calls Snapshot before
// each iteration. Also asserts the response body shape so a regression
// that returned 200 with `{"status":"error"}` still trips the test.
func TestAdminSnapshotState_OK(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, body := doNoAuthJSON(t, srv.URL, http.MethodPost, "/mock/snapshot", nil)
	require.True(t, resp.StatusCode >= 200 && resp.StatusCode < 300, "got status %d", resp.StatusCode)
	if body != nil {
		// fakegcp's admin handler returns `{"status":"ok"}` on success.
		// Older revisions returned an empty body on 204 — accept either
		// shape but reject explicit error status strings.
		if status, ok := body["status"].(string); ok {
			assert.Equal(t, "ok", status)
		}
	}
}

// TestAdminResetState_ClearsSnapshotBaseline pins the documented reset
// semantics. The previous version of this test snapshotted the EMPTY
// state, which made the Restore-after-Reset assertion vacuous: even
// without baseline-clear, Restore would put the state back to "empty"
// and the test would pass. To uniquely exercise the clear, this
// version creates a "baseline" network FIRST, snapshots that
// non-empty state, then mutates by adding a SECOND network, resets,
// restores, and asserts NEITHER network exists. A regression that
// failed to clear the baseline would resurrect the baseline network
// and trip the test.
func TestAdminResetState_ClearsSnapshotBaseline(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	// 1. Create the baseline network we want Reset to clear from the
	//    snapshot.
	baselineResp, baselineBody := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{
		"name": "baseline-net",
	})
	assertOperationDone(t, baselineResp, baselineBody)

	// 2. Snapshot — captures the baseline-net state.
	snapResp, _ := doNoAuthJSON(t, srv.URL, http.MethodPost, "/mock/snapshot", nil)
	require.True(t, snapResp.StatusCode < 300)

	// 3. Mutate — add a second network.
	mutResp, mutBody := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{
		"name": "mutation-net",
	})
	assertOperationDone(t, mutResp, mutBody)

	// 4. Reset.
	resetResp, _ := doNoAuthJSON(t, srv.URL, http.MethodPost, "/mock/reset", nil)
	require.True(t, resetResp.StatusCode < 300, "reset status %d", resetResp.StatusCode)

	// 5. Confirm Reset cleared BOTH networks.
	listResp, listBody := testutil.DoGet(t, srv, testutil.ComputePath(project, "global", "networks"))
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	if items, ok := listBody["items"].([]any); ok && len(items) > 0 {
		t.Fatalf("expected empty post-reset network list, got %d items", len(items))
	}

	// 6. Restore after reset — Reset cleared the snapshot baseline, so
	//    Restore must NOT resurrect baseline-net. The failure mode we
	//    explicitly forbid is "Restore returns 200 AND baseline-net
	//    reappears".
	restoreResp, _ := doNoAuthJSON(t, srv.URL, http.MethodPost, "/mock/restore", nil)
	postResp, postBody := testutil.DoGet(t, srv, testutil.ComputePath(project, "global", "networks"))
	require.Equal(t, http.StatusOK, postResp.StatusCode)
	if items, ok := postBody["items"].([]any); ok && len(items) > 0 {
		t.Fatalf("expected no networks after reset+restore (regardless of restore status %d), got %d items",
			restoreResp.StatusCode, len(items))
	}
}

// TestAdminRestoreState_AfterMutationRestoresOriginalState exercises the
// full snapshot → mutate → restore round-trip. This is the contract
// infrafactory's run-loop relies on for "clean" vs "incremental" modes:
// after Reset/Restore, freshly-created mock state must vanish.
func TestAdminRestoreState_AfterMutationRestoresOriginalState(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	// Snapshot the empty state.
	resp, _ := doNoAuthJSON(t, srv.URL, http.MethodPost, "/mock/snapshot", nil)
	require.True(t, resp.StatusCode < 300, "snapshot status %d", resp.StatusCode)

	// Mutate: create a network.
	netResp, netBody := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{
		"name": "post-snapshot-net",
	})
	assertOperationDone(t, netResp, netBody)

	// Confirm the mutation landed.
	listResp, listBody := testutil.DoGet(t, srv, testutil.ComputePath(project, "global", "networks"))
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	require.Contains(t, listBody, "items")

	// Restore.
	restoreResp, _ := doNoAuthJSON(t, srv.URL, http.MethodPost, "/mock/restore", nil)
	require.True(t, restoreResp.StatusCode < 300, "restore status %d", restoreResp.StatusCode)

	// Mutation should be gone.
	postResp, postBody := testutil.DoGet(t, srv, testutil.ComputePath(project, "global", "networks"))
	require.Equal(t, http.StatusOK, postResp.StatusCode)
	if items, ok := postBody["items"].([]any); ok && len(items) > 0 {
		t.Fatalf("expected post-restore network list to be empty, got %d items", len(items))
	}
}

// TestListGlobalForwardingRulesReturnsCreatedRule guards the round-trip
// shape that infrafactory's GCP topology derivation
// (`internal/harness/topology_derive_gcp.go`) depends on: a created
// forwarding rule must show up under
// `state.lb.global_forwarding_rules` in /mock/state.
func TestListGlobalForwardingRulesReturnsCreatedRule(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	// Set up the LB chain that forwardingRules.target now requires.
	mustCreate(t, srv, testutil.ComputePath(project, "global", "healthChecks"), map[string]any{
		"name":            "test-hc",
		"httpHealthCheck": map[string]any{"port": 80, "requestPath": "/"},
	})
	mustCreate(t, srv, testutil.ComputePath(project, "global", "backendServices"), map[string]any{
		"name":         "test-bs",
		"protocol":     "HTTP",
		"healthChecks": []any{"test-hc"},
	})
	mustCreate(t, srv, testutil.ComputePath(project, "global", "urlMaps"), map[string]any{
		"name":           "test-urlmap",
		"defaultService": "test-bs",
	})
	mustCreate(t, srv, testutil.ComputePath(project, "global", "sslCertificates"), map[string]any{
		"name":        "test-cert",
		"privateKey":  "fake",
		"certificate": "fake",
	})
	mustCreate(t, srv, testutil.ComputePath(project, "global", "targetHttpsProxies"), map[string]any{
		"name":            "test-proxy",
		"urlMap":          "test-urlmap",
		"sslCertificates": []any{"test-cert"},
	})

	createPath := testutil.ComputePath(project, "global", "forwardingRules")
	createResp, createBody := testutil.DoCreate(t, srv, createPath, map[string]any{
		"name":      "list-test-fwd",
		"target":    "https://example.invalid/compute/v1/projects/" + project + "/global/targetHttpsProxies/test-proxy",
		"portRange": "80",
	})
	assertOperationDone(t, createResp, createBody)

	listResp, listBody := testutil.DoGet(t, srv, createPath)
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	require.Contains(t, listBody, "items")

	items, _ := listBody["items"].([]any)
	require.Greater(t, len(items), 0, "expected at least one forwarding rule item")
	first, _ := items[0].(map[string]any)
	require.NotNil(t, first)
	assert.Equal(t, "list-test-fwd", first["name"])
	assert.Equal(t, "compute#forwardingRule", first["kind"])

	// /mock/state must surface the rule in the LB section so the
	// topology derivation's rawGCPState struct can parse it.
	stateResp, stateBody := testutil.DoGet(t, srv, "/mock/state")
	require.Equal(t, http.StatusOK, stateResp.StatusCode)
	lb, _ := stateBody["lb"].(map[string]any)
	require.NotNil(t, lb, "expected lb section in /mock/state")
	rules, _ := lb["global_forwarding_rules"].([]any)
	require.Greater(t, len(rules), 0, "expected at least one global forwarding rule in mock state")
}

// TestCreateSQLInstancePersistsPrivateNetwork pins the
// settings.ipConfiguration.privateNetwork field that infrafactory's
// `policies/gcp/no_public_sql.rego` and topology derivation key off.
func TestCreateSQLInstancePersistsPrivateNetwork(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	mustCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{
		"name": "main-vpc",
	})

	createPath := testutil.SQLPath(project, "instances")
	privateNet := "projects/" + project + "/global/networks/main-vpc"
	resp, body := testutil.DoCreate(t, srv, createPath, map[string]any{
		"name":            "private-pg",
		"databaseVersion": "POSTGRES_15",
		"region":          region,
		"settings": map[string]any{
			"tier": "db-f1-micro",
			"ipConfiguration": map[string]any{
				"ipv4Enabled":    false,
				"privateNetwork": privateNet,
			},
		},
	})
	assertOperationDone(t, resp, body)

	getResp, getBody := testutil.DoGet(t, srv, testutil.SQLPath(project, "instances", "private-pg"))
	require.Equal(t, http.StatusOK, getResp.StatusCode)
	settings, _ := getBody["settings"].(map[string]any)
	require.NotNil(t, settings, "expected settings map")
	ipCfg, _ := settings["ipConfiguration"].(map[string]any)
	require.NotNil(t, ipCfg, "expected ipConfiguration sub-map")
	assert.Equal(t, privateNet, ipCfg["privateNetwork"])
	assert.Equal(t, false, ipCfg["ipv4Enabled"])
}

// TestAdminGetState_ReturnsAllResourcesGroupedByService closes the
// S41-T1 admin coverage gap: GET /mock/state must return a map
// keyed by service name, with each value carrying the resources
// created for that service. Today's shape is:
//
//	{
//	  "compute":  { networks: [...], instances: [...], ... },
//	  "container": { clusters: [...], nodePools: [...] },
//	  "sql":      { instances: [...], databases: [...], users: [...] },
//	  ...
//	}
//
// — see repository.FullState. The test creates one resource across
// three different services (compute, container, sql) and asserts
// each one shows up under its own service key.
func TestAdminGetState_ReturnsAllResourcesGroupedByService(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	// Seed: one resource in each of three services.
	mustCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{
		"name": "fullstate-net",
	})
	mustCreate(t, srv, testutil.ContainerPath(project, location, "clusters"), map[string]any{
		"cluster": map[string]any{"name": "fullstate-cluster"},
	})
	mustCreate(t, srv, testutil.SQLPath(project, "instances"), map[string]any{
		"name": "fullstate-sql",
	})

	resp, body := testutil.DoGet(t, srv, "/mock/state")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, body)

	// Every well-known service key must be present, even when
	// empty, so callers can iterate without nil-check fallbacks.
	for _, svc := range []string{"compute", "container", "sql", "iam", "storage", "operations", "dns", "lb", "secretmanager", "pubsub", "cloudrun"} {
		require.Contains(t, body, svc, "expected /mock/state to include %s key", svc)
		assert.IsType(t, map[string]any{}, body[svc], "expected %s to be a service map", svc)
	}

	// compute.networks contains our network.
	compute, _ := body["compute"].(map[string]any)
	require.NotNil(t, compute)
	networks, _ := compute["networks"].([]any)
	require.NotEmpty(t, networks, "expected at least one network under compute.networks")
	foundNet := false
	for _, n := range networks {
		nm, _ := n.(map[string]any)
		if nm == nil {
			continue
		}
		if nm["name"] == "fullstate-net" {
			foundNet = true
			break
		}
	}
	assert.True(t, foundNet, "expected compute.networks to include fullstate-net")

	// container.clusters contains our cluster.
	container, _ := body["container"].(map[string]any)
	require.NotNil(t, container)
	clusters, _ := container["clusters"].([]any)
	require.NotEmpty(t, clusters, "expected at least one cluster under container.clusters")

	// sql.instances contains our SQL instance.
	sqlState, _ := body["sql"].(map[string]any)
	require.NotNil(t, sqlState)
	sqlInstances, _ := sqlState["instances"].([]any)
	require.NotEmpty(t, sqlInstances, "expected at least one instance under sql.instances")
}

// TestAdminGetServiceState_ReturnsOnlyThatService pins the
// /mock/state/{service} route. For a known service name the
// handler must return that service's section only (same shape as
// FullState[service]); for an unknown service it must 404.
func TestAdminGetServiceState_ReturnsOnlyThatService(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	// Seed resources in three services so the assertion that
	// /mock/state/compute does NOT contain sql/container keys is
	// non-vacuous.
	mustCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{
		"name": "scoped-net",
	})
	mustCreate(t, srv, testutil.ContainerPath(project, location, "clusters"), map[string]any{
		"cluster": map[string]any{"name": "scoped-cluster"},
	})
	mustCreate(t, srv, testutil.SQLPath(project, "instances"), map[string]any{
		"name": "scoped-sql",
	})

	// /mock/state/compute returns only compute's sub-tree.
	resp, body := testutil.DoGet(t, srv, "/mock/state/compute")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, body)
	require.Contains(t, body, "networks", "compute service-state must surface networks")
	assert.NotContains(t, body, "clusters", "compute service-state must NOT include container.clusters")
	assert.NotContains(t, body, "databases", "compute service-state must NOT include sql.databases")
	networks, _ := body["networks"].([]any)
	require.NotEmpty(t, networks)

	// /mock/state/sql returns only sql's sub-tree.
	resp, body = testutil.DoGet(t, srv, "/mock/state/sql")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, body)
	require.Contains(t, body, "instances", "sql service-state must surface instances")
	assert.NotContains(t, body, "networks", "sql service-state must NOT leak compute.networks")
	sqlInstances, _ := body["instances"].([]any)
	require.NotEmpty(t, sqlInstances)

	// /mock/state/container returns only container's sub-tree.
	resp, body = testutil.DoGet(t, srv, "/mock/state/container")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, body)
	require.Contains(t, body, "clusters")
	require.Contains(t, body, "nodePools")

	// Unknown service → 404 (ServiceState maps unknown service to
	// models.ErrNotFound and writeDomainError emits the 404).
	resp, _ = testutil.DoGet(t, srv, "/mock/state/no-such-service")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestCreateClusterPersistsSubnetwork pins the GKE cluster Network and
// Subnetwork fields the GCP `vpc_required.rego` policy and topology
// derivation depend on. The VPC + subnet are seeded first because
// cluster create now FK-validates them.
func TestCreateClusterPersistsSubnetwork(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	mustCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{
		"name": "main-vpc",
	})
	mustCreate(t, srv, testutil.ComputePath(project, "regions", region, "subnetworks"), map[string]any{
		"name":        "main-subnet",
		"network":     "projects/" + project + "/global/networks/main-vpc",
		"ipCidrRange": "10.0.0.0/24",
	})

	createPath := testutil.ContainerPath(project, location, "clusters")
	resp, body := testutil.DoCreate(t, srv, createPath, map[string]any{
		"cluster": map[string]any{
			"name":       "vpc-cluster",
			"network":    "projects/" + project + "/global/networks/main-vpc",
			"subnetwork": "projects/" + project + "/regions/" + region + "/subnetworks/main-subnet",
		},
	})
	assertOperationDone(t, resp, body)

	getResp, getBody := testutil.DoGet(t, srv, testutil.ContainerPath(project, location, "clusters", "vpc-cluster"))
	require.Equal(t, http.StatusOK, getResp.StatusCode)
	assert.Equal(t, "projects/"+project+"/global/networks/main-vpc", getBody["network"])
	assert.Equal(t, "projects/"+project+"/regions/"+region+"/subnetworks/main-subnet", getBody["subnetwork"])
}
