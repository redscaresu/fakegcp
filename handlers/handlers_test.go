package handlers_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/redscaresu/fakegcp/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	project  = "test-project"
	zone     = "us-central1-a"
	region   = "us-central1"
	location = "us-central1"
)

func doNoAuthJSON(t *testing.T, srvURL, method, path string, body any) (*http.Response, map[string]any) {
	t.Helper()

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, srvURL+path, reqBody)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	if len(raw) == 0 {
		return resp, nil
	}

	out := map[string]any{}
	require.NoError(t, json.Unmarshal(raw, &out))
	return resp, out
}

func assertOperationDone(t *testing.T, resp *http.Response, body map[string]any) {
	t.Helper()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, body)
	assert.Equal(t, "DONE", body["status"])
}

func requireStringField(t *testing.T, body map[string]any, key string) string {
	t.Helper()
	require.Contains(t, body, key)
	val, ok := body[key].(string)
	require.True(t, ok, "field %s should be string", key)
	return val
}

func requireListField(t *testing.T, body map[string]any, key string) []any {
	t.Helper()
	require.Contains(t, body, key)
	items, ok := body[key].([]any)
	require.True(t, ok, "field %s should be array", key)
	return items
}

func TestAuth(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	computePath := testutil.ComputePath(project, "global", "networks")
	resp, _ := doNoAuthJSON(t, srv.URL, http.MethodGet, computePath, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	adminResp, _ := doNoAuthJSON(t, srv.URL, http.MethodGet, "/mock/state", nil)
	assert.Equal(t, http.StatusOK, adminResp.StatusCode)
}

func TestNetworkCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	networksPath := testutil.ComputePath(project, "global", "networks")

	resp, body := testutil.DoCreate(t, srv, networksPath, map[string]any{"name": "test-net"})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoGet(t, srv, testutil.ComputePath(project, "global", "networks", "test-net"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "test-net", body["name"])
	assert.Equal(t, "compute#network", body["kind"])
	assert.Contains(t, requireStringField(t, body, "selfLink"), "/compute/v1/projects/"+project+"/global/networks/test-net")
	assert.Equal(t, true, body["autoCreateSubnetworks"])

	resp, body = testutil.DoGet(t, srv, networksPath)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	items := requireListField(t, body, "items")
	require.Len(t, items, 1)

	resp, body = testutil.DoPatch(t, srv, testutil.ComputePath(project, "global", "networks", "test-net"), map[string]any{"description": "updated"})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "global", "networks", "test-net"))
	assertOperationDone(t, resp, body)

	resp, _ = testutil.DoGet(t, srv, testutil.ComputePath(project, "global", "networks", "test-net"))
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestSubnetworkCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	_, _ = testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{"name": "test-net"})
	_, netBody := testutil.DoGet(t, srv, testutil.ComputePath(project, "global", "networks", "test-net"))
	networkLink := requireStringField(t, netBody, "selfLink")

	resp, body := testutil.DoCreate(t, srv, testutil.ComputePath(project, "regions", region, "subnetworks"), map[string]any{
		"name":        "test-subnet",
		"network":     networkLink,
		"ipCidrRange": "10.0.0.0/24",
	})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoGet(t, srv, testutil.ComputePath(project, "regions", region, "subnetworks", "test-subnet"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "test-subnet", body["name"])
	assert.Equal(t, region, body["region"])
	assert.Equal(t, "compute#subnetwork", body["kind"])
	assert.NotEmpty(t, requireStringField(t, body, "gatewayAddress"))
	assert.NotEmpty(t, requireStringField(t, body, "fingerprint"))

	resp, body = testutil.DoGet(t, srv, testutil.ComputePath(project, "regions", region, "subnetworks"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, requireListField(t, body, "items"), 1)

	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "regions", region, "subnetworks", "test-subnet"))
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "global", "networks", "test-net"))
	assertOperationDone(t, resp, body)
}

func TestSubnetworkFKViolation(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "regions", region, "subnetworks"), map[string]any{
		"name":        "test-subnet",
		"network":     "https://example.invalid/compute/v1/projects/test-project/global/networks/does-not-exist",
		"ipCidrRange": "10.0.0.0/24",
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestFirewallCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	_, _ = testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{"name": "test-net"})
	_, netBody := testutil.DoGet(t, srv, testutil.ComputePath(project, "global", "networks", "test-net"))
	networkLink := requireStringField(t, netBody, "selfLink")

	resp, body := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "firewalls"), map[string]any{
		"name":    "test-fw",
		"network": networkLink,
	})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoGet(t, srv, testutil.ComputePath(project, "global", "firewalls", "test-fw"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "INGRESS", body["direction"])

	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "global", "firewalls", "test-fw"))
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "global", "networks", "test-net"))
	assertOperationDone(t, resp, body)
}

