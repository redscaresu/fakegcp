package handlers_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/redscaresu/fakegcp/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// HTTP-layer ON DELETE CASCADE coverage for S41-T4. Each test
// creates a parent resource and a child resource through the public
// API, deletes the parent, and asserts the children disappear (GET
// returns 404). Mirrors the cascade rules declared in
// repository.go's migrate() schema:
//
//   container_clusters       -> container_node_pools
//   sql_instances            -> sql_databases
//   sql_instances            -> sql_users
//   iam_service_accounts     -> iam_sa_keys
//   compute_routers          -> compute_router_nats
//   secretmanager_secrets    -> secretmanager_versions
//
// The dns_managed_zones -> dns_record_sets relationship is also a
// CASCADE in the schema but the DNS zone handler refuses to delete
// a non-empty zone (returns 409 in-use), so the cascade can never
// fire via the public API. That contract is already pinned by
// TestDNSZoneDeleteWithRecords in handlers_test.go and is not
// re-tested here.
//
// Some of these are partially covered by TestRouterDeleteWithNATs,
// TestSADeleteCascadesKeys, TestSecretDeleteWithVersions in
// handlers_test.go, which assert the parent DELETE returns 200 but
// don't verify the children are gone via GET. The tests below
// extend that contract to the post-cascade GET → 404 assertion.

// TestClusterDeleteCascadesNodePools: deleting a GKE cluster wipes
// every nodePool underneath it (container_node_pools has ON DELETE
// CASCADE on the cluster FK).
func TestClusterDeleteCascadesNodePools(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	clusterName := "cascade-cluster"
	mustCreate(t, srv, testutil.ContainerPath(project, location, "clusters"), map[string]any{
		"cluster": map[string]any{"name": clusterName},
	})

	poolNames := []string{"pool-a", "pool-b"}
	for _, n := range poolNames {
		mustCreate(t, srv, testutil.ContainerPath(project, location, "clusters", clusterName, "nodePools"), map[string]any{
			"nodePool": map[string]any{"name": n},
		})
	}

	resp, _ := testutil.DoDelete(t, srv, testutil.ContainerPath(project, location, "clusters", clusterName))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Cluster itself is gone.
	resp, _ = testutil.DoGet(t, srv, testutil.ContainerPath(project, location, "clusters", clusterName))
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// And every nodePool is gone too.
	for _, n := range poolNames {
		resp, _ := testutil.DoGet(t, srv, testutil.ContainerPath(project, location, "clusters", clusterName, "nodePools", n))
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, "nodePool %s should have been cascade-deleted", n)
	}
}

// TestSQLInstanceDeleteCascadesDatabases: deleting a Cloud SQL
// instance wipes every database underneath it.
func TestSQLInstanceDeleteCascadesDatabases(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	instanceName := "cascade-sql"
	mustCreate(t, srv, testutil.SQLPath(project, "instances"), map[string]any{
		"name": instanceName,
	})

	dbNames := []string{"app", "audit"}
	for _, d := range dbNames {
		mustCreate(t, srv, testutil.SQLPath(project, "instances", instanceName, "databases"), map[string]any{
			"name": d,
		})
	}

	resp, _ := testutil.DoDelete(t, srv, testutil.SQLPath(project, "instances", instanceName))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Each database is gone.
	for _, d := range dbNames {
		resp, _ := testutil.DoGet(t, srv, testutil.SQLPath(project, "instances", instanceName, "databases", d))
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, "database %s should have been cascade-deleted", d)
	}
}

// TestSQLInstanceDeleteCascadesUsers: deleting a Cloud SQL instance
// wipes every SQL user underneath it. The List handler doesn't
// itself FK-check the parent instance (it just returns the rows
// matching instance_name), so we assert the post-cascade list is
// empty AND the parent instance GET returns 404. The combination
// proves the cascade fired: rows were created, parent went away,
// rows are no longer queryable.
//
// NOTE on handler-level gap: ListSQLUsers should arguably 404 when
// the instance is gone (real Cloud SQL does), matching the same
// gap noted on ListSAKeys in fk_violation_test.go. Filed as a
// follow-up — not fixed in this ticket per S41 acceptance.
func TestSQLInstanceDeleteCascadesUsers(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	instanceName := "cascade-sql-users"
	mustCreate(t, srv, testutil.SQLPath(project, "instances"), map[string]any{
		"name": instanceName,
	})
	mustCreate(t, srv, testutil.SQLPath(project, "instances", instanceName, "users"), map[string]any{
		"name":     "deploy-user",
		"password": "p1",
	})

	// Confirm the user is reachable before the cascade.
	listResp, listBody := testutil.DoGet(t, srv, testutil.SQLPath(project, "instances", instanceName, "users"))
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	require.Contains(t, listBody, "items")
	preItems, _ := listBody["items"].([]any)
	require.NotEmpty(t, preItems, "expected the user to be reachable before parent delete")

	resp, _ := testutil.DoDelete(t, srv, testutil.SQLPath(project, "instances", instanceName))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Parent instance is gone.
	resp, _ = testutil.DoGet(t, srv, testutil.SQLPath(project, "instances", instanceName))
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// Users for that instance are no longer queryable. ListSQLUsers
	// returns 200 with an empty items list (today's fakegcp behavior),
	// which is sufficient to prove the rows were cascade-deleted.
	listResp, listBody = testutil.DoGet(t, srv, testutil.SQLPath(project, "instances", instanceName, "users"))
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	if items, ok := listBody["items"].([]any); ok {
		assert.Empty(t, items, "expected zero users after parent SQL instance delete")
	}
}

