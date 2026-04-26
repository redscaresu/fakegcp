package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

func (app *Application) CreateNetwork(w http.ResponseWriter, r *http.Request) {
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

	body["kind"] = "compute#network"
	if _, ok := body["autoCreateSubnetworks"]; !ok {
		body["autoCreateSubnetworks"] = true
	}
	body["selfLink"] = selfLink(r, "compute", "v1", "projects", project, "global", "networks", name)
	body["creationTimestamp"] = time.Now().Format(time.RFC3339)

	created, err := app.repo.CreateNetwork(project, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(created, "selfLink"), "insert"))
}

func (app *Application) GetNetwork(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetNetwork(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) ListNetworks(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")

	items, err := app.repo.ListNetworks(project)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	resp := map[string]any{
		"kind":     "compute#networkList",
		"selfLink": selfLink(r, "compute", "v1", "projects", project, "global", "networks"),
	}
	if len(items) > 0 {
		resp["items"] = items
	}
	writeJSON(w, http.StatusOK, resp)
}

func (app *Application) UpdateNetwork(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")

	patch, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}

	updated, err := app.repo.UpdateNetwork(project, name, patch)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(updated, "selfLink"), "patch"))
}

func (app *Application) DeleteNetwork(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetNetwork(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := app.repo.DeleteNetwork(project, name); err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(item, "selfLink"), "delete"))
}

func (app *Application) CreateSubnetwork(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	region := chi.URLParam(r, "region")

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

	fp := make([]byte, 8)
	if _, err := rand.Read(fp); err != nil {
		fp = []byte(time.Now().Format("15040500"))
	}

	body["kind"] = "compute#subnetwork"
	body["gatewayAddress"] = "10.0.0.1"
	body["fingerprint"] = hex.EncodeToString(fp)
	body["selfLink"] = selfLink(r, "compute", "v1", "projects", project, "regions", region, "subnetworks", name)
	body["region"] = regionSelfLink(r, project, region)
	body["creationTimestamp"] = time.Now().Format(time.RFC3339)

	created, err := app.repo.CreateSubnetwork(project, region, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", region, getString(created, "selfLink"), "insert"))
}

func (app *Application) GetSubnetwork(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	region := chi.URLParam(r, "region")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetSubnetwork(project, region, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) ListSubnetworks(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	region := chi.URLParam(r, "region")

	items, err := app.repo.ListSubnetworks(project, region)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	resp := map[string]any{
		"kind":     "compute#subnetworkList",
		"selfLink": selfLink(r, "compute", "v1", "projects", project, "regions", region, "subnetworks"),
	}
	if len(items) > 0 {
		resp["items"] = items
	}
	writeJSON(w, http.StatusOK, resp)
}

func (app *Application) UpdateSubnetwork(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	region := chi.URLParam(r, "region")
	name := chi.URLParam(r, "name")

	patch, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}

	updated, err := app.repo.UpdateSubnetwork(project, region, name, patch)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", region, getString(updated, "selfLink"), "patch"))
}

func (app *Application) DeleteSubnetwork(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	region := chi.URLParam(r, "region")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetSubnetwork(project, region, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := app.repo.DeleteSubnetwork(project, region, name); err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", region, getString(item, "selfLink"), "delete"))
}

func (app *Application) CreateFirewall(w http.ResponseWriter, r *http.Request) {
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

	body["kind"] = "compute#firewall"
	if _, ok := body["direction"]; !ok {
		body["direction"] = "INGRESS"
	}
	body["selfLink"] = selfLink(r, "compute", "v1", "projects", project, "global", "firewalls", name)
	body["creationTimestamp"] = time.Now().Format(time.RFC3339)

	created, err := app.repo.CreateFirewall(project, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(created, "selfLink"), "insert"))
}

func (app *Application) GetFirewall(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetFirewall(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) ListFirewalls(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")

	items, err := app.repo.ListFirewalls(project)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	resp := map[string]any{
		"kind":     "compute#firewallList",
		"selfLink": selfLink(r, "compute", "v1", "projects", project, "global", "firewalls"),
	}
	if len(items) > 0 {
		resp["items"] = items
	}
	writeJSON(w, http.StatusOK, resp)
}

func (app *Application) UpdateFirewall(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")

	patch, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}

	updated, err := app.repo.UpdateFirewall(project, name, patch)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(updated, "selfLink"), "patch"))
}

func (app *Application) DeleteFirewall(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetFirewall(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := app.repo.DeleteFirewall(project, name); err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(item, "selfLink"), "delete"))
}
