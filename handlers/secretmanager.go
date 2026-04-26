package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

func (app *Application) CreateSecret(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")

	body, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}
	// secretId is a query parameter per the Secret Manager v1 spec, but we
	// also accept it from the body for tooling/test convenience.
	secretID := r.URL.Query().Get("secretId")
	if secretID == "" {
		secretID, _ = body["secretId"].(string)
	}
	if secretID == "" {
		writeGCPError(w, http.StatusBadRequest, "Missing required field: secretId", "required")
		return
	}
	body["name"] = fmt.Sprintf("projects/%s/secrets/%s", project, secretID)
	body["createTime"] = time.Now().Format(time.RFC3339)

	created, err := app.repo.CreateSecret(project, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, created)
}

func (app *Application) ListSecrets(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	items, err := app.repo.ListSecrets(project)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"secrets": items})
}

func (app *Application) GetSecret(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	secret := chi.URLParam(r, "secret")

	item, err := app.repo.GetSecret(project, secret)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) DeleteSecret(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	secret := chi.URLParam(r, "secret")

	if err := app.repo.DeleteSecret(project, secret); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

func (app *Application) CreateSecretVersion(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	secret := chi.URLParam(r, "secret")

	body, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}
	payload, _ := body["payload"].(map[string]any)
	if payload == nil {
		writeGCPError(w, http.StatusBadRequest, "Missing required field: payload", "required")
		return
	}
	if _, ok := payload["data"].(string); !ok {
		writeGCPError(w, http.StatusBadRequest, "Missing required field: payload.data", "required")
		return
	}
	body["state"] = "ENABLED"
	body["createTime"] = time.Now().Format(time.RFC3339)

	created, err := app.repo.CreateSecretVersion(project, secret, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, created)
}

func (app *Application) ListSecretVersions(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	secret := chi.URLParam(r, "secret")

	items, err := app.repo.ListSecretVersions(project, secret)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": items})
}

func (app *Application) GetSecretVersion(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	secret := chi.URLParam(r, "secret")
	version := chi.URLParam(r, "version")

	if version == "latest" {
		item, err := app.repo.GetLatestSecretVersion(project, secret)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}

	name := fmt.Sprintf("projects/%s/secrets/%s/versions/%s", project, secret, version)
	item, err := app.repo.GetSecretVersion(name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) DestroySecretVersion(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	secret := chi.URLParam(r, "secret")
	version := chi.URLParam(r, "version")

	name := fmt.Sprintf("projects/%s/secrets/%s/versions/%s", project, secret, version)
	if err := app.repo.DeleteSecretVersion(name); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

// EnableSecretVersion + DisableSecretVersion implement the v1
// :enable/:disable verbs. We don't model state transitions in detail
// — we just look the version up to confirm it exists and echo back
// the current record with the requested state set.
func (app *Application) EnableSecretVersion(w http.ResponseWriter, r *http.Request) {
	app.setSecretVersionState(w, r, "ENABLED")
}

func (app *Application) DisableSecretVersion(w http.ResponseWriter, r *http.Request) {
	app.setSecretVersionState(w, r, "DISABLED")
}

func (app *Application) setSecretVersionState(w http.ResponseWriter, r *http.Request, state string) {
	project := chi.URLParam(r, "project")
	secret := chi.URLParam(r, "secret")
	version := chi.URLParam(r, "version")

	name := fmt.Sprintf("projects/%s/secrets/%s/versions/%s", project, secret, version)
	item, err := app.repo.GetSecretVersion(name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	item["state"] = state
	writeJSON(w, http.StatusOK, item)
}
