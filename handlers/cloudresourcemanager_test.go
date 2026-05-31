package handlers_test

import (
	"net/http"
	"testing"

	"github.com/redscaresu/fakegcp/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetProjectReturnsActiveProject pins the Ticket C closeout.
// terraform-provider-google v5's getProject helper preflights many
// resources by calling GET /v1/projects/{project} on
// cloudresourcemanager.googleapis.com. Pre-fix, the route 501'd and
// the SDK surfaced a misleading 401 ACCESS_TOKEN_TYPE_UNSUPPORTED
// error that looked like a real-cloud escape. Now it returns a
// synthetic ACTIVE project so the preflight succeeds.
func TestGetProjectReturnsActiveProject(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, body := testutil.DoGet(t, srv, "/v1/projects/"+project)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, project, body["projectId"])
	assert.Equal(t, "ACTIVE", body["lifecycleState"])
	assert.Equal(t, project, body["name"])
	assert.NotEmpty(t, body["projectNumber"])
}

// TestGetProjectRequiresAuth confirms the route runs under the same
// requireBearerToken middleware as the rest of the GCP surface — no
// special carve-out for the preflight helper.
func TestGetProjectRequiresAuth(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := doNoAuthJSON(t, srv.URL, http.MethodGet, "/v1/projects/"+project, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestGetProjectV3ReturnsActiveProject pins the Ticket D-2 closeout —
// the resource-manager v3 GetProject route. terraform-provider-google
// v5's newer code paths (notably google_service_networking_connection's
// getProject preflight) use v3, NOT v1. With only v1 implemented, those
// calls escaped to real cloud and surfaced as misleading 401 errors.
// Response shape differs from v1: "name" is "projects/{id}", state field
// is "state" not "lifecycleState", parent is a string not an object.
func TestGetProjectV3ReturnsActiveProject(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, body := testutil.DoGet(t, srv, "/v3/projects/"+project)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "projects/"+project, body["name"])
	assert.Equal(t, project, body["projectId"])
	assert.Equal(t, "ACTIVE", body["state"])
	assert.Contains(t, body["parent"], "organizations/")
}

// TestGetProjectV3RequiresAuth confirms v3 also runs under
// requireBearerToken — symmetric with v1.
func TestGetProjectV3RequiresAuth(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, _ := doNoAuthJSON(t, srv.URL, http.MethodGet, "/v3/projects/"+project, nil)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}
