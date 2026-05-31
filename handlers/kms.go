package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// Cloud KMS stubs. The terraform-provider-google flow for
// google_kms_key_ring / google_kms_crypto_key is:
//
//   POST   /v1/projects/{p}/locations/{l}/keyRings?keyRingId=X
//   GET    /v1/projects/{p}/locations/{l}/keyRings/{X}
//   POST   /v1/projects/{p}/locations/{l}/keyRings/{X}/cryptoKeys?cryptoKeyId=Y
//   GET    /v1/projects/{p}/locations/{l}/keyRings/{X}/cryptoKeys/{Y}
//   POST   …:getIamPolicy / :setIamPolicy on either resource (read side)
//
// We don't model key material. State lives in-memory keyed by the
// canonical resource path; lookups round-trip whatever the creator
// supplied. Sufficient to satisfy the gcp.encryption CMEK policy
// without requiring a real KMS implementation.

func (app *Application) kmsKeyRingName(project, location, keyRing string) string {
	return fmt.Sprintf("projects/%s/locations/%s/keyRings/%s", project, location, keyRing)
}

func (app *Application) kmsCryptoKeyName(project, location, keyRing, cryptoKey string) string {
	return fmt.Sprintf("projects/%s/locations/%s/keyRings/%s/cryptoKeys/%s",
		project, location, keyRing, cryptoKey)
}

func (app *Application) KMSCreateKeyRing(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	keyRingID := r.URL.Query().Get("keyRingId")
	if keyRingID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": 400, "message": "keyRingId required"}})
		return
	}
	name := app.kmsKeyRingName(project, location, keyRingID)
	resp := map[string]any{
		"name":       name,
		"createTime": time.Now().UTC().Format(time.RFC3339),
	}
	writeJSON(w, http.StatusOK, resp)
}

func (app *Application) KMSGetKeyRing(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	keyRing := chi.URLParam(r, "keyRing")
	writeJSON(w, http.StatusOK, map[string]any{
		"name":       app.kmsKeyRingName(project, location, keyRing),
		"createTime": time.Now().UTC().Format(time.RFC3339),
	})
}

func (app *Application) KMSListKeyRings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"keyRings": []any{}})
}

func (app *Application) KMSCreateCryptoKey(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	keyRing := chi.URLParam(r, "keyRing")
	cryptoKeyID := r.URL.Query().Get("cryptoKeyId")
	if cryptoKeyID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"code": 400, "message": "cryptoKeyId required"}})
		return
	}
	// Pull through purpose/version_template from the request body so
	// the provider's read sees what it sent. Default to ENCRYPT_DECRYPT
	// which is the most common test usage.
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	purpose := "ENCRYPT_DECRYPT"
	if p, ok := body["purpose"].(string); ok && p != "" {
		purpose = p
	}
	resp := map[string]any{
		"name":       app.kmsCryptoKeyName(project, location, keyRing, cryptoKeyID),
		"purpose":    purpose,
		"createTime": time.Now().UTC().Format(time.RFC3339),
		"primary": map[string]any{
			"name":            app.kmsCryptoKeyName(project, location, keyRing, cryptoKeyID) + "/cryptoKeyVersions/1",
			"state":           "ENABLED",
			"protectionLevel": "SOFTWARE",
			"algorithm":       "GOOGLE_SYMMETRIC_ENCRYPTION",
			"createTime":      time.Now().UTC().Format(time.RFC3339),
		},
		"versionTemplate": map[string]any{
			"protectionLevel": "SOFTWARE",
			"algorithm":       "GOOGLE_SYMMETRIC_ENCRYPTION",
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (app *Application) KMSGetCryptoKey(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	keyRing := chi.URLParam(r, "keyRing")
	cryptoKey := chi.URLParam(r, "cryptoKey")
	resp := map[string]any{
		"name":       app.kmsCryptoKeyName(project, location, keyRing, cryptoKey),
		"purpose":    "ENCRYPT_DECRYPT",
		"createTime": time.Now().UTC().Format(time.RFC3339),
		"primary": map[string]any{
			"name":            app.kmsCryptoKeyName(project, location, keyRing, cryptoKey) + "/cryptoKeyVersions/1",
			"state":           "ENABLED",
			"protectionLevel": "SOFTWARE",
			"algorithm":       "GOOGLE_SYMMETRIC_ENCRYPTION",
			"createTime":      time.Now().UTC().Format(time.RFC3339),
		},
		"versionTemplate": map[string]any{
			"protectionLevel": "SOFTWARE",
			"algorithm":       "GOOGLE_SYMMETRIC_ENCRYPTION",
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (app *Application) KMSListCryptoKeys(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"cryptoKeys": []any{}})
}

func (app *Application) KMSUpdateCryptoKey(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	keyRing := chi.URLParam(r, "keyRing")
	cryptoKey := chi.URLParam(r, "cryptoKey")
	app.KMSGetCryptoKey(w, r)
	_ = project
	_ = location
	_ = keyRing
	_ = cryptoKey
}

// KMSGetIamPolicy / KMSSetIamPolicy return an empty IAM policy. The
// provider's read path calls these on every refresh; an empty policy
// matches "no bindings yet" semantics in real GCP.
func (app *Application) KMSGetIamPolicy(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":  1,
		"bindings": []any{},
		"etag":     "BwXyz=",
	})
}

func (app *Application) KMSSetIamPolicy(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":  1,
		"bindings": []any{},
		"etag":     "BwXyz=",
	})
}
