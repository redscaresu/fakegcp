package handlers_test

import (
	"net/http"
	"testing"

	"github.com/redscaresu/fakegcp/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKMSCryptoKeyVersionsListReturnsPrimary pins the N12 cryptoKeyVersions
// LIST handler. terraform-provider-google's destroy flow lists versions
// to enumerate which ones to schedule for destruction; without this
// route the destroy 501s and CMEK-required scenarios stay stuck.
// Surfaced in gcp-cloud-sql + gcp-storage 2026-06-02 sweeps.
func TestKMSCryptoKeyVersionsListReturnsPrimary(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	path := "/v1/projects/" + project + "/locations/us-central1/keyRings/test-ring/cryptoKeys/test-key/cryptoKeyVersions"
	resp, body := testutil.DoGet(t, srv, path)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	versions, ok := body["cryptoKeyVersions"].([]any)
	require.True(t, ok, "cryptoKeyVersions should be array, got %T", body["cryptoKeyVersions"])
	require.Len(t, versions, 1)

	v := versions[0].(map[string]any)
	assert.Equal(t, "ENABLED", v["state"])
	assert.Equal(t, "GOOGLE_SYMMETRIC_ENCRYPTION", v["algorithm"])
	assert.Equal(t, "SOFTWARE", v["protectionLevel"])
	assert.Contains(t, v["name"], "/cryptoKeyVersions/1")
}

// TestKMSCryptoKeyVersionsListBarePrefix confirms the dual-prefix
// registration covers both `/v1/projects/...` (v5 lib-client) and bare
// `/projects/...` (template-based BasePath). Matches the existing KMS
// dual-prefix discipline.
func TestKMSCryptoKeyVersionsListBarePrefix(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	path := "/projects/" + project + "/locations/us-central1/keyRings/test-ring/cryptoKeys/test-key/cryptoKeyVersions"
	resp, _ := testutil.DoGet(t, srv, path)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestKMSCryptoKeyVersionDestroySchedulesDeletion pins the :destroy
// endpoint. Real Cloud KMS flips state to DESTROY_SCHEDULED with a
// 24h grace period; we model the immediate state transition since
// destroy timing isn't observed by the provider.
func TestKMSCryptoKeyVersionDestroySchedulesDeletion(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	path := "/v1/projects/" + project + "/locations/us-central1/keyRings/test-ring/cryptoKeys/test-key/cryptoKeyVersions/1:destroy"
	resp, body := testutil.DoCreate(t, srv, path, map[string]any{})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "DESTROY_SCHEDULED", body["state"])
	assert.NotEmpty(t, body["destroyTime"])
}

// TestKMSCryptoKeyVersionGet pins the per-version GET. The destroy
// flow may read individual versions before scheduling :destroy.
func TestKMSCryptoKeyVersionGet(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	path := "/v1/projects/" + project + "/locations/us-central1/keyRings/test-ring/cryptoKeys/test-key/cryptoKeyVersions/1"
	resp, body := testutil.DoGet(t, srv, path)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ENABLED", body["state"])
	assert.Contains(t, body["name"], "/cryptoKeyVersions/1")
}

// TestKMSCryptoKeyIamPolicyRoundTrip pins the N12 IAM round-trip fix.
// Before: SetIamPolicy returned an empty {bindings:[]} regardless of
// what was sent, so the subsequent Read returned empty and the
// provider raised "Provider produced inconsistent result after apply"
// for google_kms_crypto_key_iam_member. Now Set stores + Get returns.
func TestKMSCryptoKeyIamPolicyRoundTrip(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	setPath := "/v1/projects/" + project + "/locations/us-central1/keyRings/test-ring/cryptoKeys/test-key:setIamPolicy"
	want := map[string]any{
		"policy": map[string]any{
			"version": 1,
			"bindings": []any{
				map[string]any{
					"role":    "roles/cloudkms.cryptoKeyEncrypterDecrypter",
					"members": []any{"serviceAccount:sql-sa@" + project + ".iam.gserviceaccount.com"},
				},
			},
		},
	}
	resp, setBody := testutil.DoCreate(t, srv, setPath, want)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	bindings, ok := setBody["bindings"].([]any)
	require.True(t, ok, "Set response should include bindings")
	require.Len(t, bindings, 1)
	assert.Equal(t, "roles/cloudkms.cryptoKeyEncrypterDecrypter", bindings[0].(map[string]any)["role"])

	getPath := "/v1/projects/" + project + "/locations/us-central1/keyRings/test-ring/cryptoKeys/test-key:getIamPolicy"
	resp, getBody := testutil.DoCreate(t, srv, getPath, map[string]any{})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	getBindings, ok := getBody["bindings"].([]any)
	require.True(t, ok)
	require.Len(t, getBindings, 1, "Get must echo the binding set by Set")
	assert.Equal(t, "roles/cloudkms.cryptoKeyEncrypterDecrypter", getBindings[0].(map[string]any)["role"])
}

// TestKMSKeyRingIamPolicyRoundTrip is the key-ring-level twin of the
// crypto-key test. Same bug, same fix — both paths share KMSSetIamPolicy.
func TestKMSKeyRingIamPolicyRoundTrip(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	setPath := "/v1/projects/" + project + "/locations/us-central1/keyRings/ring-iam:setIamPolicy"
	resp, _ := testutil.DoCreate(t, srv, setPath, map[string]any{
		"policy": map[string]any{
			"bindings": []any{
				map[string]any{"role": "roles/cloudkms.admin", "members": []any{"user:admin@example.com"}},
			},
		},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	getPath := "/v1/projects/" + project + "/locations/us-central1/keyRings/ring-iam:getIamPolicy"
	resp, body := testutil.DoCreate(t, srv, getPath, map[string]any{})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	bindings, ok := body["bindings"].([]any)
	require.True(t, ok)
	require.Len(t, bindings, 1)
	assert.Equal(t, "roles/cloudkms.admin", bindings[0].(map[string]any)["role"])
}

// TestKMSCryptoKeyIamPolicyResetClearsStore confirms /mock/reset wipes
// the in-memory IAM cache. Without this, bindings from one scenario
// would leak into the next.
func TestKMSCryptoKeyIamPolicyResetClearsStore(t *testing.T) {
	srv, cleanup := testutil.NewTestServer(t)
	defer cleanup()

	setPath := "/v1/projects/" + project + "/locations/us-central1/keyRings/reset-ring/cryptoKeys/reset-key:setIamPolicy"
	_, _ = testutil.DoCreate(t, srv, setPath, map[string]any{
		"policy": map[string]any{
			"bindings": []any{
				map[string]any{"role": "roles/cloudkms.cryptoKeyEncrypterDecrypter", "members": []any{"user:a@example.com"}},
			},
		},
	})

	testutil.ResetState(t, srv)

	getPath := "/v1/projects/" + project + "/locations/us-central1/keyRings/reset-ring/cryptoKeys/reset-key:getIamPolicy"
	_, body := testutil.DoCreate(t, srv, getPath, map[string]any{})
	bindings, _ := body["bindings"].([]any)
	assert.Empty(t, bindings, "reset must clear KMS IAM bindings")
}
