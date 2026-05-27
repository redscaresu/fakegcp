package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/redscaresu/fakegcp/handlers"
	"github.com/redscaresu/fakegcp/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Package handlers regression catalogue.
//
// These tests encode high-value Terraform misconfiguration patterns
// documented under examples/misconfigured/. Each public TestRegression...
// function starts with the manifest-gated helper, then delegates to a
// check helper so TestRegressionSeedAuditNoVacuousPasses can distinguish
// landed assertions from pre-seeded stubs.

func requireHandlerImplemented(t *testing.T, id, slice, pattern string) {
	t.Helper()
	handlers.RequireHandlerImplementedForTest(t, id, slice, pattern)
}

// Cross-project FK rejection.
//
// Pattern: a self-link that embeds a different project must not be accepted by
// trailing-name collision. A local VPC named "shared" exists, but an instance
// reference to projects/other/global/networks/shared must still 404.
func TestRegressionCrossProjectFKRejection(t *testing.T) {
	requireHandlerImplemented(t, "compute", "M79", "cross-project fk")
	checkRegressionCrossProjectFKRejection(t)
}

// Wrong-collection FK rejection.
//
// Pattern: a path in the wrong collection must not satisfy an FK just because
// its final segment matches. A global address named "vpc" cannot stand in for
// a compute network named "vpc".
func TestRegressionWrongCollectionFKRejection(t *testing.T) {
	requireHandlerImplemented(t, "compute", "M79", "wrong collection fk")
	checkRegressionWrongCollectionFKRejection(t)
}

// Pub/Sub subscription topic lifecycle.
//
// Pattern: subscriptions validate the referenced topic on create, and deleting
// a topic leaves surviving subscriptions tombstoned instead of silently
// rebinding if a topic with the same name is recreated.
func TestRegressionPubSubSubscriptionTopicFK(t *testing.T) {
	requireHandlerImplemented(t, "pubsub", "M79", "subscription topic fk")
	checkRegressionPubSubSubscriptionTopicFK(t)
}

// DNS record-set parent-zone FK.
//
// Pattern: record sets are nested under a managed zone path. A missing zone
// must reject the create path with 404 rather than creating an orphan rrset.
func TestRegressionDNSRecordSetMissingZone(t *testing.T) {
	requireHandlerImplemented(t, "dns", "M79", "record set missing zone")
	checkRegressionDNSRecordSetMissingZone(t)
}

// Secret version parent-secret FK.
//
// Pattern: addVersion must prove the parent secret exists in the project before
// storing version state. A hand-written stale secret path must fail with 404.
func TestRegressionSecretVersionMissingSecret(t *testing.T) {
	requireHandlerImplemented(t, "secretmanager", "M79", "secret version missing secret")
	checkRegressionSecretVersionMissingSecret(t)
}

// Cloud SQL database parent-instance FK.
//
// Pattern: databases are addressed beneath SQL instances. Creating a database
// under a missing instance must return 404, not a malformed success envelope.
func TestRegressionSQLDatabaseMissingInstance(t *testing.T) {
	requireHandlerImplemented(t, "sql", "M79", "sql database missing instance")
	checkRegressionSQLDatabaseMissingInstance(t)
}

// GKE node-pool parent-cluster FK.
//
// Pattern: node pools are nested under clusters. A stale cluster string in HCL
// must surface as a create-path 404.
func TestRegressionGKENodePoolMissingCluster(t *testing.T) {
	requireHandlerImplemented(t, "container", "M79", "node pool missing cluster")
	checkRegressionGKENodePoolMissingCluster(t)
}

// Firewall network FK.
//
// Pattern: firewall.network is typed as a string, so plan cannot verify it.
// fakegcp must resolve it against compute networks and reject missing parents.
func TestRegressionFirewallWrongNetwork(t *testing.T) {
	requireHandlerImplemented(t, "compute", "M79", "firewall wrong network")
	checkRegressionFirewallWrongNetwork(t)
}

// Backend-service health-check FK.
//
// Pattern: healthChecks entries may be copied as raw self-links. The backend
// service create path must resolve every health check before insert.
func TestRegressionBackendServiceMissingHealthCheck(t *testing.T) {
	requireHandlerImplemented(t, "loadbalancer", "M79", "backend missing health check")
	checkRegressionBackendServiceMissingHealthCheck(t)
}

// Destroy idempotency on missing resources.
//
// Pattern: a repeated DELETE after remote state is already gone should return a
// clean GCP 404 domain error, not a 500 that hides the original destroy issue.
func TestRegressionDestroyIdempotencyMissingDelete(t *testing.T) {
	requireHandlerImplemented(t, "compute", "M79", "destroy idempotency missing delete")
	checkRegressionDestroyIdempotencyMissingDelete(t)
}

// Operation envelope shape.
//
// Pattern: GCP mutations return Operation objects. A create response must be a
// DONE operation with kind and targetLink, not the resource body directly.
func TestRegressionOperationEnvelopeShape(t *testing.T) {
	requireHandlerImplemented(t, "compute", "M79", "operation envelope shape")
	checkRegressionOperationEnvelopeShape(t)
}

// IAM service-account project scoping.
//
// Pattern: service account reads are project-scoped. A service account created
// in one project must not be returned through another project's path.
func TestRegressionIAMServiceAccountWrongProject(t *testing.T) {
	requireHandlerImplemented(t, "iam", "M79", "iam service account wrong project")
	checkRegressionIAMServiceAccountWrongProject(t)
}

// Pub/Sub ack deadline echo.
//
// Pattern: ackDeadlineSeconds is a write-then-read field the provider uses for
// drift detection. Create and PATCH must round-trip the current value.
func TestRegressionPubSubAckDeadlineEcho(t *testing.T) {
	requireHandlerImplemented(t, "pubsub", "M79", "pubsub ack deadline echo")
	checkRegressionPubSubAckDeadlineEcho(t)
}

func checkRegressionCrossProjectFKRejection(t *testing.T) {
	srv, cleanup := regressionServer(t)
	defer cleanup()
	mustCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{"name": "shared"})
	assertCreateStatus(t, srv, testutil.ComputePath(project, "zones", zone, "instances"), map[string]any{
		"name": "xproject-vm",
		"networkInterfaces": []any{
			map[string]any{"network": "projects/other-project/global/networks/shared"},
		},
	}, http.StatusNotFound)
}

func checkRegressionWrongCollectionFKRejection(t *testing.T) {
	srv, cleanup := regressionServer(t)
	defer cleanup()
	mustCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{"name": "vpc"})
	mustCreate(t, srv, testutil.ComputePath(project, "global", "addresses"), map[string]any{"name": "vpc"})
	assertCreateStatus(t, srv, testutil.ComputePath(project, "zones", zone, "instances"), map[string]any{
		"name": "wrong-collection-vm",
		"networkInterfaces": []any{
			map[string]any{"network": "projects/" + project + "/global/addresses/vpc"},
		},
	}, http.StatusNotFound)
}

