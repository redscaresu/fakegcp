package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

func randomHexString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405")))
	}
	return hex.EncodeToString(b)
}

func (app *Application) CreateBucket(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	if project == "" {
		writeGCPError(w, http.StatusBadRequest, "Missing required query parameter: project", "required")
		return
	}

	body, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}

	name, _ := body["name"].(string)
	if name == "" {
		writeGCPError(w, http.StatusBadRequest, "Missing required field: name", "required")
		return
	}

	now := time.Now().Format(time.RFC3339)
	body["kind"] = "storage#bucket"
	body["id"] = name
	body["selfLink"] = selfLink(r, "storage", "v1", "b", name)
	body["timeCreated"] = now
	body["updated"] = now
	body["etag"] = randomHexString(8)
	if _, ok := body["location"]; !ok {
		body["location"] = "US"
	}

	created, err := app.repo.CreateBucket(project, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, created)
}

func (app *Application) GetBucket(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")

	item, err := app.repo.GetBucket(bucket)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) ListBuckets(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	if project == "" {
		writeGCPError(w, http.StatusBadRequest, "Missing required query parameter: project", "required")
		return
	}

	items, err := app.repo.ListBuckets(project)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"kind":  "storage#buckets",
		"items": items,
	})
}

func (app *Application) UpdateBucket(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")

	patch, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}

	updated, err := app.repo.UpdateBucket(bucket, patch)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

func (app *Application) DeleteBucket(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")

	if err := app.repo.DeleteBucket(bucket); err != nil {
		writeDomainError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
