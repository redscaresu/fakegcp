package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// saIamStore round-trips IAM policies set on service accounts. Used
// by google_service_account_iam_member / _binding / _policy resources
// (which hit /v1/projects/{p}/serviceAccounts/{email}:{get|set}IamPolicy).
//
// fakegcp can't reliably honor cloud_resource_manager_custom_endpoint
// for project-level IAM (BatchingConfig wrapper escapes), so we
// recommend SA-level IAM as the supported substitute. That path goes
// through iam.googleapis.com which routes here.
var (
	saIamMu   sync.RWMutex
	saIamStor = map[string]map[string]any{}
)

func saIamKey(project, email string) string {
	return project + "/" + email
}

func resetSAIamStore() {
	saIamMu.Lock()
	defer saIamMu.Unlock()
	saIamStor = map[string]map[string]any{}
}

func (app *Application) CreateServiceAccount(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")

	body, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}

	accountID, _ := body["accountId"].(string)
	if accountID == "" {
		writeGCPError(w, http.StatusBadRequest, "Missing required field: accountId", "required")
		return
	}

	saData, _ := body["serviceAccount"].(map[string]any)
	if saData == nil {
		saData = map[string]any{}
	}

	email := accountID + "@" + project + ".iam.gserviceaccount.com"
	uniqueID := numericID()
	data := map[string]any{
		"accountId":      accountID,
		"email":          email,
		"uniqueId":       uniqueID,
		"name":           "projects/" + project + "/serviceAccounts/" + email,
		"projectId":      project,
		"oauth2ClientId": uniqueID,
		"disabled":       false,
		"etag":           "BwSAEtag=",
	}
	if displayName, ok := saData["displayName"].(string); ok && displayName != "" {
		data["displayName"] = displayName
	}
	if description, ok := saData["description"].(string); ok {
		data["description"] = description
	}

	created, err := app.repo.CreateServiceAccount(project, data)
	if err != nil {
		writeCreateError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, created)
}

func (app *Application) GetServiceAccount(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	email := chi.URLParam(r, "email")

	item, err := app.repo.GetServiceAccount(project, email)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) ListServiceAccounts(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")

	items, err := app.repo.ListServiceAccounts(project)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"accounts": items})
}

func (app *Application) DeleteServiceAccount(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	email := chi.URLParam(r, "email")

	if err := app.repo.DeleteServiceAccount(project, email); err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{})
}

// GetServiceAccountIamPolicy returns the policy stored for the SA,
// or an empty policy if none was set. Provider's Read-after-Apply
// uses this to verify the binding landed.
func (app *Application) GetServiceAccountIamPolicy(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	email := chi.URLParam(r, "email")
	key := saIamKey(project, email)
	saIamMu.RLock()
	stored := saIamStor[key]
	saIamMu.RUnlock()
	if stored == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"version":      1,
			"bindings":     []any{},
			"auditConfigs": []any{},
			"etag":         "BwSAInitial=",
		})
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

// SetServiceAccountIamPolicy stores the requested policy under the
// SA's storage key and returns it back. Etag rotates per Set so
// optimistic-concurrency callers see fresh values.
func (app *Application) SetServiceAccountIamPolicy(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	email := chi.URLParam(r, "email")
	key := saIamKey(project, email)
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
	if _, ok := policy["auditConfigs"]; !ok {
		policy["auditConfigs"] = []any{}
	}
	if _, ok := policy["version"]; !ok {
		policy["version"] = float64(1)
	}
	policy["etag"] = fmt.Sprintf("BwSAEtag-%d=", time.Now().UnixNano())

	saIamMu.Lock()
	saIamStor[key] = policy
	saIamMu.Unlock()

	writeJSON(w, http.StatusOK, policy)
}

// UpdateServiceAccount handles the v1 PATCH that the Terraform provider
// uses when display_name (or any other mutable field) drifts. The body
// shape mirrors Create — {serviceAccount: {...}, updateMask: "..."} —
// so we unwrap `serviceAccount` if present.
func (app *Application) UpdateServiceAccount(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	email := chi.URLParam(r, "email")

	body, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}
	patch := body
	if nested, ok := body["serviceAccount"].(map[string]any); ok {
		patch = nested
	}
	updated, err := app.repo.UpdateServiceAccount(project, email, patch)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (app *Application) CreateSAKey(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	email := chi.URLParam(r, "email")

	// Read the request so we can echo back caller-supplied fields like
	// keyAlgorithm and privateKeyType — terraform-provider-google
	// treats missing fields as drift on the next plan ("forces replacement").
	body, _ := decodeBody(r)
	keyAlgorithm, _ := body["keyAlgorithm"].(string)
	if keyAlgorithm == "" {
		keyAlgorithm = "KEY_ALG_RSA_2048"
	}
	privateKeyType, _ := body["privateKeyType"].(string)
	if privateKeyType == "" {
		privateKeyType = "TYPE_GOOGLE_CREDENTIALS_FILE"
	}

	keyID := uuid.NewString()
	keyName := "projects/" + project + "/serviceAccounts/" + email + "/keys/" + keyID

	fakeKeyJSON, err := json.Marshal(map[string]any{
		"type":           "service_account",
		"project_id":     project,
		"private_key_id": keyID,
		"client_email":   email,
	})
	if err != nil {
		writeGCPError(w, http.StatusInternalServerError, "Internal error", "internalError")
		return
	}

	now := time.Now()
	data := map[string]any{
		"name":            keyName,
		"keyType":         "USER_MANAGED",
		"keyAlgorithm":    keyAlgorithm,
		"keyOrigin":       "GOOGLE_PROVIDED",
		"privateKeyType":  privateKeyType,
		"privateKeyData":  base64.StdEncoding.EncodeToString(fakeKeyJSON),
		"validAfterTime":  now.Format(time.RFC3339),
		"validBeforeTime": now.AddDate(10, 0, 0).Format(time.RFC3339),
	}

	created, err := app.repo.CreateSAKey(project, email, data)
	if err != nil {
		writeCreateError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, created)
}

func (app *Application) GetSAKey(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	email := chi.URLParam(r, "email")
	keyID := chi.URLParam(r, "keyId")

	name := "projects/" + project + "/serviceAccounts/" + email + "/keys/" + keyID
	item, err := app.repo.GetSAKey(name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) ListSAKeys(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	email := chi.URLParam(r, "email")

	// Real Cloud IAM returns 404 (resource not found) when the parent
	// service account doesn't exist. Match that here rather than
	// silently returning an empty list, which would let callers ignore
	// a typo'd email.
	if _, err := app.repo.GetServiceAccount(project, email); err != nil {
		writeDomainError(w, err)
		return
	}

	items, err := app.repo.ListSAKeys(email)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"keys": items})
}

func (app *Application) DeleteSAKey(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	email := chi.URLParam(r, "email")
	keyID := chi.URLParam(r, "keyId")

	name := "projects/" + project + "/serviceAccounts/" + email + "/keys/" + keyID
	if err := app.repo.DeleteSAKey(name); err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{})
}