func TestFirewallFKViolation(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "firewalls"), map[string]any{
		"name":    "test-fw",
		"network": "https://example.invalid/compute/v1/projects/test-project/global/networks/does-not-exist",
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestInstanceCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	instancesPath := testutil.ComputePath(project, "zones", zone, "instances")

	resp, body := testutil.DoCreate(t, srv, instancesPath, map[string]any{"name": "test-vm"})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoGet(t, srv, testutil.ComputePath(project, "zones", zone, "instances", "test-vm"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "test-vm", body["name"])
	assert.Equal(t, "RUNNING", body["status"])
	assert.Equal(t, "compute#instance", body["kind"])
	assert.Equal(t, zone, body["zone"])
	assert.Contains(t, requireStringField(t, body, "selfLink"), "/compute/v1/projects/"+project+"/zones/"+zone+"/instances/test-vm")

	resp, body = testutil.DoGet(t, srv, instancesPath)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, requireListField(t, body, "items"), 1)

	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "zones", zone, "instances", "test-vm"))
	assertOperationDone(t, resp, body)

	resp, _ = testutil.DoGet(t, srv, testutil.ComputePath(project, "zones", zone, "instances", "test-vm"))
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestDiskCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, body := testutil.DoCreate(t, srv, testutil.ComputePath(project, "zones", zone, "disks"), map[string]any{"name": "test-disk"})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoGet(t, srv, testutil.ComputePath(project, "zones", zone, "disks", "test-disk"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "READY", body["status"])
	assert.Equal(t, "compute#disk", body["kind"])

	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "zones", zone, "disks", "test-disk"))
	assertOperationDone(t, resp, body)
}

func TestAddressCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, body := testutil.DoCreate(t, srv, testutil.ComputePath(project, "regions", region, "addresses"), map[string]any{"name": "test-addr"})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoGet(t, srv, testutil.ComputePath(project, "regions", region, "addresses", "test-addr"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "RESERVED", body["status"])
	assert.Equal(t, "compute#address", body["kind"])
	assert.NotEmpty(t, requireStringField(t, body, "address"))

	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "regions", region, "addresses", "test-addr"))
	assertOperationDone(t, resp, body)
}

func TestGKEClusterCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	clustersPath := testutil.ContainerPath(project, location, "clusters")

	resp, body := testutil.DoCreate(t, srv, clustersPath, map[string]any{
		"cluster": map[string]any{"name": "test-cluster"},
	})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoGet(t, srv, testutil.ContainerPath(project, location, "clusters", "test-cluster"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "test-cluster", body["name"])
	assert.Equal(t, "RUNNING", body["status"])
	assert.NotEmpty(t, requireStringField(t, body, "endpoint"))

	resp, body = testutil.DoGet(t, srv, clustersPath)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "clusters")

	resp, body = testutil.DoPut(t, srv, testutil.ContainerPath(project, location, "clusters", "test-cluster"), map[string]any{
		"description": "updated",
	})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoDelete(t, srv, testutil.ContainerPath(project, location, "clusters", "test-cluster"))
	assertOperationDone(t, resp, body)
}

func TestNodePoolCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	_, _ = testutil.DoCreate(t, srv, testutil.ContainerPath(project, location, "clusters"), map[string]any{
		"cluster": map[string]any{"name": "test-cluster"},
	})

	nodePoolsPath := testutil.ContainerPath(project, location, "clusters", "test-cluster", "nodePools")
	resp, body := testutil.DoCreate(t, srv, nodePoolsPath, map[string]any{
		"nodePool": map[string]any{"name": "test-pool"},
	})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoGet(t, srv, testutil.ContainerPath(project, location, "clusters", "test-cluster", "nodePools", "test-pool"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "test-pool", body["name"])
	assert.Equal(t, "RUNNING", body["status"])

	resp, body = testutil.DoGet(t, srv, nodePoolsPath)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "nodePools")

	resp, body = testutil.DoDelete(t, srv, testutil.ContainerPath(project, location, "clusters", "test-cluster", "nodePools", "test-pool"))
	assertOperationDone(t, resp, body)
}