func checkRegressionPubSubSubscriptionTopicFK(t *testing.T) {
	srv, cleanup := regressionServer(t)
	defer cleanup()
	base := testutil.IAMPath(project)
	assertPutStatus(t, srv, base+"/subscriptions/missing-topic-sub", map[string]any{
		"topic": "projects/" + project + "/topics/missing-topic",
	}, http.StatusNotFound)
	assertPutStatus(t, srv, base+"/topics/events", map[string]any{}, http.StatusOK)
	assertPutStatus(t, srv, base+"/subscriptions/events-sub", map[string]any{
		"topic": "projects/" + project + "/topics/events",
	}, http.StatusOK)
	resp, _ := testutil.DoDelete(t, srv, base+"/topics/events")
	assertDeleteStatus(t, resp.StatusCode, http.StatusOK)
	_, body := testutil.DoGet(t, srv, base+"/subscriptions/events-sub")
	assertFieldEquals(t, body, "topic", "_deleted-topic_")
}

func checkRegressionDNSRecordSetMissingZone(t *testing.T) {
	srv, cleanup := regressionServer(t)
	defer cleanup()
	assertCreateStatus(t, srv, "/dns/v1/projects/"+project+"/managedZones/missing-zone/changes", map[string]any{
		"additions": []any{
			map[string]any{"name": "host.example.invalid.", "type": "A", "ttl": 300, "rrdatas": []any{"192.0.2.10"}},
		},
	}, http.StatusNotFound)
}

func checkRegressionSecretVersionMissingSecret(t *testing.T) {
	srv, cleanup := regressionServer(t)
	defer cleanup()
	assertCreateStatus(t, srv, testutil.IAMPath(project, "secrets", "missing-secret:addVersion"), map[string]any{
		"payload": map[string]any{"data": "ZmFrZQ=="},
	}, http.StatusNotFound)
}

func checkRegressionSQLDatabaseMissingInstance(t *testing.T) {
	srv, cleanup := regressionServer(t)
	defer cleanup()
	assertCreateStatus(t, srv, testutil.SQLPath(project, "instances", "ghost-sql", "databases"), map[string]any{
		"name": "app",
	}, http.StatusNotFound)
}

func checkRegressionGKENodePoolMissingCluster(t *testing.T) {
	srv, cleanup := regressionServer(t)
	defer cleanup()
	assertCreateStatus(t, srv, testutil.ContainerPath(project, location, "clusters", "ghost-cluster", "nodePools"), map[string]any{
		"nodePool": map[string]any{"name": "app-pool", "initialNodeCount": 1},
	}, http.StatusNotFound)
}

func checkRegressionFirewallWrongNetwork(t *testing.T) {
	srv, cleanup := regressionServer(t)
	defer cleanup()
	assertCreateStatus(t, srv, testutil.ComputePath(project, "global", "firewalls"), map[string]any{
		"name":    "allow-ssh",
		"network": "projects/" + project + "/global/networks/does-not-exist",
		"allowed": []any{
			map[string]any{"IPProtocol": "tcp", "ports": []any{"22"}},
		},
	}, http.StatusNotFound)
}

func checkRegressionBackendServiceMissingHealthCheck(t *testing.T) {
	srv, cleanup := regressionServer(t)
	defer cleanup()
	assertCreateStatus(t, srv, testutil.ComputePath(project, "global", "backendServices"), map[string]any{
		"name":         "web-backend",
		"protocol":     "HTTP",
		"healthChecks": []any{"projects/" + project + "/global/healthChecks/missing-hc"},
	}, http.StatusNotFound)
}

func checkRegressionDestroyIdempotencyMissingDelete(t *testing.T) {
	srv, cleanup := regressionServer(t)
	defer cleanup()
	resp, _ := testutil.DoDelete(t, srv, testutil.ComputePath(project, "global", "networks", "already-gone"))
	assertDeleteStatus(t, resp.StatusCode, http.StatusNotFound)
	resp, _ = testutil.DoDelete(t, srv, testutil.SQLPath(project, "instances", "missing-sql", "databases", "app"))
	assertDeleteStatus(t, resp.StatusCode, http.StatusNotFound)
}

func checkRegressionOperationEnvelopeShape(t *testing.T) {
	srv, cleanup := regressionServer(t)
	defer cleanup()
	resp, body := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{"name": "op-net"})
	assertStatusCode(t, resp.StatusCode, http.StatusOK)
	assertFieldEquals(t, body, "kind", "compute#operation")
	assertFieldEquals(t, body, "status", "DONE")
	assertFieldContains(t, body, "targetLink", "/compute/v1/projects/"+project+"/global/networks/op-net")
}

func checkRegressionIAMServiceAccountWrongProject(t *testing.T) {
	srv, cleanup := regressionServer(t)
	defer cleanup()
	assertCreateStatus(t, srv, testutil.IAMPath(project, "serviceAccounts"), map[string]any{
		"accountId": "deploy",
	}, http.StatusOK)
	email := "deploy@" + project + ".iam.gserviceaccount.com"
	resp, _ := testutil.DoGet(t, srv, testutil.IAMPath("other-project", "serviceAccounts", email))
	assertGetStatus(t, resp.StatusCode, http.StatusNotFound)
}

func checkRegressionPubSubAckDeadlineEcho(t *testing.T) {
	srv, cleanup := regressionServer(t)
	defer cleanup()
	base := testutil.IAMPath(project)
	assertPutStatus(t, srv, base+"/topics/ack-topic", map[string]any{}, http.StatusOK)
	assertPutStatus(t, srv, base+"/subscriptions/ack-sub", map[string]any{
		"topic":              "projects/" + project + "/topics/ack-topic",
		"ackDeadlineSeconds": 30,
	}, http.StatusOK)
	_, body := testutil.DoGet(t, srv, base+"/subscriptions/ack-sub")
	assertFieldEquals(t, body, "ackDeadlineSeconds", float64(30))
	resp, body := testutil.DoPatch(t, srv, base+"/subscriptions/ack-sub", map[string]any{
		"subscription": map[string]any{"ackDeadlineSeconds": 60},
		"updateMask":   "ackDeadlineSeconds",
	})
	assertStatusCode(t, resp.StatusCode, http.StatusOK)
	assertFieldEquals(t, body, "ackDeadlineSeconds", float64(60))
}

func regressionServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()
	return testutil.NewTestServer(t)
}

