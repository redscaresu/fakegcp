package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

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
	data := map[string]any{
		"accountId": accountID,
		"email":     email,
		"uniqueId":  numericID(),
		"name":      "projects/" + project + "/serviceAccounts/" + email,
		"projectId": project,
	}
	if displayName, ok := saData["displayName"].(string); ok && displayName != "" {
		data["displayName"] = displayName
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

func (app *Application) CreateSAKey(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	email := chi.URLParam(r, "email")

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
	email := chi.URLParam(r, "email")

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
