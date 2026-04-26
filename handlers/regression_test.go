package handlers_test

import (
	"net/http"
	"testing"

	"github.com/redscaresu/fakegcp/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		"name":     "zone1",
		"dnsName":  "zone1.invalid.",
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

// TestGetDNSChange404ForUnknownID guards the change-id lookup contract:
// an arbitrary id that was never recorded must 404, not silently
// fabricate a `done` response.
func TestGetDNSChange404ForUnknownID(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := testutil.DoGet(t, srv, "/dns/v1/projects/"+project+"/managedZones/zone1/changes/never-existed")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
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
		name          string
		ref           string
		wantStatus    int
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
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beName := "be-" + string(rune('a'+i))
			resp, _ := testutil.DoCreate(t, srv, testutil.ComputePath(project, "global", "backendServices"), map[string]any{
				"name":          beName,
				"protocol":      "HTTP",
				"healthChecks":  []any{tc.ref},
			})
			assert.Equal(t, tc.wantStatus, resp.StatusCode)
		})
	}
}
