package handlers_test

import (
	"net/http"
	"testing"

	"github.com/redscaresu/fakegcp/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIAMPolicyResponseIncludesAuditConfigs pins the audit-configs
// field added by N12. cloudresourcemanager.Policy in the v5 SDK
// includes AuditConfigs; some provider code paths read it and
// nil-deref when the field is missing. Returning empty array
// matches "no audit configured" semantics and unblocks the deref.
func TestIAMPolicyResponseIncludesAuditConfigs(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	_, body := testutil.DoCreate(t, srv, "/v1/projects/"+project+":getIamPolicy", map[string]any{})
	_, ok := body["auditConfigs"]
	assert.True(t, ok, "GetIamPolicy must include auditConfigs (even if empty) for v5 provider deref safety")

	resp, setBody := testutil.DoCreate(t, srv, "/v1/projects/"+project+":setIamPolicy", map[string]any{
		"policy": map[string]any{
			"bindings": []any{
				map[string]any{"role": "roles/viewer", "members": []any{"user:a@example.com"}},
			},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_, ok = setBody["auditConfigs"]
	assert.True(t, ok, "SetIamPolicy response must also include auditConfigs")
}

// TestServiceAccountCreateIncludesOauthClientIdAndDisabled pins the
// SA response shape gap. v5 google_service_account reader derefs
// .Oauth2ClientId + .Disabled; without these fields the provider's
// Apply panics with "Plugin did not respond" when the SA is used as
// a target for google_project_iam_member.
func TestServiceAccountCreateIncludesOauthClientIdAndDisabled(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	resp, body := testutil.DoCreate(t, srv, "/v1/projects/"+project+"/serviceAccounts",
		map[string]any{
			"accountId": "test-sa",
			"serviceAccount": map[string]any{
				"displayName": "Test SA",
			},
		})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	assert.NotEmpty(t, body["oauth2ClientId"], "SA response must include oauth2ClientId")
	disabled, hasDisabled := body["disabled"]
	assert.True(t, hasDisabled, "SA response must include disabled flag")
	assert.Equal(t, false, disabled)
	assert.NotEmpty(t, body["etag"], "SA response must include etag")
}