func TestNodePoolFKViolation(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoCreate(t, srv, testutil.ContainerPath(project, location, "clusters", "missing-cluster", "nodePools"), map[string]any{
		"nodePool": map[string]any{"name": "test-pool"},
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestSQLInstanceCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	instancesPath := testutil.SQLPath(project, "instances")

	resp, body := testutil.DoCreate(t, srv, instancesPath, map[string]any{"name": "test-sql"})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoGet(t, srv, testutil.SQLPath(project, "instances", "test-sql"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "test-sql", body["name"])
	assert.Equal(t, "sql#instance", body["kind"])
	assert.Equal(t, "RUNNABLE", body["state"])
	assert.NotEmpty(t, requireStringField(t, body, "connectionName"))
	ipAddrs := requireListField(t, body, "ipAddresses")
	require.NotEmpty(t, ipAddrs)

	resp, body = testutil.DoGet(t, srv, instancesPath)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "items")

	resp, body = testutil.DoPatch(t, srv, testutil.SQLPath(project, "instances", "test-sql"), map[string]any{"settings": map[string]any{"tier": "db-f1-micro"}})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoDelete(t, srv, testutil.SQLPath(project, "instances", "test-sql"))
	assertOperationDone(t, resp, body)
}

func TestSQLDatabaseCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	_, _ = testutil.DoCreate(t, srv, testutil.SQLPath(project, "instances"), map[string]any{"name": "test-sql"})

	dbPath := testutil.SQLPath(project, "instances", "test-sql", "databases")
	resp, body := testutil.DoCreate(t, srv, dbPath, map[string]any{"name": "test-db"})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoGet(t, srv, testutil.SQLPath(project, "instances", "test-sql", "databases", "test-db"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "test-db", body["name"])
	assert.Equal(t, "sql#database", body["kind"])

	resp, body = testutil.DoGet(t, srv, dbPath)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "items")

	resp, body = testutil.DoDelete(t, srv, testutil.SQLPath(project, "instances", "test-sql", "databases", "test-db"))
	assertOperationDone(t, resp, body)
}

func TestSQLDatabaseFKViolation(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoCreate(t, srv, testutil.SQLPath(project, "instances", "missing-sql", "databases"), map[string]any{"name": "test-db"})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestSQLUserCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	_, _ = testutil.DoCreate(t, srv, testutil.SQLPath(project, "instances"), map[string]any{"name": "test-sql"})

	usersPath := testutil.SQLPath(project, "instances", "test-sql", "users")
	resp, body := testutil.DoCreate(t, srv, usersPath, map[string]any{"name": "test-user", "password": "secret"})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoGet(t, srv, usersPath)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "items")

	resp, body = testutil.DoDelete(t, srv, usersPath+"?name=test-user")
	assertOperationDone(t, resp, body)
}

func TestServiceAccountCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	accountsPath := testutil.IAMPath(project, "serviceAccounts")
	resp, body := testutil.DoCreate(t, srv, accountsPath, map[string]any{
		"accountId": "test-sa",
		"serviceAccount": map[string]any{
			"displayName": "Test",
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "test-sa@test-project.iam.gserviceaccount.com", body["email"])
	assert.NotEmpty(t, requireStringField(t, body, "uniqueId"))
	assert.Equal(t, "projects/test-project/serviceAccounts/test-sa@test-project.iam.gserviceaccount.com", body["name"])

	email := "test-sa@test-project.iam.gserviceaccount.com"
	resp, body = testutil.DoGet(t, srv, testutil.IAMPath(project, "serviceAccounts", url.PathEscape(email)))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, email, body["email"])

	resp, body = testutil.DoGet(t, srv, accountsPath)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "accounts")

	resp, body = testutil.DoDelete(t, srv, testutil.IAMPath(project, "serviceAccounts", url.PathEscape(email)))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, body)
}

func TestSAKeyCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	_, _ = testutil.DoCreate(t, srv, testutil.IAMPath(project, "serviceAccounts"), map[string]any{
		"accountId": "test-sa",
	})
	email := "test-sa@test-project.iam.gserviceaccount.com"
	keysPath := testutil.IAMPath(project, "serviceAccounts", url.PathEscape(email), "keys")

	resp, body := testutil.DoCreate(t, srv, keysPath, map[string]any{})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	name := requireStringField(t, body, "name")
	assert.Contains(t, name, "/serviceAccounts/"+email+"/keys/")
	assert.NotEmpty(t, requireStringField(t, body, "privateKeyData"))
	assert.Equal(t, "USER_MANAGED", body["keyType"])

	keyID := name[strings.LastIndex(name, "/")+1:]
	require.NotEmpty(t, keyID)

	resp, body = testutil.DoGet(t, srv, keysPath)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "keys")

	resp, body = testutil.DoDelete(t, srv, testutil.IAMPath(project, "serviceAccounts", url.PathEscape(email), "keys", keyID))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, body)
}

