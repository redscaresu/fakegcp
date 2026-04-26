package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func cloudRunOperation(project, location string, response map[string]any) map[string]any {
	return map[string]any{
		"name":     fmt.Sprintf("projects/%s/locations/%s/operations/%s", project, location, uuid.NewString()),
		"done":     true,
		"response": response,
	}
}

func (app *Application) CreateCloudRunService(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")

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
	// Extract short name if fully qualified (projects/.../services/name)
	if parts := strings.Split(name, "/"); len(parts) > 1 {
		name = parts[len(parts)-1]
	}

	now := time.Now().Format(time.RFC3339)
	body["name"] = fmt.Sprintf("projects/%s/locations/%s/services/%s", project, location, name)
	body["conditions"] = []map[string]any{{"type": "Ready", "state": "CONDITION_SUCCEEDED"}}
	body["uri"] = fmt.Sprintf("https://%s-fakegcp.run.app", name)
	body["reconciling"] = false
	body["createTime"] = now
	body["updateTime"] = now

	created, err := app.repo.CreateCloudRunService(project, location, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cloudRunOperation(project, location, created))
}

func (app *Application) ListCloudRunServices(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")

	items, err := app.repo.ListCloudRunServices(project, location)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"services": items})
}

func (app *Application) GetCloudRunService(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	service := chi.URLParam(r, "service")

	item, err := app.repo.GetCloudRunService(project, location, service)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) UpdateCloudRunService(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	service := chi.URLParam(r, "service")

	patch, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}
	patch["updateTime"] = time.Now().Format(time.RFC3339)
	patch["reconciling"] = false
	patch["conditions"] = []map[string]any{{"type": "Ready", "state": "CONDITION_SUCCEEDED"}}

	updated, err := app.repo.UpdateCloudRunService(project, location, service, patch)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cloudRunOperation(project, location, updated))
}

func (app *Application) DeleteCloudRunService(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	service := chi.URLParam(r, "service")

	item, err := app.repo.GetCloudRunService(project, location, service)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := app.repo.DeleteCloudRunService(project, location, service); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cloudRunOperation(project, location, item))
}
