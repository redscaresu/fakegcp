package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/redscaresu/fakegcp/models"
)

// Memorystore (Redis) handlers — google_redis_instance backing.
//
// Wire shape mirrors Cloud Run v2: every mutation returns an
// {kind:"redis#operation", done:true, response:{...}} envelope so
// terraform-provider-google's poll loop converges immediately. State
// is a single JSON blob per (project, location, instanceId).
//
// Routes:
//   POST   /v1/projects/{project}/locations/{location}/instances?instanceId=NAME
//   GET    /v1/projects/{project}/locations/{location}/instances
//   GET    /v1/projects/{project}/locations/{location}/instances/{instance}
//   PATCH  /v1/projects/{project}/locations/{location}/instances/{instance}
//   DELETE /v1/projects/{project}/locations/{location}/instances/{instance}

func memorystoreOperation(project, location string, response map[string]any) map[string]any {
	return map[string]any{
		"name":     fmt.Sprintf("projects/%s/locations/%s/operations/%s", project, location, uuid.NewString()),
		"done":     true,
		"response": response,
	}
}

func (app *Application) CreateRedisInstance(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")

	body, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}
	// Per the Memorystore v1 spec, the instance id is the `instanceId`
	// query parameter; the body's `name` field is server-assigned.
	name := r.URL.Query().Get("instanceId")
	if name == "" {
		name, _ = body["name"].(string)
	}
	if name == "" {
		writeGCPError(w, http.StatusBadRequest, "Missing required field: instanceId", "required")
		return
	}
	if parts := strings.Split(name, "/"); len(parts) > 1 {
		name = parts[len(parts)-1]
	}

	now := time.Now().Format(time.RFC3339)
	body["name"] = fmt.Sprintf("projects/%s/locations/%s/instances/%s", project, location, name)
	body["createTime"] = now
	body["state"] = "READY"
	// Provider Read flow reads host/port even when not configured. Provide
	// synthetic values that match the Memorystore wire shape — host is
	// a private IP slot, port defaults to 6379. Persisted so subsequent
	// reads are stable.
	if _, ok := body["host"]; !ok {
		body["host"] = "10.0.0.1"
	}
	if _, ok := body["port"]; !ok {
		body["port"] = 6379
	}
	if _, ok := body["currentLocationId"]; !ok {
		body["currentLocationId"] = location
	}

	created, err := app.repo.CreateRedisInstance(project, location, body)
	if err != nil {
		if errors.Is(err, models.ErrConflict) {
			writeGCPError(w, http.StatusConflict, "Instance already exists", "conflict")
			return
		}
		writeGCPError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}
	writeJSON(w, http.StatusOK, memorystoreOperation(project, location, created))
}

func (app *Application) GetRedisInstance(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	instance := chi.URLParam(r, "instance")
	got, err := app.repo.GetRedisInstance(project, location, instance)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			writeGCPError(w, http.StatusNotFound, "Instance not found", "notFound")
			return
		}
		writeGCPError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}
	writeJSON(w, http.StatusOK, got)
}

func (app *Application) ListRedisInstances(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	items, err := app.repo.ListRedisInstances(project, location)
	if err != nil {
		writeGCPError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}
	out := map[string]any{}
	if len(items) > 0 {
		out["instances"] = items
	}
	writeJSON(w, http.StatusOK, out)
}

func (app *Application) UpdateRedisInstance(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	instance := chi.URLParam(r, "instance")
	body, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}
	updated, err := app.repo.UpdateRedisInstance(project, location, instance, body)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			writeGCPError(w, http.StatusNotFound, "Instance not found", "notFound")
			return
		}
		writeGCPError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}
	writeJSON(w, http.StatusOK, memorystoreOperation(project, location, updated))
}

func (app *Application) DeleteRedisInstance(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	instance := chi.URLParam(r, "instance")
	if err := app.repo.DeleteRedisInstance(project, location, instance); err != nil {
		if errors.Is(err, models.ErrNotFound) {
			writeGCPError(w, http.StatusNotFound, "Instance not found", "notFound")
			return
		}
		writeGCPError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}
	writeJSON(w, http.StatusOK, memorystoreOperation(project, location, map[string]any{}))
}