func assertCreateStatus(t *testing.T, srv *httptest.Server, path string, body map[string]any, want int) {
	t.Helper()
	resp, _ := testutil.DoCreate(t, srv, path, body)
	assertStatusCode(t, resp.StatusCode, want)
}

func assertPutStatus(t *testing.T, srv *httptest.Server, path string, body map[string]any, want int) {
	t.Helper()
	resp, _ := testutil.DoPut(t, srv, path, body)
	assertStatusCode(t, resp.StatusCode, want)
}

func assertGetStatus(t *testing.T, got, want int) {
	t.Helper()
	assertStatusCode(t, got, want)
}

func assertDeleteStatus(t *testing.T, got, want int) {
	t.Helper()
	assertStatusCode(t, got, want)
}

func assertStatusCode(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("status: got %d, want %d", got, want)
	}
}

func assertFieldEquals(t *testing.T, body map[string]any, key string, want any) {
	t.Helper()
	if body[key] != want {
		t.Fatalf("%s: got %v, want %v", key, body[key], want)
	}
}

func assertFieldContains(t *testing.T, body map[string]any, key, want string) {
	t.Helper()
	got, _ := body[key].(string)
	if !strings.Contains(got, want) {
		t.Fatalf("%s: got %q, want substring %q", key, got, want)
	}
}

