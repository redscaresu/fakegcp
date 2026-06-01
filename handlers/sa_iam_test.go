package handlers_test

import (
	"net/http"
	"testing"

	"github.com/redscaresu/fakegcp/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSAIamPolicyRoundTrip pins the SA-level IAM round-trip. The
// project-level google_project_iam_member escapes to real cloud
// (BatchingConfig bypass), so the GCP prompt recommends
// google_service_account_iam_member as the substitute. That resource
// hits these routes — Get must echo what Set stored, or the v5
// provider raises "Provider produced inconsistent result after apply".
func TestSAIamPolicyRoundTrip(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	// Set a binding on a synthetic SA email.
	setPath := "/v1/projects/" + project + "/serviceAccounts/cicd@example.iam.gserviceaccount.com:setIamPolicy"
	want := map[string]any{
		"policy": map[string]any{
			"version": 1,
			"bindings": []any{
				map[string]any{
					"role":    "roles/iam.serviceAccountUser",
					"members": []any{"user:dev@example.com"},
				},
			},
		},
	}
	resp, setBody := testutil.DoCreate(t, srv, setPath, want)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	bindings, ok := setBody["bindings"].([]any)
	require.True(t, ok)
	require.Len(t, bindings, 1)
	_, hasAudit := setBody["auditConfigs"]
	assert.True(t, hasAudit, "SA SetIamPolicy must include auditConfigs for provider deref safety")

	// Get must echo the binding back.
	getPath := "/v1/projects/" + project + "/serviceAccounts/cicd@example.iam.gserviceaccount.com:getIamPolicy"
	resp, getBody := testutil.DoCreate(t, srv, getPath, map[string]any{})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	getBindings, ok := getBody["bindings"].([]any)
	require.True(t, ok)
	require.Len(t, getBindings, 1)
	assert.Equal(t, "roles/iam.serviceAccountUser", getBindings[0].(map[string]any)["role"])

	// Get on a fresh SA returns empty policy + auditConfigs.
	freshGet := "/v1/projects/" + project + "/serviceAccounts/fresh@example.iam.gserviceaccount.com:getIamPolicy"
	_, freshBody := testutil.DoCreate(t, srv, freshGet, map[string]any{})
	freshBindings, _ := freshBody["bindings"].([]any)
	assert.Empty(t, freshBindings)
	_, ok = freshBody["auditConfigs"]
	assert.True(t, ok)
}

// TestSAIamPolicyResetClearsStore confirms /mock/reset wipes the
// in-memory SA IAM cache — bindings from scenario N don't leak into
// scenario N+1.
func TestSAIamPolicyResetClearsStore(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	setPath := "/v1/projects/" + project + "/serviceAccounts/reset@example.iam.gserviceaccount.com:setIamPolicy"
	_, _ = testutil.DoCreate(t, srv, setPath, map[string]any{
		"policy": map[string]any{
			"bindings": []any{
				map[string]any{"role": "roles/iam.serviceAccountUser", "members": []any{"user:a@example.com"}},
			},
		},
	})

	testutil.ResetState(t, srv)

	getPath := "/v1/projects/" + project + "/serviceAccounts/reset@example.iam.gserviceaccount.com:getIamPolicy"
	_, body := testutil.DoCreate(t, srv, getPath, map[string]any{})
	bindings, _ := body["bindings"].([]any)
	assert.Empty(t, bindings, "reset must clear SA IAM bindings")
}