func TestSAKeyFKViolation(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoCreate(t, srv, testutil.IAMPath(project, "serviceAccounts", url.PathEscape("missing@test-project.iam.gserviceaccount.com"), "keys"), map[string]any{})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestBucketCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, body := testutil.DoCreate(t, srv, "/storage/v1/b?project="+project, map[string]any{
		"name":     "test-bucket",
		"location": "US",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "test-bucket", body["name"])

	resp, body = testutil.DoGet(t, srv, "/storage/v1/b/test-bucket")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "test-bucket", body["name"])
	assert.Equal(t, "storage#bucket", body["kind"])
	assert.NotEmpty(t, requireStringField(t, body, "timeCreated"))

	resp, body = testutil.DoGet(t, srv, "/storage/v1/b?project="+project)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "items")

	resp, body = testutil.DoPatch(t, srv, "/storage/v1/b/test-bucket", map[string]any{"labels": map[string]any{"env": "test"}})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "test-bucket", body["name"])

	resp, _ = testutil.DoDelete(t, srv, "/storage/v1/b/test-bucket")
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestAdminReset(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	_, _ = testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{"name": "test-net"})

	testutil.ResetState(t, srv)

	resp, _ := testutil.DoGet(t, srv, testutil.ComputePath(project, "global", "networks", "test-net"))
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestAdminState(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	_, _ = testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{"name": "test-net"})
	_, _ = testutil.DoCreate(t, srv, testutil.ComputePath(project, "zones", zone, "instances"), map[string]any{"name": "test-vm"})

	resp, body := testutil.DoGet(t, srv, "/mock/state")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "compute")

	resp, _ = testutil.DoGet(t, srv, "/mock/state/compute")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestNetworkDeleteWithSubnetworks(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	_, _ = testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{"name": "test-net"})
	_, netBody := testutil.DoGet(t, srv, testutil.ComputePath(project, "global", "networks", "test-net"))
	networkLink := requireStringField(t, netBody, "selfLink")
	_, _ = testutil.DoCreate(t, srv, testutil.ComputePath(project, "regions", region, "subnetworks"), map[string]any{
		"name":        "test-subnet",
		"network":     networkLink,
		"ipCidrRange": "10.0.0.0/24",
	})

	resp, _ := testutil.DoDelete(t, srv, testutil.ComputePath(project, "global", "networks", "test-net"))
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestDuplicateResource(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	_, _ = testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{"name": "dup-net"})

	resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{"name": "dup-net"})
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestListEmptyOmitsItems(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, body := testutil.DoGet(t, srv, testutil.ComputePath(project, "zones", zone, "instances"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, body, "items")
}

func requireBindingsByRole(t *testing.T, body map[string]any) map[string][]string {
	t.Helper()

	require.Contains(t, body, "bindings")
	rawBindings, ok := body["bindings"].([]any)
	require.True(t, ok, "bindings should be an array")

	byRole := map[string][]string{}
	for _, item := range rawBindings {
		binding, ok := item.(map[string]any)
		require.True(t, ok, "binding should be object")
		role, ok := binding["role"].(string)
		require.True(t, ok, "binding.role should be string")

		rawMembers, ok := binding["members"].([]any)
		require.True(t, ok, "binding.members should be array")
		members := make([]string, 0, len(rawMembers))
		for _, m := range rawMembers {
			member, ok := m.(string)
			require.True(t, ok, "member should be string")
			members = append(members, member)
		}
		byRole[role] = members
	}
	return byRole
}

func TestIAMPolicy(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, body := testutil.DoCreate(t, srv, "/v1/projects/"+project+":setIamPolicy", map[string]any{
		"policy": map[string]any{
			"bindings": []map[string]any{
				{"role": "roles/viewer", "members": []string{"user:test@example.com"}},
			},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "bindings")
	require.Contains(t, body, "etag")

	resp, body = testutil.DoCreate(t, srv, "/v1/projects/"+project+":getIamPolicy", map[string]any{})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	bindings := requireBindingsByRole(t, body)
	require.Contains(t, bindings, "roles/viewer")
	assert.Equal(t, []string{"user:test@example.com"}, bindings["roles/viewer"])

	resp, body = testutil.DoCreate(t, srv, "/v1/projects/"+project+":setIamPolicy", map[string]any{
		"policy": map[string]any{
			"bindings": []map[string]any{
				{"role": "roles/viewer", "members": []string{"user:test@example.com"}},
				{"role": "roles/editor", "members": []string{"user:test@example.com"}},
			},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, body = testutil.DoCreate(t, srv, "/v1/projects/"+project+":getIamPolicy", map[string]any{})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	bindings = requireBindingsByRole(t, body)
	require.Contains(t, bindings, "roles/viewer")
	require.Contains(t, bindings, "roles/editor")
	assert.Equal(t, []string{"user:test@example.com"}, bindings["roles/viewer"])
	assert.Equal(t, []string{"user:test@example.com"}, bindings["roles/editor"])
}

func TestRouterCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	_, _ = testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{"name": "router-net"})
	_, netBody := testutil.DoGet(t, srv, testutil.ComputePath(project, "global", "networks", "router-net"))
	networkLink := requireStringField(t, netBody, "selfLink")

	routersPath := testutil.ComputePath(project, "regions", region, "routers")
	resp, body := testutil.DoCreate(t, srv, routersPath, map[string]any{
		"name":    "test-router",
		"network": networkLink,
	})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoGet(t, srv, testutil.ComputePath(project, "regions", region, "routers", "test-router"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "test-router", body["name"])
	assert.Equal(t, "compute#router", body["kind"])
	assert.Equal(t, region, body["region"])

	resp, body = testutil.DoGet(t, srv, routersPath)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, requireListField(t, body, "items"), 1)

	resp, body = testutil.DoPatch(t, srv, testutil.ComputePath(project, "regions", region, "routers", "test-router"), map[string]any{"description": "updated"})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "regions", region, "routers", "test-router"))
	assertOperationDone(t, resp, body)
}

func TestRouterFKViolation(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "regions", region, "routers"), map[string]any{
		"name":    "test-router",
		"network": "https://example.invalid/compute/v1/projects/test-project/global/networks/missing-net",
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestRouterNATCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	_, _ = testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{"name": "router-net"})
	_, netBody := testutil.DoGet(t, srv, testutil.ComputePath(project, "global", "networks", "router-net"))
	networkLink := requireStringField(t, netBody, "selfLink")
	_, _ = testutil.DoCreate(t, srv, testutil.ComputePath(project, "regions", region, "routers"), map[string]any{
		"name":    "test-router",
		"network": networkLink,
	})

	natsPath := testutil.ComputePath(project, "regions", region, "routers", "test-router", "nats")
	resp, body := testutil.DoCreate(t, srv, natsPath, map[string]any{"name": "test-nat"})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoGet(t, srv, testutil.ComputePath(project, "regions", region, "routers", "test-router", "nats", "test-nat"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "test-nat", body["name"])

	resp, body = testutil.DoGet(t, srv, natsPath)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, requireListField(t, body, "items"), 1)

	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "regions", region, "routers", "test-router", "nats", "test-nat"))
	assertOperationDone(t, resp, body)
}

