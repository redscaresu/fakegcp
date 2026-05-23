package handlers_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/redscaresu/fakegcp/testutil"
	"github.com/stretchr/testify/assert"
)

// HTTP-layer FK violation coverage for S41-T3. Each test posts (or
// puts) a child resource whose required parent reference does not
// exist and asserts the handler returns 404 (mapped via
// writeDomainError / writeCreateError from models.ErrNotFound).
//
// These complement the repository-level FK tests in
// repository/repository_test.go by exercising the same constraint
// through the public REST surface — i.e. they catch regressions
// where a handler skips repo-layer validation, decodes the body
// incorrectly, or swallows the not-found error.
//
// A handful of related cases (TestSubnetworkFKViolation,
// TestFirewallFKViolation, TestNodePoolFKViolation,
// TestSQLDatabaseFKViolation, TestSAKeyFKViolation,
// TestRouterFKViolation, TestRouterNATFKViolation,
// TestDNSRecordSetFKViolation, TestPubSubSubscriptionFKViolation)
// already exist in handlers_test.go; the ones below add the
// remaining parent-child pairs called out in S41-T3.

// TestInstanceFKViolationMissingNetwork asserts a CreateInstance
// whose networkInterfaces[0].network points at a non-existent VPC is
// rejected with 404. validateInstanceNetworkInterfaces enforces the
// FK in the repo; the handler must surface it as
// models.ErrNotFound → 404.
func TestInstanceFKViolationMissingNetwork(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "zones", zone, "instances"), map[string]any{
		"name": "fk-vm-no-net",
		"networkInterfaces": []any{
			map[string]any{
				"network": "projects/" + project + "/global/networks/does-not-exist",
			},
		},
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestInstanceFKViolationMissingSubnetwork covers the subnetwork
// branch of validateInstanceNetworkInterfaces: network exists but
// the referenced subnet does not.
func TestInstanceFKViolationMissingSubnetwork(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	mustCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{
		"name": "fk-net",
	})

	resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "zones", zone, "instances"), map[string]any{
		"name": "fk-vm-no-subnet",
		"networkInterfaces": []any{
			map[string]any{
				"network":    "projects/" + project + "/global/networks/fk-net",
				"subnetwork": "projects/" + project + "/regions/" + region + "/subnetworks/missing-subnet",
			},
		},
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestSubnetworkFKViolationByRegionalPath complements the existing
// TestSubnetworkFKViolation (which uses a self-link). Here the
// subnet references a non-existent network via the relative
// `projects/<p>/global/networks/<n>` form to make sure both ref
// shapes are FK-validated.
func TestSubnetworkFKViolationByRegionalPath(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "regions", region, "subnetworks"), map[string]any{
		"name":        "fk-subnet",
		"network":     "projects/" + project + "/global/networks/missing-vpc",
		"ipCidrRange": "10.10.0.0/24",
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestFirewallFKViolationByRelativePath mirrors firewall FK
// enforcement using the relative-path reference shape (the existing
// TestFirewallFKViolation uses an absolute self-link).
func TestFirewallFKViolationByRelativePath(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "firewalls"), map[string]any{
		"name":    "fk-fw",
		"network": "projects/" + project + "/global/networks/missing-vpc",
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestSQLUserFKViolation: creating a user under a non-existent SQL
// instance must 404 (CreateSQLUser → repo.CreateSQLUser → FK on
// sql_users.instance_name).
func TestSQLUserFKViolation(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoCreate(t, srv, testutil.SQLPath(project, "instances", "missing-sql", "users"), map[string]any{
		"name":     "fk-user",
		"password": "pw",
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestSecretVersionFKViolation: addVersion against a non-existent
// secret must 404. secretmanager_versions has FK on secret_name.
func TestSecretVersionFKViolation(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoCreate(t, srv, testutil.IAMPath(project, "secrets", "missing-secret:addVersion"), map[string]any{
		"payload": map[string]any{"data": "aGVsbG8="},
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestBackendServiceFKViolationHealthCheck: a backend service whose
// healthChecks reference a missing health check is rejected with 404
// (validateListFK → writeDomainError).
func TestBackendServiceFKViolationHealthCheck(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "backendServices"), map[string]any{
		"name":         "fk-bs",
		"healthChecks": []any{"projects/" + project + "/global/healthChecks/missing-hc"},
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestURLMapFKViolationDefaultService: url map's defaultService
// references a non-existent backend service.
func TestURLMapFKViolationDefaultService(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "urlMaps"), map[string]any{
		"name":           "fk-urlmap",
		"defaultService": "projects/" + project + "/global/backendServices/missing-bs",
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestTargetHTTPSProxyFKViolationURLMap: missing urlMap reference.
func TestTargetHTTPSProxyFKViolationURLMap(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	// SSL cert exists; only the urlMap is missing.
	mustCreate(t, srv, testutil.ComputePath(project, "global", "sslCertificates"), map[string]any{
		"name":        "fk-cert",
		"certificate": "fake",
		"privateKey":  "fake",
	})

	resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "targetHttpsProxies"), map[string]any{
		"name":            "fk-proxy",
		"urlMap":          "projects/" + project + "/global/urlMaps/missing-urlmap",
		"sslCertificates": []any{"projects/" + project + "/global/sslCertificates/fk-cert"},
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestTargetHTTPSProxyFKViolationSSLCert: missing sslCertificates
// reference (urlMap is present so we isolate the cert FK).
func TestTargetHTTPSProxyFKViolationSSLCert(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	mustCreate(t, srv, testutil.ComputePath(project, "global", "healthChecks"), map[string]any{
		"name": "fk-hc",
	})
	mustCreate(t, srv, testutil.ComputePath(project, "global", "backendServices"), map[string]any{
		"name":         "fk-bs",
		"healthChecks": []any{"fk-hc"},
	})
	mustCreate(t, srv, testutil.ComputePath(project, "global", "urlMaps"), map[string]any{
		"name":           "fk-urlmap",
		"defaultService": "fk-bs",
	})

	resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "targetHttpsProxies"), map[string]any{
		"name":            "fk-proxy",
		"urlMap":          "fk-urlmap",
		"sslCertificates": []any{"projects/" + project + "/global/sslCertificates/missing-cert"},
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestForwardingRuleFKViolationTarget: forwarding rule pointing at a
// non-existent targetHttpsProxy. Per validateForwardingRuleTarget
// the path resolves to (project, "targetHttpsProxies", name); the
// missing-name lookup surfaces as 404.
func TestForwardingRuleFKViolationTarget(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "forwardingRules"), map[string]any{
		"name":      "fk-fwd",
		"target":    "projects/" + project + "/global/targetHttpsProxies/missing-proxy",
		"portRange": "443",
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestForwardingRuleFKViolationIPAddress: forwarding rule's
// IPAddress references a non-existent globalAddress by name (literal
// IPs short-circuit; bare names FK-resolve against
// compute_global_addresses).
func TestForwardingRuleFKViolationIPAddress(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	// Set up a valid target so only IPAddress FK fails.
	mustCreate(t, srv, testutil.ComputePath(project, "global", "healthChecks"), map[string]any{
		"name": "fk-hc",
	})
	mustCreate(t, srv, testutil.ComputePath(project, "global", "backendServices"), map[string]any{
		"name":         "fk-bs",
		"healthChecks": []any{"fk-hc"},
	})
	mustCreate(t, srv, testutil.ComputePath(project, "global", "urlMaps"), map[string]any{
		"name":           "fk-urlmap",
		"defaultService": "fk-bs",
	})
	mustCreate(t, srv, testutil.ComputePath(project, "global", "sslCertificates"), map[string]any{
		"name":        "fk-cert",
		"certificate": "fake",
		"privateKey":  "fake",
	})
	mustCreate(t, srv, testutil.ComputePath(project, "global", "targetHttpsProxies"), map[string]any{
		"name":            "fk-proxy",
		"urlMap":          "fk-urlmap",
		"sslCertificates": []any{"fk-cert"},
	})

	resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "forwardingRules"), map[string]any{
		"name":      "fk-fwd",
		"target":    "fk-proxy",
		"portRange": "443",
		"IPAddress": "missing-static-ip",
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestNodePoolFKViolationViaRelativePath complements the existing
// TestNodePoolFKViolation: same constraint, different ref shape (no
// cluster ever created, FK is on path param).
func TestNodePoolFKViolationViaRelativePath(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoCreate(t, srv, testutil.ContainerPath(project, location, "clusters", "ghost-cluster", "nodePools"), map[string]any{
		"nodePool": map[string]any{"name": "fk-pool"},
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestSAKeyGetFKViolation: GET an individual SA key under a missing
// service account must 404 — the keys table FKs on the SA email and
// the row genuinely doesn't exist, so this surfaces the FK violation
// at GET time (the closest analogue of "list children of missing
// parent" we can express without a handler change).
//
// NOTE on parent-existence enforcement gap: ListSAKeys today returns
// 200 with an empty list for a non-existent SA email rather than
// 404. Real Cloud IAM returns 404 in that case. This is a known
// fidelity gap to file as a separate follow-up — DO NOT fix in this
// ticket per the S41-T3 instructions. The GET-by-name path below
// gives us FK coverage without depending on that buggy List
// behaviour.
func TestSAKeyGetFKViolation(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	email := url.PathEscape("ghost@" + project + ".iam.gserviceaccount.com")
	resp, _ := testutil.DoGet(t, srv, testutil.IAMPath(project, "serviceAccounts", email, "keys", "any-key-id"))
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestDNSRecordSetGetFKViolation: GET rrset under a missing zone
// must 404 (parent missing, not a stale child reference).
func TestDNSRecordSetGetFKViolation(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoGet(t, srv, "/dns/v1/projects/"+project+"/managedZones/missing-zone/rrsets/www.example.com./A")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestRouterNATGetFKViolation: GET nat under a missing router must
// 404 — the router itself is the FK parent.
func TestRouterNATGetFKViolation(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoGet(t, srv, testutil.ComputePath(project, "regions", region, "routers", "ghost-router", "nats", "any-nat"))
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestSQLDatabaseGetFKViolation: GET sql database under a missing
// instance must 404 (instance is the FK parent).
func TestSQLDatabaseGetFKViolation(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoGet(t, srv, testutil.SQLPath(project, "instances", "ghost-sql", "databases", "any-db"))
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
