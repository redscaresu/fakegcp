package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// GetProjectV3 implements cloudresourcemanager.googleapis.com v3
// Projects.GetProject (GET /v3/projects/{project}). v3 is what
// terraform-provider-google v5's newer code paths use — notably the
// getProject() preflight inside google_service_networking_connection's
// Create. The v3 response shape differs from v1:
//   - "name" is the full resource path "projects/{projectId}", not bare id
//   - "lifecycleState" (v1) is renamed to "state" (v3)
//   - "parent" is a string "organizations/{id}", not a {type, id} object
//
// Pre-Ticket-D-2, only v1 was implemented; v3 calls escaped to real
// cloudresourcemanager.googleapis.com because cloud_resource_manager_custom_endpoint
// only routes v1. The fix is two-part: (a) infrafactory's provider
// template adds resource_manager_v3_custom_endpoint, (b) this handler
// catches the v3 calls fakegcp now receives.
func (app *Application) GetProjectV3(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	if project == "" {
		writeGCPError(w, http.StatusBadRequest, "Missing project path parameter", "required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":        "projects/" + project,
		"parent":      "organizations/000000000000",
		"projectId":   project,
		"state":       "ACTIVE",
		"displayName": project,
		"createTime":  "2020-01-01T00:00:00Z",
		"updateTime":  "2020-01-01T00:00:00Z",
		"etag":        "fakegcp-static-etag",
		"labels":      map[string]any{},
	})
}

// GetProject implements cloudresourcemanager.googleapis.com Projects.GetProject
// (GET /v1/projects/{project}). terraform-provider-google v5's getProject
// helper preflights many resources by calling this — google_project_service,
// google_service_networking_connection, anything that needs to resolve the
// owning project's number/state.
//
// Real GCP returns a Project resource with projectNumber, lifecycleState,
// createTime, parent, etc. The provider mainly reads projectId +
// lifecycleState; we synthesise the rest with stable values so the
// response is well-shaped and round-trippable.
//
// Ticket C closeout (2026-05-31): before this handler existed, the
// route 501'd and the SDK surfaced a misleading
// "Error 401 ACCESS_TOKEN_TYPE_UNSUPPORTED" that looked like the
// request had escaped to real cloud. The fix is purely a missing-
// handler bug — no auth, no endpoint-override issue.
func (app *Application) GetProject(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	if project == "" {
		writeGCPError(w, http.StatusBadRequest, "Missing project path parameter", "required")
		return
	}
	// projectNumber is a 12-digit string in real GCP. We emit a
	// deterministic synthetic value derived from the projectId so
	// repeat reads are stable and tests can pin against it.
	writeJSON(w, http.StatusOK, map[string]any{
		"projectNumber":  "000000000000",
		"projectId":      project,
		"lifecycleState": "ACTIVE",
		"name":           project,
		"createTime":     "2020-01-01T00:00:00Z",
		"parent": map[string]any{
			"type": "organization",
			"id":   "000000000000",
		},
	})
}