func TestRouterNATFKViolation(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "regions", region, "routers", "missing-router", "nats"), map[string]any{"name": "test-nat"})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestRouterDeleteWithNATs(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	_, _ = testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{"name": "router-net"})
	_, netBody := testutil.DoGet(t, srv, testutil.ComputePath(project, "global", "networks", "router-net"))
	networkLink := requireStringField(t, netBody, "selfLink")
	_, _ = testutil.DoCreate(t, srv, testutil.ComputePath(project, "regions", region, "routers"), map[string]any{
		"name":    "test-router",
		"network": networkLink,
	})
	_, _ = testutil.DoCreate(t, srv, testutil.ComputePath(project, "regions", region, "routers", "test-router", "nats"), map[string]any{"name": "test-nat"})

	resp, body := testutil.DoDelete(t, srv, testutil.ComputePath(project, "regions", region, "routers", "test-router"))
	assertOperationDone(t, resp, body)

	resp, _ = testutil.DoGet(t, srv, testutil.ComputePath(project, "regions", region, "routers", "test-router", "nats", "test-nat"))
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestDNSZoneCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	zonesPath := "/dns/v1/projects/" + project + "/managedZones"
	resp, body := testutil.DoCreate(t, srv, zonesPath, map[string]any{
		"name":    "test-zone",
		"dnsName": "example.com.",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "test-zone", body["name"])

	resp, body = testutil.DoGet(t, srv, zonesPath+"/test-zone")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "test-zone", body["name"])
	assert.Equal(t, "dns#managedZone", body["kind"])
	assert.Equal(t, "example.com.", body["dnsName"])
	assert.NotEmpty(t, requireListField(t, body, "nameServers"))

	resp, body = testutil.DoGet(t, srv, zonesPath)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "managedZones")

	resp, body = testutil.DoDelete(t, srv, zonesPath+"/test-zone")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, body)
}

func TestDNSRecordSetCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	zonesPath := "/dns/v1/projects/" + project + "/managedZones"
	_, _ = testutil.DoCreate(t, srv, zonesPath, map[string]any{
		"name":    "test-zone",
		"dnsName": "example.com.",
	})

	rrsetsPath := zonesPath + "/test-zone/rrsets"
	resp, body := testutil.DoCreate(t, srv, rrsetsPath, map[string]any{
		"name":    "www.example.com.",
		"type":    "A",
		"ttl":     300,
		"rrdatas": []string{"1.2.3.4"},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "www.example.com.", body["name"])

	resp, _ = testutil.DoGet(t, srv, rrsetsPath+"/www.example.com./A")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, body = testutil.DoGet(t, srv, rrsetsPath)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "rrsets")

	resp, body = testutil.DoDelete(t, srv, rrsetsPath+"/www.example.com./A")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, body)
}

