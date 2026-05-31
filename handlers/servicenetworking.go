package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Service Networking handlers implement the subset of
// servicenetworking.googleapis.com/v1 that terraform-provider-google's
// google_service_networking_connection resource exercises:
//
//	POST   /v1/services/{service}/connections
//	GET    /v1/services/{service}/connections?network=X
//	PATCH  /v1/services/{service}/connections/{connection}
//	POST   /v1/services/{service}/connections/{connection}   (DeleteConnection verb)
//
// All mutating ops return a Long Running Operation with done=true so
// the provider's CommonOperationWaiter short-circuits without polling
// (real GCP would return done=false + Operation.name for a subsequent
// GET on the operation URL; we don't model that). Read is a List call
// filtered client-side by network, matching how the provider walks
// the response.

// CreateServiceNetworkingConnection handles
// POST /v1/services/{service}/connections. The body shape is the
// Connection: {network, reservedPeeringRanges}. We synthesise a
// `peering` output field (real GCP picks the peering name; the
// provider only reads it back into state).
func (app *Application) CreateServiceNetworkingConnection(w http.ResponseWriter, r *http.Request) {
	service := chi.URLParam(r, "service")
	body, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}
	network, _ := body["network"].(string)
	if network == "" {
		writeGCPError(w, http.StatusBadRequest, "Missing required field: network", "required")
		return
	}
	if _, ok := body["peering"]; !ok {
		body["peering"] = "servicenetworking-googleapis-com"
	}

	app.snConnectionsMu.Lock()
	if app.snConnections == nil {
		app.snConnections = map[string]map[string]any{}
	}
	app.snConnections[snConnectionKey(service, network)] = body
	app.snConnectionsMu.Unlock()

	writeJSON(w, http.StatusOK, doneOperation("sn-create-"+sanitizeOpName(network), body))
}

// ListServiceNetworkingConnections handles
// GET /v1/services/{service}/connections?network=X. The provider's
// Read enumerates and matches by network, so the filter is server-side
// best-effort: missing filter returns all connections under the
// service.
func (app *Application) ListServiceNetworkingConnections(w http.ResponseWriter, r *http.Request) {
	service := chi.URLParam(r, "service")
	filter := r.URL.Query().Get("network")

	app.snConnectionsMu.RLock()
	var out []map[string]any
	prefix := service + "/"
	for k, v := range app.snConnections {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		if filter != "" {
			if n, _ := v["network"].(string); n != filter {
				continue
			}
		}
		out = append(out, v)
	}
	app.snConnectionsMu.RUnlock()

	if out == nil {
		out = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": out})
}

// PatchServiceNetworkingConnection handles
// PATCH /v1/services/{service}/connections/{connection}. The provider
// uses connection="-" with updateMask="reservedPeeringRanges" and
// force=true, so the body's `network` field identifies the real
// connection to mutate.
func (app *Application) PatchServiceNetworkingConnection(w http.ResponseWriter, r *http.Request) {
	service := chi.URLParam(r, "service")
	body, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}
	network, _ := body["network"].(string)
	if network != "" {
		app.snConnectionsMu.Lock()
		key := snConnectionKey(service, network)
		if existing, ok := app.snConnections[key]; ok {
			for k, v := range body {
				existing[k] = v
			}
		} else {
			if _, has := body["peering"]; !has {
				body["peering"] = "servicenetworking-googleapis-com"
			}
			if app.snConnections == nil {
				app.snConnections = map[string]map[string]any{}
			}
			app.snConnections[key] = body
		}
		app.snConnectionsMu.Unlock()
	}
	writeJSON(w, http.StatusOK, doneOperation("sn-patch-"+sanitizeOpName(network), body))
}

// DeleteServiceNetworkingConnection handles
// POST /v1/services/{service}/connections/{connection} (the
// DeleteConnection verb — same URL as Patch but POST). The body
// carries the network ID via consumerNetwork; if absent, we don't
// know which connection to drop, so emit a done op and leave state
// untouched (the provider then retries Read which returns the entry
// still present — matches the real-API "no-op" pattern when the
// deletion target isn't found).
func (app *Application) DeleteServiceNetworkingConnection(w http.ResponseWriter, r *http.Request) {
	service := chi.URLParam(r, "service")
	body, _ := decodeBody(r)
	network, _ := body["consumerNetwork"].(string)
	if network != "" {
		app.snConnectionsMu.Lock()
		delete(app.snConnections, snConnectionKey(service, network))
		app.snConnectionsMu.Unlock()
	}
	writeJSON(w, http.StatusOK, doneOperation("sn-delete-"+sanitizeOpName(network), nil))
}

// doneOperation builds a Long Running Operation with done=true so the
// provider's wait loop short-circuits. The response field is included
// when payload != nil so cancellable RPCs that read response.* don't
// nil-deref.
func doneOperation(id string, payload map[string]any) map[string]any {
	op := map[string]any{
		"name": "operations/" + id,
		"done": true,
	}
	if payload != nil {
		op["response"] = map[string]any{
			"@type":                 "type.googleapis.com/google.cloud.servicenetworking.v1.Connection",
			"network":               payload["network"],
			"peering":               payload["peering"],
			"reservedPeeringRanges": payload["reservedPeeringRanges"],
		}
	}
	return op
}

// sanitizeOpName produces a chi-safe operation ID by stripping
// slashes that would otherwise be parsed as additional URL segments
// if a caller ever polled /v1/operations/{name}.
func sanitizeOpName(s string) string {
	s = strings.ReplaceAll(s, "/", "-")
	if s == "" {
		s = "anon"
	}
	return fmt.Sprintf("%s", s)
}
