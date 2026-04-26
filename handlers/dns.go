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
	// creationTime is server-assigned. Overwriting any client-supplied
	// value matters for identity-check correctness: a malicious or
	// careless caller could otherwise reuse an older timestamp on a
	// recreate, spoofing in-place semantics.
	body["creationTime"] = time.Now().Format(time.RFC3339Nano)

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

	// Real Cloud DNS changes are atomic: a change either applies in full
	// or doesn't apply at all. We approximate this by validating every
	// addition + deletion up-front and snapshotting the records we'd
	// remove before touching state, so any failure mid-apply can roll
	// the change back.
	deletions, _ := body["deletions"].([]any)
	additions, _ := body["additions"].([]any)

	type rrset struct {
		name  string
		rtype string
		entry map[string]any // populated for deletions only (rollback)
	}

	pendingDeletions := make([]rrset, 0, len(deletions))
	for _, d := range deletions {
		entry, _ := d.(map[string]any)
		if entry == nil {
			continue
		}
		name, _ := entry["name"].(string)
		rrtype, _ := entry["type"].(string)
		if name == "" || rrtype == "" {
			writeGCPError(w, http.StatusBadRequest, "Missing required field: name/type in deletion", "required")
			return
		}
		// Snapshot the existing rrset so we can re-create it on rollback.
		// A miss is fine: nothing to roll back.
		current, _ := app.repo.GetDNSRecordSet(project, zone, name, rrtype)
		pendingDeletions = append(pendingDeletions, rrset{name: name, rtype: rrtype, entry: current})
	}

	pendingAdditions := make([]map[string]any, 0, len(additions))
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
		// Server-assigned, never trust the client. See note in
		// CreateDNSRecordSet.
		entry["creationTime"] = time.Now().Format(time.RFC3339Nano)
		pendingAdditions = append(pendingAdditions, entry)
	}

	rollback := func(deleted []rrset, added []rrset) {
		// Order matters: a (delete A, add replacement A) followed by a
		// failed third op must end with the original A in place. We
		// must undo additions FIRST so the slot is empty, then re-add
		// the records we removed. Reversing the order would have the
		// re-create collide with the addition we just inserted, get
		// silently rejected, and then the delete-of-addition would
		// leave neither rrset present.
		for _, rec := range added {
			_ = app.repo.DeleteDNSRecordSet(project, zone, rec.name, rec.rtype)
		}
		for _, rec := range deleted {
			if rec.entry != nil {
				_, _ = app.repo.CreateDNSRecordSet(project, zone, rec.entry)
			}
		}
	}

	completedDeletions := make([]rrset, 0, len(pendingDeletions))
	for _, rec := range pendingDeletions {
		if err := app.repo.DeleteDNSRecordSet(project, zone, rec.name, rec.rtype); err != nil {
			rollback(completedDeletions, nil)
			writeDomainError(w, err)
			return
		}
		completedDeletions = append(completedDeletions, rec)
	}

	completedAdditions := make([]rrset, 0, len(pendingAdditions))
	for _, entry := range pendingAdditions {
		if _, err := app.repo.CreateDNSRecordSet(project, zone, entry); err != nil {
			rollback(completedDeletions, completedAdditions)
			writeCreateError(w, err)
			return
		}
		name, _ := entry["name"].(string)
		rrtype, _ := entry["type"].(string)
		completedAdditions = append(completedAdditions, rrset{name: name, rtype: rrtype})
	}

	change := map[string]any{
		"kind":      "dns#change",
		"id":        fmt.Sprintf("change-%d", time.Now().UnixNano()),
		"status":    "done",
		"startTime": time.Now().Format(time.RFC3339),
	}
	if len(additions) > 0 {
		change["additions"] = additions
	}
	if len(deletions) > 0 {
		change["deletions"] = deletions
	}
	app.recordDNSChange(change)
	writeJSON(w, http.StatusOK, change)
}

// GetDNSChange returns the change record CreateDNSChange persisted.
// fakegcp applies changes synchronously, so the cached record is
// always status=done. Returning a 404 for an unknown change id (rather
// than blindly fabricating a done record) catches malformed polling
// against the test mock the same way Cloud DNS would.
func (app *Application) GetDNSChange(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "change")
	if change := app.lookupDNSChange(id); change != nil {
		writeJSON(w, http.StatusOK, change)
		return
	}
	writeGCPError(w, http.StatusNotFound, fmt.Sprintf("Change %q not found", id), "notFound")
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