func TestDNSRecordSetFKViolation(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoCreate(t, srv, "/dns/v1/projects/"+project+"/managedZones/missing-zone/rrsets", map[string]any{
		"name":    "www.example.com.",
		"type":    "A",
		"ttl":     300,
		"rrdatas": []string{"1.2.3.4"},
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestDNSZoneDeleteWithRecords(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	zonesPath := "/dns/v1/projects/" + project + "/managedZones"
	_, _ = testutil.DoCreate(t, srv, zonesPath, map[string]any{
		"name":    "test-zone",
		"dnsName": "example.com.",
	})
	_, _ = testutil.DoCreate(t, srv, zonesPath+"/test-zone/rrsets", map[string]any{
		"name":    "www.example.com.",
		"type":    "A",
		"ttl":     300,
		"rrdatas": []string{"1.2.3.4"},
	})

	resp, body := testutil.DoDelete(t, srv, zonesPath+"/test-zone")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, body)

	resp, _ = testutil.DoGet(t, srv, zonesPath+"/test-zone/rrsets/www.example.com./A")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGlobalAddressCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, body := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "addresses"), map[string]any{"name": "test-gaddr"})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoGet(t, srv, testutil.ComputePath(project, "global", "addresses", "test-gaddr"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "test-gaddr", body["name"])
	assert.Equal(t, "compute#address", body["kind"])

	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "global", "addresses", "test-gaddr"))
	assertOperationDone(t, resp, body)
}

func TestHealthCheckCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	healthChecksPath := testutil.ComputePath(project, "global", "healthChecks")
	resp, body := testutil.DoCreate(t, srv, healthChecksPath, map[string]any{"name": "test-hc"})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoGet(t, srv, testutil.ComputePath(project, "global", "healthChecks", "test-hc"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "compute#healthCheck", body["kind"])
	assert.Equal(t, float64(5), body["checkIntervalSec"])
	assert.Equal(t, float64(5), body["timeoutSec"])
	assert.Equal(t, float64(2), body["healthyThreshold"])
	assert.Equal(t, float64(2), body["unhealthyThreshold"])

	resp, body = testutil.DoPatch(t, srv, testutil.ComputePath(project, "global", "healthChecks", "test-hc"), map[string]any{"checkIntervalSec": 10})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "global", "healthChecks", "test-hc"))
	assertOperationDone(t, resp, body)
}

func TestBackendServiceCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, body := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "backendServices"), map[string]any{"name": "test-bs"})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoGet(t, srv, testutil.ComputePath(project, "global", "backendServices", "test-bs"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "compute#backendService", body["kind"])

	resp, body = testutil.DoPatch(t, srv, testutil.ComputePath(project, "global", "backendServices", "test-bs"), map[string]any{"protocol": "HTTPS"})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "global", "backendServices", "test-bs"))
	assertOperationDone(t, resp, body)
}

func TestSSLCertificateCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, body := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "sslCertificates"), map[string]any{
		"name":        "test-cert",
		"certificate": "-----BEGIN CERTIFICATE-----\nMIIF...\n-----END CERTIFICATE-----",
		"privateKey":  "-----BEGIN PRIVATE KEY-----\nMIIE...\n-----END PRIVATE KEY-----",
	})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoGet(t, srv, testutil.ComputePath(project, "global", "sslCertificates", "test-cert"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "compute#sslCertificate", body["kind"])

	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "global", "sslCertificates", "test-cert"))
	assertOperationDone(t, resp, body)
}

func TestURLMapCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	// urlMap.defaultService is now FK-validated against backendServices,
	// so we have to set up the dependency first. The same chain holds in
	// production: a URL map cannot reference a non-existent backend.
	mustCreate(t, srv, testutil.ComputePath(project, "global", "healthChecks"), map[string]any{
		"name": "test-hc",
		"httpHealthCheck": map[string]any{"port": 80, "requestPath": "/"},
	})
	mustCreate(t, srv, testutil.ComputePath(project, "global", "backendServices"), map[string]any{
		"name":         "test-bs",
		"protocol":     "HTTP",
		"healthChecks": []any{"test-hc"},
	})

	resp, body := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "urlMaps"), map[string]any{
		"name":           "test-urlmap",
		"defaultService": "https://example.invalid/compute/v1/projects/test-project/global/backendServices/test-bs",
	})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoGet(t, srv, testutil.ComputePath(project, "global", "urlMaps", "test-urlmap"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "compute#urlMap", body["kind"])

	resp, body = testutil.DoPatch(t, srv, testutil.ComputePath(project, "global", "urlMaps", "test-urlmap"), map[string]any{"description": "updated"})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "global", "urlMaps", "test-urlmap"))
	assertOperationDone(t, resp, body)
}

func TestTargetHTTPSProxyCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	// targetHttpsProxies references urlMap + sslCertificates, both
	// FK-validated. Set up the prerequisites first.
	mustCreate(t, srv, testutil.ComputePath(project, "global", "healthChecks"), map[string]any{
		"name": "test-hc",
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

	resp, body := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "targetHttpsProxies"), map[string]any{
		"name":            "test-proxy",
		"urlMap":          "https://example.invalid/compute/v1/projects/test-project/global/urlMaps/test-urlmap",
		"sslCertificates": []string{"https://example.invalid/compute/v1/projects/test-project/global/sslCertificates/test-cert"},
	})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoGet(t, srv, testutil.ComputePath(project, "global", "targetHttpsProxies", "test-proxy"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "compute#targetHttpsProxy", body["kind"])

	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "global", "targetHttpsProxies", "test-proxy"))
	assertOperationDone(t, resp, body)
}

func TestGlobalForwardingRuleCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	// forwardingRules.target is FK-validated against the matching
	// proxy collection (targetHttpsProxies in this case). Set up the
	// full chain first.
	mustCreate(t, srv, testutil.ComputePath(project, "global", "healthChecks"), map[string]any{
		"name": "test-hc",
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

	resp, body := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "forwardingRules"), map[string]any{
		"name":       "test-fwd",
		"target":     "https://example.invalid/compute/v1/projects/test-project/global/targetHttpsProxies/test-proxy",
		"portRange":  "443",
		"IPProtocol": "TCP",
	})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoGet(t, srv, testutil.ComputePath(project, "global", "forwardingRules", "test-fwd"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "compute#forwardingRule", body["kind"])

	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "global", "forwardingRules", "test-fwd"))
	assertOperationDone(t, resp, body)
}

func TestSecretCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	secretsPath := testutil.IAMPath(project, "secrets")
	resp, body := testutil.DoCreate(t, srv, secretsPath, map[string]any{
		"secretId":    "test-secret",
		"replication": map[string]any{"automatic": map[string]any{}},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, requireStringField(t, body, "name"), "test-secret")

	resp, body = testutil.DoGet(t, srv, testutil.IAMPath(project, "secrets", "test-secret"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, requireStringField(t, body, "name"), "test-secret")

	resp, body = testutil.DoGet(t, srv, secretsPath)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "secrets")

	resp, body = testutil.DoDelete(t, srv, testutil.IAMPath(project, "secrets", "test-secret"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, body)
}

func TestSecretVersionCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	_, _ = testutil.DoCreate(t, srv, testutil.IAMPath(project, "secrets"), map[string]any{
		"secretId":    "test-secret",
		"replication": map[string]any{"automatic": map[string]any{}},
	})

	resp, body := testutil.DoCreate(t, srv, testutil.IAMPath(project, "secrets", "test-secret:addVersion"), map[string]any{
		"payload": map[string]any{"data": "aGVsbG8="},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, requireStringField(t, body, "name"), "/secrets/test-secret/versions/")

	resp, body = testutil.DoGet(t, srv, testutil.IAMPath(project, "secrets", "test-secret", "versions", "1"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "name")
	assert.Equal(t, "ENABLED", body["state"])

	resp, body = testutil.DoGet(t, srv, testutil.IAMPath(project, "secrets", "test-secret", "versions"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "versions")
}

func TestSecretDeleteWithVersions(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	_, _ = testutil.DoCreate(t, srv, testutil.IAMPath(project, "secrets"), map[string]any{
		"secretId":    "test-secret",
		"replication": map[string]any{"automatic": map[string]any{}},
	})
	_, _ = testutil.DoCreate(t, srv, testutil.IAMPath(project, "secrets", "test-secret:addVersion"), map[string]any{
		"payload": map[string]any{"data": "aGVsbG8="},
	})

	resp, body := testutil.DoDelete(t, srv, testutil.IAMPath(project, "secrets", "test-secret"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, body)

	resp, _ = testutil.DoGet(t, srv, testutil.IAMPath(project, "secrets", "test-secret", "versions", "1"))
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestPubSubTopicCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	topicsPath := testutil.IAMPath(project, "topics")
	resp, body := testutil.DoPut(t, srv, testutil.IAMPath(project, "topics", "test-topic"), map[string]any{})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "projects/"+project+"/topics/test-topic", body["name"])

	resp, body = testutil.DoGet(t, srv, testutil.IAMPath(project, "topics", "test-topic"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "projects/"+project+"/topics/test-topic", body["name"])

	resp, body = testutil.DoGet(t, srv, topicsPath)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "topics")

	resp, body = testutil.DoDelete(t, srv, testutil.IAMPath(project, "topics", "test-topic"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, body)
}

func TestPubSubSubscriptionCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	_, _ = testutil.DoPut(t, srv, testutil.IAMPath(project, "topics", "test-topic"), map[string]any{})

	subsPath := testutil.IAMPath(project, "subscriptions")
	resp, body := testutil.DoPut(t, srv, testutil.IAMPath(project, "subscriptions", "test-sub"), map[string]any{
		"topic": "projects/" + project + "/topics/test-topic",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "projects/"+project+"/subscriptions/test-sub", body["name"])

	resp, body = testutil.DoGet(t, srv, testutil.IAMPath(project, "subscriptions", "test-sub"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "projects/"+project+"/subscriptions/test-sub", body["name"])
	assert.Equal(t, "projects/"+project+"/topics/test-topic", body["topic"])

	resp, body = testutil.DoGet(t, srv, subsPath)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "subscriptions")

	resp, body = testutil.DoDelete(t, srv, testutil.IAMPath(project, "subscriptions", "test-sub"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, body)
}

func TestPubSubSubscriptionFKViolation(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoPut(t, srv, testutil.IAMPath(project, "subscriptions", "test-sub"), map[string]any{
		"topic": "projects/" + project + "/topics/missing-topic",
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestPubSubTopicDeleteWithSubscriptions(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	// Create topic
	resp, _ := testutil.DoPut(t, srv, testutil.IAMPath(project, "topics", "del-topic"), map[string]any{})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Create subscription referencing the topic
	resp, _ = testutil.DoPut(t, srv, testutil.IAMPath(project, "subscriptions", "del-sub"), map[string]any{
		"topic": "projects/" + project + "/topics/del-topic",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Delete topic — should succeed (subscriptions survive in real GCP)
	resp, _ = testutil.DoDelete(t, srv, testutil.IAMPath(project, "topics", "del-topic"))
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Subscription should still exist
	resp, _ = testutil.DoGet(t, srv, testutil.IAMPath(project, "subscriptions", "del-sub"))
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestSADeleteCascadesKeys(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	// Create SA
	resp, _ := testutil.DoCreate(t, srv, testutil.IAMPath(project, "serviceAccounts"), map[string]any{
		"accountId":      "cascade-sa",
		"serviceAccount": map[string]any{"displayName": "Cascade Test"},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	email := "cascade-sa@" + project + ".iam.gserviceaccount.com"

	// Create key
	resp, _ = testutil.DoCreate(t, srv, testutil.IAMPath(project, "serviceAccounts", email, "keys"), map[string]any{})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Delete SA — should cascade (keys deleted too)
	resp, _ = testutil.DoDelete(t, srv, testutil.IAMPath(project, "serviceAccounts", email))
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestCloudRunServiceCRUD(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	servicesPath := "/v2/projects/" + project + "/locations/" + location + "/services"
	resp, body := testutil.DoCreate(t, srv, servicesPath, map[string]any{"name": "test-svc"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, true, body["done"])

	resp, body = testutil.DoGet(t, srv, servicesPath+"/test-svc")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "projects/"+project+"/locations/"+location+"/services/test-svc", body["name"])
	assert.NotEmpty(t, requireStringField(t, body, "uri"))
	require.Contains(t, body, "conditions")

	resp, body = testutil.DoGet(t, srv, servicesPath)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "services")

	resp, body = testutil.DoPatch(t, srv, servicesPath+"/test-svc", map[string]any{"description": "updated"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, true, body["done"])

	resp, body = testutil.DoDelete(t, srv, servicesPath+"/test-svc")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, true, body["done"])
}

func TestFullLBStack(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	testutil.ResetState(t, srv)

	resp, body := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "healthChecks"), map[string]any{"name": "test-hc"})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "backendServices"), map[string]any{
		"name":         "test-bs",
		"healthChecks": []string{"/compute/v1/projects/" + project + "/global/healthChecks/test-hc"},
	})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "urlMaps"), map[string]any{
		"name":           "test-urlmap",
		"defaultService": "/compute/v1/projects/" + project + "/global/backendServices/test-bs",
	})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "sslCertificates"), map[string]any{
		"name":        "test-cert",
		"certificate": "cert-data",
		"privateKey":  "key-data",
	})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "targetHttpsProxies"), map[string]any{
		"name":            "test-proxy",
		"urlMap":          "/compute/v1/projects/" + project + "/global/urlMaps/test-urlmap",
		"sslCertificates": []string{"/compute/v1/projects/" + project + "/global/sslCertificates/test-cert"},
	})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "forwardingRules"), map[string]any{
		"name":       "test-fwd",
		"target":     "/compute/v1/projects/" + project + "/global/targetHttpsProxies/test-proxy",
		"portRange":  "443",
		"IPProtocol": "TCP",
	})
	assertOperationDone(t, resp, body)

	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "global", "forwardingRules", "test-fwd"))
	assertOperationDone(t, resp, body)
	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "global", "targetHttpsProxies", "test-proxy"))
	assertOperationDone(t, resp, body)
	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "global", "sslCertificates", "test-cert"))
	assertOperationDone(t, resp, body)
	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "global", "urlMaps", "test-urlmap"))
	assertOperationDone(t, resp, body)
	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "global", "backendServices", "test-bs"))
	assertOperationDone(t, resp, body)
	resp, body = testutil.DoDelete(t, srv, testutil.ComputePath(project, "global", "healthChecks", "test-hc"))
	assertOperationDone(t, resp, body)
}
