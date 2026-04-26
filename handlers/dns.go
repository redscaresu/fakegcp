package handlers

import (
	"fmt"
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

// CreateDNSChange implements the v1 changes API the Google Terraform
// provider uses to mutate record sets transactionally. We unwrap the
// {additions, deletions} envelope and dispatch to the rrset CRUD
// operations one at a time. The response shape is the standard
// dns#change envelope.
func (app *Application) CreateDNSChange(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	zone := chi.URLParam(r, "zone")

	body, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}
	deletions, _ := body["deletions"].([]any)
	for _, d := range deletions {
		entry, _ := d.(map[string]any)
		if entry == nil {
			continue
		}
		name, _ := entry["name"].(string)
		rrtype, _ := entry["type"].(string)
		if name == "" || rrtype == "" {
			continue
		}
		if err := app.repo.DeleteDNSRecordSet(project, zone, name, rrtype); err != nil {
			writeDomainError(w, err)
			return
		}
	}
	additions, _ := body["additions"].([]any)
	for _, a := range additions {
		entry, _ := a.(map[string]any)
		if entry == nil {
			continue
		}
		name, _ := entry["name"].(string)
		rrtype, _ := entry["type"].(string)
		if name == "" || rrtype == "" {
			writeGCPError(w, http.StatusBadRequest, "Missing required field: name/type in addition", "required")
			return
		}
		entry["kind"] = "dns#resourceRecordSet"
		if _, err := app.repo.CreateDNSRecordSet(project, zone, entry); err != nil {
			writeCreateError(w, err)
			return
		}
	}

	body["kind"] = "dns#change"
	body["id"] = fmt.Sprintf("change-%d", time.Now().UnixNano())
	body["status"] = "done"
	body["startTime"] = time.Now().Format(time.RFC3339)
	writeJSON(w, http.StatusOK, body)
}

// GetDNSChange returns a stub change record. fakegcp applies changes
// synchronously inside CreateDNSChange, so any change id the provider
// polls for is, by definition, already done. Echoing back a done shape
// is enough for terraform-provider-google's wait loop.
func (app *Application) GetDNSChange(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "change")
	writeJSON(w, http.StatusOK, map[string]any{
		"kind":      "dns#change",
		"id":        id,
		"status":    "done",
		"startTime": time.Now().Format(time.RFC3339),
	})
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
