package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
)

// enabledServices tracks which APIs have been enabled per project so
// that the provider's post-enable polling loop (calls ListServices
// with filter=state:ENABLED and expects to find the just-enabled
// service) terminates. Map: project → set of service-IDs.
//
// Keyed at package scope because Application is constructed fresh
// per fakegcp boot AND the state is purely in-process — there's no
// persistence story for serviceusage today (a separate ticket could
// move this into the SQLite repository if it becomes load-bearing).
var (
	enabledServicesMu sync.RWMutex
	enabledServices   = map[string]map[string]bool{}
)

func markServiceEnabled(project, service string) {
	enabledServicesMu.Lock()
	defer enabledServicesMu.Unlock()
	if enabledServices[project] == nil {
		enabledServices[project] = map[string]bool{}
	}
	enabledServices[project][service] = true
}

func listEnabledServices(project string) []string {
	enabledServicesMu.RLock()
	defer enabledServicesMu.RUnlock()
	if enabledServices[project] == nil {
		return nil
	}
	out := make([]string, 0, len(enabledServices[project]))
	for svc := range enabledServices[project] {
		out = append(out, svc)
	}
	return out
}

// Service Usage API stub (M70).
//
// terraform-provider-google's `google_project_service` resource calls
// serviceusage.googleapis.com to enable/disable individual GCP APIs
// per project before downstream resources can be created (e.g.
// pubsub.googleapis.com must be enabled before google_pubsub_topic).
// fakegcp doesn't model per-service enable/disable state — every
// service is "always enabled" because handlers route by URL path,
// not by an API-enabled gate. Returning a structured ENABLED
// response for every service request lets google_project_service
// apply cleanly without forcing the LLM into a feedback iteration.
//
// Real Service Usage returns rich metadata (parent project number,
// quota config, dependency graph, etc.); we emit just the fields
// terraform-provider-google's Read flow inspects: name, parent,
// state. That's enough for apply→plan-no-op→destroy.
//
// Wire shape per https://cloud.google.com/service-usage/docs/reference/rest:
//   GET  /v1/projects/{p}/services/{svc}        → returns a Service
//   GET  /v1/projects/{p}/services              → returns a paged list
//   POST /v1/projects/{p}/services/{svc}:enable → returns an Operation (done)
//   POST /v1/projects/{p}/services/{svc}:disable → returns an Operation (done)
//   POST /v1/projects/{p}/services:batchEnable   → returns an Operation (done)

func serviceUsageResponse(project, service string) map[string]any {
	if service == "" {
		service = "default"
	}
	return map[string]any{
		"name":   "projects/" + project + "/services/" + service,
		"parent": "projects/" + project,
		"config": map[string]any{
			"name":  service,
			"title": service,
		},
		"state": "ENABLED",
	}
}

func (app *Application) GetProjectService(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	service := chi.URLParam(r, "service")
	writeJSON(w, http.StatusOK, serviceUsageResponse(project, service))
}

// ListProjectServices returns every service that's been enabled in
// this project. The provider's post-enable polling loop calls this
// with filter=state:ENABLED expecting to find the just-enabled
// service — if we return an empty list, the loop runs until the 20m
// apply timeout (the failure mode the M70 stub was added to prevent).
func (app *Application) ListProjectServices(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	enabled := listEnabledServices(project)
	services := make([]map[string]any, 0, len(enabled))
	for _, svc := range enabled {
		services = append(services, serviceUsageResponse(project, svc))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"services": services,
	})
}

// EnableProjectService / DisableProjectService / BatchEnableProjectServices
// return a "DONE" Operation that's already complete + record the
// service as enabled so subsequent ListProjectServices calls find it.
func (app *Application) EnableProjectService(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	service := chi.URLParam(r, "service")
	markServiceEnabled(project, service)
	writeJSON(w, http.StatusOK, serviceUsageOperation(project, service))
}

func (app *Application) DisableProjectService(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	service := chi.URLParam(r, "service")
	enabledServicesMu.Lock()
	delete(enabledServices[project], service)
	enabledServicesMu.Unlock()
	writeJSON(w, http.StatusOK, serviceUsageOperation(project, service))
}

func (app *Application) BatchEnableProjectServices(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	// Body shape: { "serviceIds": ["pubsub.googleapis.com", ...] }.
	var body struct {
		ServiceIds []string `json:"serviceIds"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	for _, svc := range body.ServiceIds {
		markServiceEnabled(project, strings.TrimSpace(svc))
	}
	writeJSON(w, http.StatusOK, serviceUsageOperation(project, "batch"))
}

func serviceUsageOperation(project, service string) map[string]any {
	// Use a flat operation name so the provider's "GET the operation"
	// path resolves to /v1/operations/<id> with no embedded slashes
	// that chi's router would interpret as additional path segments.
	// terraform-provider-google polls the operation even when the
	// initial response says done=true; if our GET /v1/operations/<id>
	// route doesn't exist, the poll loops until the provider's 20m
	// timeout expires (the failure mode this whole stub was added to
	// prevent).
	return map[string]any{
		"name": "operations/sustub-" + project + "-" + service,
		"done": true,
		"response": map[string]any{
			"@type":   "type.googleapis.com/google.api.serviceusage.v1.EnableServiceResponse",
			"service": serviceUsageResponse(project, service),
		},
	}
}

// GetServiceUsageOperation handles the post-Enable polling GET. We
// return done=true unconditionally — there's no real async work in
// fakegcp, so the operation is "complete" the instant the provider
// asks about it.
func (app *Application) GetServiceUsageOperation(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	writeJSON(w, http.StatusOK, map[string]any{
		"name": "operations/" + name,
		"done": true,
		"response": map[string]any{
			"@type": "type.googleapis.com/google.api.serviceusage.v1.EnableServiceResponse",
		},
	})
}
