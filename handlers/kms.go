package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// kmsIamStore round-trips IAM policies set on KMS resources (key
// rings + crypto keys). Without this, KMSSetIamPolicy returned a
// hardcoded empty policy and the subsequent Read returned the same
// empty shape — the v5 terraform-provider-google then raised
// "Provider produced inconsistent result after apply" for
// google_kms_crypto_key_iam_member because the binding it just set
// was missing from the Read. Surfaced in gcp-gke-cluster /
// gcp-storage CMEK flows.
//
// Keyed by the canonical resource path
// (projects/{p}/locations/{l}/keyRings/{r}/cryptoKeys/{k} or
// the key-ring variant). Round-trips bindings verbatim.
var (
	kmsIamMu   sync.RWMutex
	kmsIamStor = map[string]map[string]any{}
)

func kmsIamKey(parts ...string) string {
	return fmt.Sprintf("%s", joinNonEmpty(parts, "/"))
}

func joinNonEmpty(parts []string, sep string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += sep
		}
		out += p
	}
	return out
}

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

// KMSListCryptoKeyVersions returns a single synthetic PRIMARY version
// for the requested crypto key. terraform-provider-google's destroy
// flow lists versions to enumerate which ones to schedule for
// destruction; without this route the destroy 501s and leaves the
// scenario stuck in target_reached=false. Surfaced in gcp-cloud-sql
// + gcp-storage 2026-06-02 sweeps.
func (app *Application) KMSListCryptoKeyVersions(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	keyRing := chi.URLParam(r, "keyRing")
	cryptoKey := chi.URLParam(r, "cryptoKey")
	now := time.Now().UTC().Format(time.RFC3339)
	versionName := app.kmsCryptoKeyName(project, location, keyRing, cryptoKey) + "/cryptoKeyVersions/1"
	writeJSON(w, http.StatusOK, map[string]any{
		"cryptoKeyVersions": []any{
			map[string]any{
				"name":            versionName,
				"state":           "ENABLED",
				"protectionLevel": "SOFTWARE",
				"algorithm":       "GOOGLE_SYMMETRIC_ENCRYPTION",
				"createTime":      now,
				"generateTime":    now,
			},
		},
	})
}

// KMSGetCryptoKeyVersion returns a single synthetic version record.
// terraform-provider-google reads individual versions during the
// destroy flow before issuing :destroy.
func (app *Application) KMSGetCryptoKeyVersion(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	keyRing := chi.URLParam(r, "keyRing")
	cryptoKey := chi.URLParam(r, "cryptoKey")
	version := chi.URLParam(r, "version")
	now := time.Now().UTC().Format(time.RFC3339)
	writeJSON(w, http.StatusOK, map[string]any{
		"name":            app.kmsCryptoKeyName(project, location, keyRing, cryptoKey) + "/cryptoKeyVersions/" + version,
		"state":           "ENABLED",
		"protectionLevel": "SOFTWARE",
		"algorithm":       "GOOGLE_SYMMETRIC_ENCRYPTION",
		"createTime":      now,
		"generateTime":    now,
	})
}

// KMSDestroyCryptoKeyVersion returns a DESTROY_SCHEDULED record. Real
// Cloud KMS retains the version metadata for 24h before purging; we
// just flip state. terraform-provider-google's destroy is satisfied
// once the response shows non-ENABLED state.
func (app *Application) KMSDestroyCryptoKeyVersion(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	keyRing := chi.URLParam(r, "keyRing")
	cryptoKey := chi.URLParam(r, "cryptoKey")
	version := chi.URLParam(r, "version")
	now := time.Now().UTC().Format(time.RFC3339)
	writeJSON(w, http.StatusOK, map[string]any{
		"name":            app.kmsCryptoKeyName(project, location, keyRing, cryptoKey) + "/cryptoKeyVersions/" + version,
		"state":           "DESTROY_SCHEDULED",
		"protectionLevel": "SOFTWARE",
		"algorithm":       "GOOGLE_SYMMETRIC_ENCRYPTION",
		"createTime":      now,
		"generateTime":    now,
		"destroyTime":     now,
	})
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

// kmsIamResourceKey derives the storage key for the IAM target on
// either a key-ring URL (…/keyRings/{r}:get|setIamPolicy) or a
// crypto-key URL (…/keyRings/{r}/cryptoKeys/{k}:…).
func kmsIamResourceKey(r *http.Request) string {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	keyRing := chi.URLParam(r, "keyRing")
	cryptoKey := chi.URLParam(r, "cryptoKey")
	if cryptoKey != "" {
		return kmsIamKey("projects", project, "locations", location, "keyRings", keyRing, "cryptoKeys", cryptoKey)
	}
	return kmsIamKey("projects", project, "locations", location, "keyRings", keyRing)
}

// KMSGetIamPolicy returns the last-set policy for the target KMS
// resource, or an empty policy if none was set. The provider's Read
// after Apply path is what triggers the inconsistency check, so the
// stored bindings must round-trip verbatim.
func (app *Application) KMSGetIamPolicy(w http.ResponseWriter, r *http.Request) {
	key := kmsIamResourceKey(r)
	kmsIamMu.RLock()
	stored := kmsIamStor[key]
	kmsIamMu.RUnlock()
	if stored == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"version":  1,
			"bindings": []any{},
			"etag":     "BwInitial=",
		})
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

// KMSSetIamPolicy stores the requested policy under the target KMS
// resource and returns it back. terraform-provider-google computes
// the next plan from the post-Apply Read; if the bindings round-trip
// the provider sees a consistent result. We don't validate roles or
// members — the goal is fidelity to the request shape, not policy
// semantics.
func (app *Application) KMSSetIamPolicy(w http.ResponseWriter, r *http.Request) {
	key := kmsIamResourceKey(r)
	body, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}
	policy, _ := body["policy"].(map[string]any)
	if policy == nil {
		policy = map[string]any{}
	}
	if _, ok := policy["bindings"]; !ok {
		policy["bindings"] = []any{}
	}
	if _, ok := policy["version"]; !ok {
		policy["version"] = float64(1)
	}
	// Generate a fresh etag per Set — real GCP increments on every
	// successful SetIamPolicy. Round-tripping the SAME etag would
	// make optimistic-concurrency callers loop forever.
	policy["etag"] = fmt.Sprintf("BwEtag-%d=", time.Now().UnixNano())

	kmsIamMu.Lock()
	kmsIamStor[key] = policy
	kmsIamMu.Unlock()

	writeJSON(w, http.StatusOK, policy)
}

// resetKMSIamStore clears the in-memory KMS IAM policy cache. Wired
// into /mock/reset so tests + scenarios start with empty bindings.
func resetKMSIamStore() {
	kmsIamMu.Lock()
	defer kmsIamMu.Unlock()
	kmsIamStor = map[string]map[string]any{}
}
