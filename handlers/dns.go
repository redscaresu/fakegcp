package handlers

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

func (app *Application) CreateDNSZone(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")

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
	if _, ok := body["dnsName"]; !ok {
		writeGCPError(w, http.StatusBadRequest, "Missing required field: dnsName", "required")
		return
	}

	body["kind"] = "dns#managedZone"
	body["visibility"] = "public"
	body["nameServers"] = []string{"ns1.fakegcp.com", "ns2.fakegcp.com"}
	body["creationTime"] = time.Now().Format(time.RFC3339)

	created, err := app.repo.CreateDNSZone(project, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, created)
}

func (app *Application) ListDNSZones(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")

	items, err := app.repo.ListDNSZones(project)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"kind":         "dns#managedZonesListResponse",
		"managedZones": items,
	})
}

func (app *Application) GetDNSZone(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	zone := chi.URLParam(r, "zone")

	item, err := app.repo.GetDNSZone(project, zone)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) UpdateDNSZone(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	zone := chi.URLParam(r, "zone")

	patch, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}

	updated, err := app.repo.UpdateDNSZone(project, zone, patch)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (app *Application) DeleteDNSZone(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	zone := chi.URLParam(r, "zone")

	if err := app.repo.DeleteDNSZone(project, zone); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

func (app *Application) CreateDNSRecordSet(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	zone := chi.URLParam(r, "zone")

	body, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}
	name, _ := body["name"].(string)
	rrtype, _ := body["type"].(string)
	if name == "" {
		writeGCPError(w, http.StatusBadRequest, "Missing required field: name", "required")
		return
	}
	if rrtype == "" {
		writeGCPError(w, http.StatusBadRequest, "Missing required field: type", "required")
		return
	}

	body["kind"] = "dns#resourceRecordSet"

	created, err := app.repo.CreateDNSRecordSet(project, zone, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, created)
}

func (app *Application) ListDNSRecordSets(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	zone := chi.URLParam(r, "zone")

	items, err := app.repo.ListDNSRecordSets(project, zone)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"kind":   "dns#resourceRecordSetsListResponse",
		"rrsets": items,
	})
}

func (app *Application) GetDNSRecordSet(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	zone := chi.URLParam(r, "zone")
	name := chi.URLParam(r, "name")
	rrtype := chi.URLParam(r, "type")

	item, err := app.repo.GetDNSRecordSet(project, zone, name, rrtype)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) DeleteDNSRecordSet(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	zone := chi.URLParam(r, "zone")
	name := chi.URLParam(r, "name")
	rrtype := chi.URLParam(r, "type")

	if err := app.repo.DeleteDNSRecordSet(project, zone, name, rrtype); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}