// TestSecretVersionEnableDisablePersists guards the regression where
// :enable / :disable mutated the response body but never wrote the
// new state back, so the next GET still showed the old value.
func TestSecretVersionEnableDisablePersists(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	_, _ = testutil.DoCreate(t, srv, testutil.IAMPath(project, "secrets"), map[string]any{
		"secretId":    "rotation-test",
		"replication": map[string]any{"automatic": map[string]any{}},
	})
	_, _ = testutil.DoCreate(t, srv, testutil.IAMPath(project, "secrets", "rotation-test:addVersion"), map[string]any{
		"payload": map[string]any{"data": "aGVsbG8="},
	})

	resp, _ := testutil.DoCreate(t, srv, testutil.IAMPath(project, "secrets", "rotation-test", "versions", "1:disable"), map[string]any{})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, body := testutil.DoGet(t, srv, testutil.IAMPath(project, "secrets", "rotation-test", "versions", "1"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "DISABLED", body["state"], "disable must persist; subsequent GET still showed old state")

	resp, _ = testutil.DoCreate(t, srv, testutil.IAMPath(project, "secrets", "rotation-test", "versions", "1:enable"), map[string]any{})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, body = testutil.DoGet(t, srv, testutil.IAMPath(project, "secrets", "rotation-test", "versions", "1"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ENABLED", body["state"], "enable must persist; subsequent GET still showed old state")
}

// TestSetLabelsGlobalRejectsUnknownTarget guards the regression where
// /<resource>/<name>/setLabels returned a DONE operation regardless of
// whether the target existed. With validation, a typo'd resource name
// must surface as a 404.
func TestSetLabelsGlobalRejectsUnknownTarget(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "addresses", "missing-ip", "setLabels"), map[string]any{
		"labels":           map[string]string{"env": "prod"},
		"labelFingerprint": "abc",
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"setLabels against a non-existent resource must 404, not silently succeed")
}

// TestSetLabelsGlobalAcceptsExistingTarget covers the happy path so a
// regression that broke setLabels for legitimate compute resources
// would surface here too.
func TestSetLabelsGlobalAcceptsExistingTarget(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	_, _ = testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "addresses"), map[string]any{
		"name": "lb-ip",
	})
	resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "addresses", "lb-ip", "setLabels"), map[string]any{
		"labels":           map[string]string{"env": "prod"},
		"labelFingerprint": "abc",
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestGetDNSChangeReturnsRecordedChange covers the cache-and-look-up
// path: CreateDNSChange persists the change body, GetDNSChange must
// return that same body for any subsequent poll.
func TestGetDNSChangeReturnsRecordedChange(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	_, _ = testutil.DoCreate(t, srv, "/dns/v1/projects/"+project+"/managedZones", map[string]any{
		"name":       "zone1",
		"dnsName":    "zone1.invalid.",
		"visibility": "public",
	})
	resp, body := testutil.DoCreate(t, srv, "/dns/v1/projects/"+project+"/managedZones/zone1/changes", map[string]any{
		"additions": []any{
			map[string]any{
				"name":    "host.zone1.invalid.",
				"type":    "A",
				"ttl":     300,
				"rrdatas": []any{"192.0.2.10"},
			},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	id, _ := body["id"].(string)
	require.NotEmpty(t, id)

	resp, body = testutil.DoGet(t, srv, "/dns/v1/projects/"+project+"/managedZones/zone1/changes/"+id)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, id, body["id"])
	assert.Equal(t, "done", body["status"])
}

// TestGetDNSChange404ForUnknownID guards the change-id lookup
// contract: an arbitrary id that was never recorded must 404,
// not silently fabricate a `done` response. The parent zone is
// seeded first so requireDNSZone doesn't 404 the request before
// it reaches the change-id lookup — that would let this test
// pass for the wrong reason.
func TestGetDNSChange404ForUnknownID(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	mustCreate(t, srv, "/dns/v1/projects/"+project+"/managedZones", map[string]any{
		"name":       "zone1",
		"dnsName":    "zone1.invalid.",
		"visibility": "public",
	})

	resp, _ := testutil.DoGet(t, srv, "/dns/v1/projects/"+project+"/managedZones/zone1/changes/never-existed")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestLBChainUpdateFKValidation pins the FK gates the update-path
// adds for the LB chain. Each subtest seeds a minimal valid LB
// resource, then issues the relevant PATCH/PUT with a missing
// reference and asserts a 400 (or 404) — without the gate, a
// non-existent ref would slip through the create gate that already
// guards the same field.
func TestLBChainUpdateFKValidation(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	// Seed: a minimal valid LB chain we can later patch with bad refs.
	mustCreate(t, srv, testutil.ComputePath(project, "global", "healthChecks"), map[string]any{
		"name":            "good-hc",
		"httpHealthCheck": map[string]any{"port": 80, "requestPath": "/"},
	})
	mustCreate(t, srv, testutil.ComputePath(project, "global", "backendServices"), map[string]any{
		"name":         "good-bs",
		"protocol":     "HTTP",
		"healthChecks": []any{"good-hc"},
	})
	mustCreate(t, srv, testutil.ComputePath(project, "global", "urlMaps"), map[string]any{
		"name":           "good-um",
		"defaultService": "good-bs",
	})
	mustCreate(t, srv, testutil.ComputePath(project, "global", "sslCertificates"), map[string]any{
		"name":        "good-cert",
		"privateKey":  "fake",
		"certificate": "fake",
	})
	mustCreate(t, srv, testutil.ComputePath(project, "global", "targetHttpsProxies"), map[string]any{
		"name":            "good-thp",
		"urlMap":          "good-um",
		"sslCertificates": []any{"good-cert"},
	})

	t.Run("backendService PATCH rejects missing healthCheck", func(t *testing.T) {
		resp, _ := testutil.DoPatch(t, srv, testutil.ComputePath(project, "global", "backendServices", "good-bs"), map[string]any{
			"healthChecks": []any{"missing-hc"},
		})
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("backendService PATCH rejects cross-project healthCheck self-link", func(t *testing.T) {
		resp, _ := testutil.DoPatch(t, srv, testutil.ComputePath(project, "global", "backendServices", "good-bs"), map[string]any{
			"healthChecks": []any{"projects/other/global/healthChecks/good-hc"},
		})
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("urlMap PATCH rejects missing defaultService", func(t *testing.T) {
		resp, _ := testutil.DoPatch(t, srv, testutil.ComputePath(project, "global", "urlMaps", "good-um"), map[string]any{
			"defaultService": "missing-bs",
		})
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("targetHttpsProxy PATCH rejects missing urlMap", func(t *testing.T) {
		resp, _ := testutil.DoPatch(t, srv, testutil.ComputePath(project, "global", "targetHttpsProxies", "good-thp"), map[string]any{
			"urlMap": "missing-um",
		})
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("targetHttpsProxy PATCH rejects missing sslCertificate", func(t *testing.T) {
		resp, _ := testutil.DoPatch(t, srv, testutil.ComputePath(project, "global", "targetHttpsProxies", "good-thp"), map[string]any{
			"sslCertificates": []any{"missing-cert"},
		})
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("forwardingRule create rejects missing target", func(t *testing.T) {
		resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "forwardingRules"), map[string]any{
			"name":      "fr-bad-target",
			"target":    "projects/" + project + "/global/targetHttpsProxies/missing-thp",
			"portRange": "443",
		})
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("forwardingRule create rejects unsupported target collection", func(t *testing.T) {
		resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "forwardingRules"), map[string]any{
			"name":      "fr-unsupported-target",
			"target":    "projects/" + project + "/global/targetTcpProxies/whatever",
			"portRange": "443",
		})
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("forwardingRule create rejects nonexistent IPAddress self-link", func(t *testing.T) {
		resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "forwardingRules"), map[string]any{
			"name":      "fr-bad-ip",
			"target":    "good-thp",
			"IPAddress": "projects/" + project + "/global/addresses/missing-addr",
			"portRange": "443",
		})
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("forwardingRule create rejects nonexistent IPAddress bare name", func(t *testing.T) {
		resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "forwardingRules"), map[string]any{
			"name":      "fr-bad-bare-ip",
			"target":    "good-thp",
			"IPAddress": "missing-addr",
			"portRange": "443",
		})
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("forwardingRule create rejects malformed IPAddress", func(t *testing.T) {
		// Neither a parseable IP nor a recognisable resource path.
		resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "forwardingRules"), map[string]any{
			"name":      "fr-malformed-ip",
			"target":    "good-thp",
			"IPAddress": "not.a.valid.ref/with/extra/segments",
			"portRange": "443",
		})
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("forwardingRule create rejects cross-project IPAddress self-link", func(t *testing.T) {
		resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "forwardingRules"), map[string]any{
			"name":      "fr-xproject-ip",
			"target":    "good-thp",
			"IPAddress": "projects/other/global/addresses/lb-ip",
			"portRange": "443",
		})
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("forwardingRule create accepts literal IP in IPAddress", func(t *testing.T) {
		resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "forwardingRules"), map[string]any{
			"name":      "fr-literal-ip",
			"target":    "good-thp",
			"IPAddress": "203.0.113.42",
			"portRange": "443",
		})
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("forwardingRule create accepts existing reserved IPAddress bare name", func(t *testing.T) {
		mustCreate(t, srv, testutil.ComputePath(project, "global", "addresses"), map[string]any{
			"name": "lb-reserved-ip",
		})
		resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "forwardingRules"), map[string]any{
			"name":      "fr-reserved-ip",
			"target":    "good-thp",
			"IPAddress": "lb-reserved-ip",
			"portRange": "443",
		})
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("forwardingRule create accepts literal IPv6 in IPAddress", func(t *testing.T) {
		resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "forwardingRules"), map[string]any{
			"name":      "fr-literal-ipv6",
			"target":    "good-thp",
			"IPAddress": "2001:db8::1",
			"portRange": "443",
		})
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("forwardingRule create accepts same-project relative IPAddress path", func(t *testing.T) {
		mustCreate(t, srv, testutil.ComputePath(project, "global", "addresses"), map[string]any{
			"name": "lb-rel-ip",
		})
		resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "forwardingRules"), map[string]any{
			"name":      "fr-relative-ip",
			"target":    "good-thp",
			"IPAddress": "projects/" + project + "/global/addresses/lb-rel-ip",
			"portRange": "443",
		})
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("forwardingRule create accepts absolute self-link IPAddress", func(t *testing.T) {
		mustCreate(t, srv, testutil.ComputePath(project, "global", "addresses"), map[string]any{
			"name": "lb-abs-ip",
		})
		resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "forwardingRules"), map[string]any{
			"name":      "fr-abs-ip",
			"target":    "good-thp",
			"IPAddress": "https://www.googleapis.com/compute/v1/projects/" + project + "/global/addresses/lb-abs-ip",
			"portRange": "443",
		})
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

// mustCreate wraps DoCreate with a require on a 200, so test setup
// failures surface locally instead of cascading into mysterious 404s
// in later assertions.
func mustCreate(t *testing.T, srv *httptest.Server, path string, body map[string]any) {
	t.Helper()
	resp, _ := testutil.DoCreate(t, srv, path, body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "setup create failed: POST %s", path)
}

// TestSecretVersionDestroyPreservesRow pins the v1 :destroy contract:
// the version row stays in the table with state=DESTROYED and a
// destroyTime, the payload is cleared, and a subsequent GET still
// returns the version. Hard-deleting it would diverge from the real
// Secret Manager API.
func TestSecretVersionDestroyPreservesRow(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	mustCreate(t, srv, testutil.IAMPath(project, "secrets"), map[string]any{
		"secretId":    "destroy-me",
		"replication": map[string]any{"automatic": map[string]any{}},
	})
	mustCreate(t, srv, testutil.IAMPath(project, "secrets", "destroy-me:addVersion"), map[string]any{
		"payload": map[string]any{"data": "aGVsbG8="},
	})

	resp, _ := testutil.DoCreate(t, srv, testutil.IAMPath(project, "secrets", "destroy-me", "versions", "1:destroy"), map[string]any{})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, body := testutil.DoGet(t, srv, testutil.IAMPath(project, "secrets", "destroy-me", "versions", "1"))
	require.Equal(t, http.StatusOK, resp.StatusCode,
		"destroyed version must still be GETtable, real Secret Manager keeps the row")
	assert.Equal(t, "DESTROYED", body["state"])
	assert.NotEmpty(t, body["destroyTime"])
	_, hasPayload := body["payload"]
	assert.False(t, hasPayload, "destroyed version must not retain its payload")
}

// TestEnableDisableRejectDestroyedVersion pins the terminal-state
// contract: once a Secret Manager version is destroyed, neither
// :enable nor :disable can resurrect it. The real API returns
// 409; we surface that via the repo's ErrConflict mapping.
func TestEnableDisableRejectDestroyedVersion(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	mustCreate(t, srv, testutil.IAMPath(project, "secrets"), map[string]any{
		"secretId":    "terminal",
		"replication": map[string]any{"automatic": map[string]any{}},
	})
	mustCreate(t, srv, testutil.IAMPath(project, "secrets", "terminal:addVersion"), map[string]any{
		"payload": map[string]any{"data": "aGVsbG8="},
	})

	resp, _ := testutil.DoCreate(t, srv, testutil.IAMPath(project, "secrets", "terminal", "versions", "1:destroy"), map[string]any{})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, body := testutil.DoCreate(t, srv, testutil.IAMPath(project, "secrets", "terminal", "versions", "1:enable"), map[string]any{})
	assert.Equal(t, http.StatusConflict, resp.StatusCode,
		"DESTROYED is terminal — :enable must not resurrect the version")
	assertConflictReason(t, body, "conflict")
	assertContainsTerminal(t, body)

	resp, body = testutil.DoCreate(t, srv, testutil.IAMPath(project, "secrets", "terminal", "versions", "1:disable"), map[string]any{})
	assert.Equal(t, http.StatusConflict, resp.StatusCode,
		"DESTROYED is terminal — :disable must not silently rewrite state")
	assertConflictReason(t, body, "conflict")
	assertContainsTerminal(t, body)

	resp, body = testutil.DoGet(t, srv, testutil.IAMPath(project, "secrets", "terminal", "versions", "1"))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "DESTROYED", body["state"], "version must remain DESTROYED after rejected resurrects")
}

// assertConflictReason asserts the GCP-shaped error body has the
// expected `reason` on the first errors[] entry, so we don't just
// check that some 409 came back but pin the actual payload.
func assertConflictReason(t *testing.T, body map[string]any, want string) {
	t.Helper()
	envelope, ok := body["error"].(map[string]any)
	require.True(t, ok, "expected error envelope, got %v", body)
	errs, _ := envelope["errors"].([]any)
	require.Greater(t, len(errs), 0, "expected at least one error entry")
	first, _ := errs[0].(map[string]any)
	require.NotNil(t, first)
	assert.Equal(t, want, first["reason"])
}

// assertContainsTerminal asserts the 409 message describes the
// terminal-state class of conflict (rather than the
// resource-in-use FK class). Pins the reason mapping wired up
// in writeDomainError.
func assertContainsTerminal(t *testing.T, body map[string]any) {
	t.Helper()
	envelope, ok := body["error"].(map[string]any)
	require.True(t, ok)
	msg, _ := envelope["message"].(string)
	assert.Contains(t, msg, "terminal", "expected terminal-state 409 message, got %q", msg)
}

// TestGetDNSChangeIsScopedByZone pins the (project, zone, id)
// keying for cached DNS changes. A poll against managed-zone B
// against a change created in zone A must 404, not silently
// resolve to A's change.
func TestGetDNSChangeIsScopedByZone(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	dnsZonePath := "/dns/v1/projects/" + project + "/managedZones"
	mustCreate(t, srv, dnsZonePath, map[string]any{
		"name":       "zone-a",
		"dnsName":    "a.invalid.",
		"visibility": "public",
	})
	mustCreate(t, srv, dnsZonePath, map[string]any{
		"name":       "zone-b",
		"dnsName":    "b.invalid.",
		"visibility": "public",
	})

	resp, body := testutil.DoCreate(t, srv, dnsZonePath+"/zone-a/changes", map[string]any{
		"additions": []any{
			map[string]any{"name": "host.a.invalid.", "type": "A", "ttl": 300, "rrdatas": []any{"192.0.2.10"}},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	id, _ := body["id"].(string)
	require.NotEmpty(t, id)

	resp, _ = testutil.DoGet(t, srv, dnsZonePath+"/zone-a/changes/"+id)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "owning zone must still resolve")

	resp, _ = testutil.DoGet(t, srv, dnsZonePath+"/zone-b/changes/"+id)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"a different zone must not be able to look up zone-a's change id")
}

// TestNetworkFKRejectsCrossProjectAndMismatch pins two cross-resource
// FK rules added in pass 27:
//   - resolveSameProjectName rejects any self-link that targets a
//     different project even if the trailing name happens to exist
//     locally.
//   - validateInstanceNetworkInterfaces / validateClusterNetwork
//     reject a subnet whose stored parent network doesn't match the
//     requested network (real GCE/GKE rule).
func TestNetworkFKRejectsCrossProjectAndMismatch(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	mustCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{"name": "vpc-a"})
	mustCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{"name": "vpc-b"})
	mustCreate(t, srv, testutil.ComputePath(project, "regions", region, "subnetworks"), map[string]any{
		"name":        "subnet-a",
		"network":     "projects/" + project + "/global/networks/vpc-a",
		"ipCidrRange": "10.0.0.0/24",
	})

	t.Run("compute instance cross-project network rejected", func(t *testing.T) {
		resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "zones", zone, "instances"), map[string]any{
			"name":        "vm-xproject",
			"machineType": "zones/" + zone + "/machineTypes/n1-standard-1",
			"networkInterfaces": []any{
				map[string]any{"network": "projects/other/global/networks/vpc-a"},
			},
		})
		assert.Equal(t, http.StatusNotFound, resp.StatusCode,
			"foreign-project network self-link must 404 even though vpc-a exists locally")
	})

	t.Run("compute instance VPC/subnet mismatch rejected", func(t *testing.T) {
		resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "zones", zone, "instances"), map[string]any{
			"name":        "vm-mismatch",
			"machineType": "zones/" + zone + "/machineTypes/n1-standard-1",
			"networkInterfaces": []any{
				map[string]any{
					"network":    "projects/" + project + "/global/networks/vpc-b",
					"subnetwork": "projects/" + project + "/regions/" + region + "/subnetworks/subnet-a",
				},
			},
		})
		assert.Equal(t, http.StatusNotFound, resp.StatusCode,
			"subnet whose parent VPC is vpc-a must not be accepted under requested network vpc-b")
	})

	t.Run("GKE cluster cross-project network rejected", func(t *testing.T) {
		resp, _ := testutil.DoCreate(t, srv, testutil.ContainerPath(project, location, "clusters"), map[string]any{
			"cluster": map[string]any{
				"name":    "cluster-xproject",
				"network": "projects/other/global/networks/vpc-a",
			},
		})
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("Cloud SQL cross-project privateNetwork rejected", func(t *testing.T) {
		resp, _ := testutil.DoCreate(t, srv, testutil.SQLPath(project, "instances"), map[string]any{
			"name":            "sql-xproject",
			"databaseVersion": "POSTGRES_15",
			"region":          region,
			"settings": map[string]any{
				"tier": "db-f1-micro",
				"ipConfiguration": map[string]any{
					"privateNetwork": "projects/other/global/networks/vpc-a",
				},
			},
		})
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

// TestPubSubSubscriptionRejectsCrossProjectTopic pins the same
// project-scoped FK contract on Pub/Sub subscription create.
func TestPubSubSubscriptionRejectsCrossProjectTopic(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	pubsubBase := testutil.IAMPath(project)
	resp, _ := testutil.DoPut(t, srv, pubsubBase+"/topics/local", map[string]any{})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, _ = testutil.DoPut(t, srv, pubsubBase+"/subscriptions/cross", map[string]any{
		"topic": "projects/other/topics/local",
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"foreign-project topic self-link must 404 even though local topic exists")
}

// TestNetworkFKBareNameAndPatchSync pins two pass-30 fixes:
//   - bare-name subnet refs (e.g. "subnet-a") are now scoped to
//     the instance's zone-derived region or the cluster's
//     location, and a missing/wrong-VPC subnet at that scope is
//     rejected. Pre-fix code skipped the FK lookup when no
//     /regions/<r>/ segment was in the ref.
//   - PATCH that flips `network` on a subnetwork now re-derives
//     `network_name` so the visible ref and the hidden FK key
//     stay consistent (no split-brain).
func TestNetworkFKBareNameAndPatchSync(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	mustCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{"name": "vpc-a"})
	mustCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{"name": "vpc-b"})
	mustCreate(t, srv, testutil.ComputePath(project, "regions", region, "subnetworks"), map[string]any{
		"name":        "subnet-a",
		"network":     "projects/" + project + "/global/networks/vpc-a",
		"ipCidrRange": "10.0.0.0/24",
	})

	t.Run("bare-name subnet rejected when missing in zone-derived region", func(t *testing.T) {
		resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "zones", zone, "instances"), map[string]any{
			"name":        "vm-bare-missing",
			"machineType": "zones/" + zone + "/machineTypes/n1-standard-1",
			"networkInterfaces": []any{
				map[string]any{
					"network":    "projects/" + project + "/global/networks/vpc-a",
					"subnetwork": "missing-subnet",
				},
			},
		})
		assert.Equal(t, http.StatusNotFound, resp.StatusCode,
			"bare-name subnet must be scoped to the zone-derived region; missing-subnet doesn't exist there")
	})

	t.Run("regional GKE cluster accepts bare-name subnet at location", func(t *testing.T) {
		// location is a region for regional clusters;
		// regionFromZone must return it unchanged. The pre-fix code
		// stripped the trailing "-central1" via simple
		// LastIndex("-") and returned "us", so subnet-a wasn't
		// found in region "us" and the cluster create 404'd.
		resp, body := testutil.DoCreate(t, srv, testutil.ContainerPath(project, region, "clusters"), map[string]any{
			"cluster": map[string]any{
				"name":       "regional-cluster",
				"network":    "projects/" + project + "/global/networks/vpc-a",
				"subnetwork": "subnet-a",
			},
		})
		assertOperationDone(t, resp, body)
	})

	t.Run("subnet PATCH that flips network re-derives network_name", func(t *testing.T) {
		// PATCH the subnet's network from vpc-a to vpc-b. After
		// the fix the stored network_name flips too, so a later
		// instance bound to vpc-b + subnet-a is accepted (their
		// stored parent VPC matches).
		resp, _ := testutil.DoPatch(t, srv, testutil.ComputePath(project, "regions", region, "subnetworks", "subnet-a"), map[string]any{
			"network": "projects/" + project + "/global/networks/vpc-b",
		})
		require.Equal(t, http.StatusOK, resp.StatusCode)

		resp, _ = testutil.DoCreate(t, srv, testutil.ComputePath(project, "zones", zone, "instances"), map[string]any{
			"name":        "vm-after-flip",
			"machineType": "zones/" + zone + "/machineTypes/n1-standard-1",
			"networkInterfaces": []any{
				map[string]any{
					"network":    "projects/" + project + "/global/networks/vpc-b",
					"subnetwork": "projects/" + project + "/regions/" + region + "/subnetworks/subnet-a",
				},
			},
		})
		assert.Equal(t, http.StatusOK, resp.StatusCode,
			"after PATCH flipped subnet's network to vpc-b, the VPC/subnet pair check must accept (vpc-b, subnet-a)")
	})
}

// TestNetworkFKRejectsWrongCollectionAndPostMergePatch pins two
// follow-on fixes from pass 28:
//   - resolveSameProjectName now rejects same-project paths whose
//     collection segment doesn't match the expected one (e.g. an
//     `addresses` self-link can't satisfy a `networks` FK).
//   - UpdateCluster validates the *post-merge* state, so a partial
//     PATCH that flips only `subnetwork` can't smuggle in a
//     mismatched VPC/subnet pair.
func TestNetworkFKRejectsWrongCollectionAndPostMergePatch(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	mustCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{"name": "vpc-a"})
	mustCreate(t, srv, testutil.ComputePath(project, "global", "networks"), map[string]any{"name": "vpc-b"})
	mustCreate(t, srv, testutil.ComputePath(project, "global", "addresses"), map[string]any{"name": "vpc-a"})
	mustCreate(t, srv, testutil.ComputePath(project, "regions", region, "subnetworks"), map[string]any{
		"name":        "subnet-a",
		"network":     "projects/" + project + "/global/networks/vpc-a",
		"ipCidrRange": "10.0.0.0/24",
	})
	mustCreate(t, srv, testutil.ComputePath(project, "regions", region, "subnetworks"), map[string]any{
		"name":        "subnet-b",
		"network":     "projects/" + project + "/global/networks/vpc-b",
		"ipCidrRange": "10.0.1.0/24",
	})

	t.Run("addresses path cannot satisfy networks FK", func(t *testing.T) {
		// vpc-a exists as both an address and a network. An
		// instance pointing at the addresses path must NOT
		// pass the networks FK by trailing-name collision.
		resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "zones", zone, "instances"), map[string]any{
			"name":        "vm-wrong-collection",
			"machineType": "zones/" + zone + "/machineTypes/n1-standard-1",
			"networkInterfaces": []any{
				map[string]any{"network": "projects/" + project + "/global/addresses/vpc-a"},
			},
		})
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("relative wrong-collection path also rejected", func(t *testing.T) {
		// Same trap, but expressed as a relative path with no
		// projects/<p>/... prefix. Pre-fix code fell through to
		// parts[len-1] for relative refs and accepted this as
		// "vpc-a" against the networks FK.
		resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "zones", zone, "instances"), map[string]any{
			"name":        "vm-relative-wrong-collection",
			"machineType": "zones/" + zone + "/machineTypes/n1-standard-1",
			"networkInterfaces": []any{
				map[string]any{"network": "global/addresses/vpc-a"},
			},
		})
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("partial PATCH cannot smuggle in mismatched subnet", func(t *testing.T) {
		// Seed a valid cluster bound to vpc-a + subnet-a.
		mustCreate(t, srv, testutil.ContainerPath(project, location, "clusters"), map[string]any{
			"cluster": map[string]any{
				"name":       "patch-cluster",
				"network":    "projects/" + project + "/global/networks/vpc-a",
				"subnetwork": "projects/" + project + "/regions/" + region + "/subnetworks/subnet-a",
			},
		})

		// PATCH that only flips subnetwork to subnet-b (which is
		// in vpc-b, not vpc-a). The pre-fix check would have
		// validated the patch in isolation and accepted it; now
		// the post-merge state is validated and the patch fails.
		resp, _ := testutil.DoPut(t, srv, testutil.ContainerPath(project, location, "clusters", "patch-cluster"), map[string]any{
			"subnetwork": "projects/" + project + "/regions/" + region + "/subnetworks/subnet-b",
		})
		assert.Equal(t, http.StatusNotFound, resp.StatusCode,
			"flipping just subnetwork to a subnet whose parent VPC differs from the cluster's network must be rejected")
	})
}

// TestComputeInstanceRejectsMissingNetwork pins the FK validation
// added on instance create: a networkInterfaces entry referencing
// a nonexistent network or subnetwork must 404, not silently
// persist a dangling instance that real GCE would reject.
func TestComputeInstanceRejectsMissingNetwork(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "zones", zone, "instances"), map[string]any{
		"name":        "bad-vm",
		"machineType": "zones/" + zone + "/machineTypes/n1-standard-1",
		"networkInterfaces": []any{
			map[string]any{
				"network": "projects/" + project + "/global/networks/missing-vpc",
			},
		},
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestGKEClusterRejectsMissingNetwork pins the same FK guard for
// google_container_cluster's network/subnetwork.
func TestGKEClusterRejectsMissingNetwork(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoCreate(t, srv, testutil.ContainerPath(project, location, "clusters"), map[string]any{
		"cluster": map[string]any{
			"name":    "bad-cluster",
			"network": "projects/" + project + "/global/networks/missing-vpc",
		},
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestSQLInstanceRejectsMissingPrivateNetwork pins the FK guard
// on Cloud SQL settings.ipConfiguration.privateNetwork.
func TestSQLInstanceRejectsMissingPrivateNetwork(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoCreate(t, srv, testutil.SQLPath(project, "instances"), map[string]any{
		"name":            "bad-pg",
		"databaseVersion": "POSTGRES_15",
		"region":          region,
		"settings": map[string]any{
			"tier": "db-f1-micro",
			"ipConfiguration": map[string]any{
				"privateNetwork": "projects/" + project + "/global/networks/missing-vpc",
			},
		},
	})
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestPubSubTopicDeleteOrphansSubscriptionTombstone pins the
// real-Pub/Sub orphan contract: deleting a topic leaves the
// surviving subscriptions in place, but their `topic` field is
// rewritten to "_deleted-topic_" (and topic_name is cleared
// internally) so a topic recreated with the same name does NOT
// silently re-bind the old subscriptions.
func TestPubSubTopicDeleteOrphansSubscriptionTombstone(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	pubsubBase := testutil.IAMPath(project)

	resp, _ := testutil.DoPut(t, srv, pubsubBase+"/topics/orig", map[string]any{})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp, _ = testutil.DoPut(t, srv, pubsubBase+"/subscriptions/sub", map[string]any{
		"topic": "projects/" + project + "/topics/orig",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, _ = testutil.DoDelete(t, srv, pubsubBase+"/topics/orig")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, body := testutil.DoGet(t, srv, pubsubBase+"/subscriptions/sub")
	require.Equal(t, http.StatusOK, resp.StatusCode, "subscription must survive topic delete")
	assert.Equal(t, "_deleted-topic_", body["topic"],
		"topic field must flip to _deleted-topic_ tombstone, real Pub/Sub semantics")

	// Recreate the topic with the same name. The orphaned
	// subscription must NOT silently re-bind to the new topic.
	resp, _ = testutil.DoPut(t, srv, pubsubBase+"/topics/orig", map[string]any{})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, body = testutil.DoGet(t, srv, pubsubBase+"/subscriptions/sub")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "_deleted-topic_", body["topic"],
		"orphaned subscription must remain bound to the tombstone, not the recreated topic")
}

// TestPubSubSubscriptionTopicIsImmutable pins the contract that
// PATCH on a pubsub subscription must not retarget the parent
// topic. A pre-fix bug let a patched `topic` self-link land in
// the JSON body while the stored binding still pointed at the
// original; GET would then report split-brain state.
func TestPubSubSubscriptionTopicIsImmutable(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	pubsubBase := testutil.IAMPath(project)

	// Two topics: original parent, and a decoy a malicious patch
	// would try to retarget to.
	resp, _ := testutil.DoPut(t, srv, pubsubBase+"/topics/orig", map[string]any{})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp, _ = testutil.DoPut(t, srv, pubsubBase+"/topics/decoy", map[string]any{})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, _ = testutil.DoPut(t, srv, pubsubBase+"/subscriptions/sub", map[string]any{
		"topic":              "projects/" + project + "/topics/orig",
		"ackDeadlineSeconds": 30,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// PATCH that tries to flip both ack deadline AND topic. The
	// ack deadline is mutable (and should land); the topic must
	// be ignored.
	resp, _ = testutil.DoPatch(t, srv, pubsubBase+"/subscriptions/sub", map[string]any{
		"subscription": map[string]any{
			"topic":              "projects/" + project + "/topics/decoy",
			"ackDeadlineSeconds": 60,
		},
		"updateMask": "ackDeadlineSeconds,topic",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp, body := testutil.DoGet(t, srv, pubsubBase+"/subscriptions/sub")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "projects/"+project+"/topics/orig", body["topic"],
		"topic must remain bound to the original parent — Pub/Sub doesn't allow retargeting on PATCH")
	assert.EqualValues(t, 60, body["ackDeadlineSeconds"],
		"ack deadline (a mutable field) should still take effect")
}

// TestResetClearsDNSChangeCache pins that /mock/reset wipes the
// in-memory DNS change history alongside the repo. Without this,
// a stale change id from before the reset would still resolve and
// diverge from the cleared rrset state.
func TestResetClearsDNSChangeCache(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	dnsZonePath := "/dns/v1/projects/" + project + "/managedZones"
	mustCreate(t, srv, dnsZonePath, map[string]any{
		"name":       "reset-zone",
		"dnsName":    "r.invalid.",
		"visibility": "public",
	})
	resp, body := testutil.DoCreate(t, srv, dnsZonePath+"/reset-zone/changes", map[string]any{
		"additions": []any{
			map[string]any{"name": "host.r.invalid.", "type": "A", "ttl": 300, "rrdatas": []any{"192.0.2.10"}},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	id, _ := body["id"].(string)
	require.NotEmpty(t, id)

	resetResp, _ := testutil.DoCreate(t, srv, "/mock/reset", nil)
	require.Equal(t, http.StatusOK, resetResp.StatusCode)

	resp, _ = testutil.DoGet(t, srv, dnsZonePath+"/reset-zone/changes/"+id)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"reset must clear the DNS change cache")
}

// TestDNSChangeRollbackOrder pins the rollback order for the
// (delete A, add replacement A, fail mid-additions) interleaving.
// Earlier code rolled back deletions before additions, so the
// re-create of A collided with the freshly-added A and silently
// no-op'd; the subsequent addition-cleanup deleted A, leaving
// neither rrset present. The fix undoes additions FIRST, then
// re-creates the deletions. Without that ordering this test
// regresses to "the original A is gone after rollback".
func TestDNSChangeRollbackOrder(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	dnsZonePath := "/dns/v1/projects/" + project + "/managedZones"
	_, _ = testutil.DoCreate(t, srv, dnsZonePath, map[string]any{
		"name":       "rollback-zone",
		"dnsName":    "rollback.invalid.",
		"visibility": "public",
	})

	// Seed: existing rrset A pointing at 192.0.2.10.
	originalRR := map[string]any{
		"name":    "host.rollback.invalid.",
		"type":    "A",
		"ttl":     300,
		"rrdatas": []any{"192.0.2.10"},
	}
	_, _ = testutil.DoCreate(t, srv, dnsZonePath+"/rollback-zone/changes", map[string]any{
		"additions": []any{originalRR},
	})

	// Now submit a change that:
	//   - deletes the existing A, AND
	//   - adds replacement A pointing at 192.0.2.20, AND
	//   - tries to add a malformed rrset with no `type`, which forces
	//     a partial-failure mid-additions and triggers rollback.
	resp, _ := testutil.DoCreate(t, srv, dnsZonePath+"/rollback-zone/changes", map[string]any{
		"deletions": []any{originalRR},
		"additions": []any{
			map[string]any{
				"name":    "host.rollback.invalid.",
				"type":    "A",
				"ttl":     300,
				"rrdatas": []any{"192.0.2.20"},
			},
			// missing `type` → CreateDNSChange validates additions up
			// front and rejects with 400 before any state change.
			// To force a mid-additions failure we need an addition
			// that the up-front validator accepts but the repository
			// rejects. A duplicate of the replacement we already
			// added does that — fakegcp's rrset table has a unique
			// (project, zone, name, type) constraint.
			map[string]any{
				"name":    "host.rollback.invalid.",
				"type":    "A",
				"ttl":     300,
				"rrdatas": []any{"192.0.2.30"},
			},
		},
	})
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("expected change to fail mid-additions, got 200")
	}

	// After rollback, the original rrset must still resolve.
	resp, body := testutil.DoGet(t, srv, dnsZonePath+"/rollback-zone/rrsets/host.rollback.invalid./A")
	require.Equal(t, http.StatusOK, resp.StatusCode,
		"after rollback the original rrset must still exist")
	rrdatas, _ := body["rrdatas"].([]any)
	require.Equal(t, 1, len(rrdatas), "expected exactly the original rrdatas")
	assert.Equal(t, "192.0.2.10", rrdatas[0],
		"rollback re-created the original rrdata; replacement value would mean wrong rollback order")
}

// TestBackendServiceFKValidatesHealthCheckShape guards the new
// path-shape FK: a self-link pointing at a different project or a
// different collection must be rejected even when a same-named
// local resource exists, otherwise the FK check is a fig leaf.
func TestBackendServiceFKValidatesHealthCheckShape(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	// Set up a local health check the bogus self-links will share a
	// trailing name with.
	_, _ = testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "healthChecks"), map[string]any{
		"name": "test-hc",
		"httpHealthCheck": map[string]any{
			"port":        80,
			"requestPath": "/",
		},
	})

	cases := []struct {
		name       string
		ref        string
		wantStatus int
	}{
		{
			name:       "cross-project self-link rejected",
			ref:        "projects/other-project/global/healthChecks/test-hc",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "wrong-collection self-link rejected",
			ref:        "projects/" + project + "/global/backendServices/test-hc",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "bare name accepted (resolves locally)",
			ref:        "test-hc",
			wantStatus: http.StatusOK,
		},
		{
			name:       "same-project self-link accepted",
			ref:        "projects/" + project + "/global/healthChecks/test-hc",
			wantStatus: http.StatusOK,
		},
		{
			name:       "compute/v1 prefixed same-project self-link accepted",
			ref:        "compute/v1/projects/" + project + "/global/healthChecks/test-hc",
			wantStatus: http.StatusOK,
		},
		{
			name:       "absolute URL same-project self-link accepted",
			ref:        "https://www.googleapis.com/compute/v1/projects/" + project + "/global/healthChecks/test-hc",
			wantStatus: http.StatusOK,
		},
		{
			name:       "absolute URL cross-project self-link rejected",
			ref:        "https://www.googleapis.com/compute/v1/projects/other-project/global/healthChecks/test-hc",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "absolute URL wrong-collection self-link rejected",
			ref:        "https://www.googleapis.com/compute/v1/projects/" + project + "/global/backendServices/test-hc",
			wantStatus: http.StatusBadRequest,
		},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beName := "be-" + string(rune('a'+i))
			resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "backendServices"), map[string]any{
				"name":         beName,
				"protocol":     "HTTP",
				"healthChecks": []any{tc.ref},
			})
			assert.Equal(t, tc.wantStatus, resp.StatusCode)
		})
	}
}