// TestServiceAccountDeleteCascadesKeys: deleting an IAM service
// account wipes every key it owns. Extends the existing
// TestSADeleteCascadesKeys (which only asserts the DELETE returns
// 200) with a post-delete GET → 404 on the key.
func TestServiceAccountDeleteCascadesKeys(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	mustCreate(t, srv, testutil.IAMPath(project, "serviceAccounts"), map[string]any{
		"accountId":      "cascade-sa-keys",
		"serviceAccount": map[string]any{"displayName": "Cascade"},
	})
	email := "cascade-sa-keys@" + project + ".iam.gserviceaccount.com"
	escEmail := url.PathEscape(email)

	keyResp, keyBody := testutil.DoCreate(t, srv, testutil.IAMPath(project, "serviceAccounts", escEmail, "keys"), map[string]any{})
	require.Equal(t, http.StatusOK, keyResp.StatusCode)
	keyName := requireStringField(t, keyBody, "name")
	// Trailing path segment is the key id.
	keyID := keyName[len("projects/"+project+"/serviceAccounts/"+email+"/keys/"):]

	// Delete the SA.
	resp, _ := testutil.DoDelete(t, srv, testutil.IAMPath(project, "serviceAccounts", escEmail))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Key must be gone too (ON DELETE CASCADE on iam_sa_keys).
	resp, _ = testutil.DoGet(t, srv, testutil.IAMPath(project, "serviceAccounts", escEmail, "keys", keyID))
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestRouterDeleteCascadesNATs extends TestRouterDeleteWithNATs by
// creating multiple NATs and asserting GET → 404 for each after the
// router is deleted. The existing test only asserts the cascade for
// a single NAT.
func TestRouterDeleteCascadesNATs(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	mustCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{
		"name": "cascade-router-net",
	})
	mustCreate(t, srv, testutil.ComputePath(project, "regions", region, "routers"), map[string]any{
		"name":    "cascade-router",
		"network": "projects/" + project + "/global/networks/cascade-router-net",
	})

	natNames := []string{"nat-a", "nat-b", "nat-c"}
	for _, n := range natNames {
		mustCreate(t, srv, testutil.ComputePath(project, "regions", region, "routers", "cascade-router", "nats"), map[string]any{
			"name": n,
		})
	}

	resp, _ := testutil.DoDelete(t, srv, testutil.ComputePath(project, "regions", region, "routers", "cascade-router"))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	for _, n := range natNames {
		resp, _ := testutil.DoGet(t, srv, testutil.ComputePath(project, "regions", region, "routers", "cascade-router", "nats", n))
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, "router nat %s should have been cascade-deleted", n)
	}
}

// TestSecretDeleteCascadesVersions extends
// TestSecretDeleteWithVersions by creating multiple versions and
// asserting each is gone after the parent secret is deleted.
func TestSecretDeleteCascadesVersions(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	mustCreate(t, srv, testutil.IAMPath(project, "secrets"), map[string]any{
		"secretId":    "cascade-secret",
		"replication": map[string]any{"automatic": map[string]any{}},
	})
	// Two versions.
	mustCreate(t, srv, testutil.IAMPath(project, "secrets", "cascade-secret:addVersion"), map[string]any{
		"payload": map[string]any{"data": "djE="}, // "v1"
	})
	mustCreate(t, srv, testutil.IAMPath(project, "secrets", "cascade-secret:addVersion"), map[string]any{
		"payload": map[string]any{"data": "djI="}, // "v2"
	})

	resp, _ := testutil.DoDelete(t, srv, testutil.IAMPath(project, "secrets", "cascade-secret"))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	for _, v := range []string{"1", "2"} {
		resp, _ := testutil.DoGet(t, srv, testutil.IAMPath(project, "secrets", "cascade-secret", "versions", v))
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, "secret version %s should have been cascade-deleted", v)
	}
}
