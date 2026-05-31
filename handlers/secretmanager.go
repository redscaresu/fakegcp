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

func (app *Application) UpdateSecret(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	secret := chi.URLParam(r, "secret")

	patch, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}
	updated, err := app.repo.UpdateSecret(project, secret, patch)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
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

// AccessSecretVersion implements GET .../versions/{version}:access — the
// payload-fetching verb. Per the Secret Manager v1 spec the response
// shape is {name, payload: {data}} where `data` is the base64-encoded
// payload as it was sent on addVersion. terraform-provider-google's
// secret_version Read path always issues this in addition to the
// basic GET on the version; without the route, Read 404s and a tofu
// apply that re-reads after CreateSecretVersion fails immediately.
//
// Both `latest` and a concrete version ID are supported, mirroring
// GetSecretVersion.
func (app *Application) AccessSecretVersion(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	secret := chi.URLParam(r, "secret")
	version := chi.URLParam(r, "version")

	var item map[string]any
	var err error
	if version == "latest" {
		item, err = app.repo.GetLatestSecretVersion(project, secret)
	} else {
		name := fmt.Sprintf("projects/%s/secrets/%s/versions/%s", project, secret, version)
		item, err = app.repo.GetSecretVersion(name)
	}
	if err != nil {
		writeDomainError(w, err)
		return
	}

	// Build the AccessSecretVersionResponse. Repo stores the verbatim
	// addVersion body which already includes `payload.data`; lift it
	// alongside `name` and drop everything else so the response
	// matches the real API shape.
	resp := map[string]any{"name": item["name"]}
	if payload, ok := item["payload"].(map[string]any); ok {
		resp["payload"] = payload
	} else {
		resp["payload"] = map[string]any{"data": ""}
	}
	writeJSON(w, http.StatusOK, resp)
}

// DestroySecretVersion implements the v1 :destroy verb. Per Secret
// Manager semantics, the version is *not* deleted — it transitions
// to state=DESTROYED, the payload is cleared, and a destroyTime is
// recorded. Subsequent GETs still return the version with the new
// state. Hard-deleting it would diverge from the real API and break
// any caller that expects to see DESTROYED versions in list/get.
func (app *Application) DestroySecretVersion(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	secret := chi.URLParam(r, "secret")
	version := chi.URLParam(r, "version")

	name := fmt.Sprintf("projects/%s/secrets/%s/versions/%s", project, secret, version)
	updated, err := app.repo.DestroySecretVersion(name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// EnableSecretVersion + DisableSecretVersion implement the v1
// :enable/:disable verbs. The transition is persisted so a subsequent
// GET reflects the new state, matching the real Secret Manager.
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
	updated, err := app.repo.SetSecretVersionState(name, state)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
