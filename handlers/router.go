package handlers

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

func (app *Application) CreateRouter(w http.ResponseWriter, r *http.Request) {
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

	body["kind"] = "compute#router"
	body["selfLink"] = selfLink(r, "compute", "v1", "projects", project, "regions", region, "routers", name)
	body["region"] = regionSelfLink(r, project, region)
	body["creationTimestamp"] = time.Now().Format(time.RFC3339)
	if _, ok := body["bgp"]; !ok {
		body["bgp"] = map[string]any{"asn": float64(64514)}
	}
	if bgp, ok := body["bgp"].(map[string]any); ok {
		if _, ok := bgp["asn"]; !ok {
			bgp["asn"] = float64(64514)
		}
	}

	created, err := app.repo.CreateRouter(project, region, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", region, getString(created, "selfLink"), "insert"))
}

func (app *Application) ListRouters(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	region := chi.URLParam(r, "region")

	items, err := app.repo.ListRouters(project, region)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	resp := map[string]any{
		"kind":     "compute#routerList",
		"selfLink": selfLink(r, "compute", "v1", "projects", project, "regions", region, "routers"),
	}
	if len(items) > 0 {
		resp["items"] = items
	}
	writeJSON(w, http.StatusOK, resp)
}

func (app *Application) GetRouter(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	region := chi.URLParam(r, "region")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetRouter(project, region, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) UpdateRouter(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	region := chi.URLParam(r, "region")
	name := chi.URLParam(r, "name")

	patch, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}
	updated, err := app.repo.UpdateRouter(project, region, name, patch)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", region, getString(updated, "selfLink"), "patch"))
}

func (app *Application) DeleteRouter(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	region := chi.URLParam(r, "region")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetRouter(project, region, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := app.repo.DeleteRouter(project, region, name); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", region, getString(item, "selfLink"), "delete"))
}

func (app *Application) CreateRouterNAT(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	region := chi.URLParam(r, "region")
	routerName := chi.URLParam(r, "router")

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
	if _, ok := body["sourceSubnetworkIpRangesToNat"]; !ok {
		body["sourceSubnetworkIpRangesToNat"] = "ALL_SUBNETWORKS_ALL_IP_RANGES"
	}
	body["selfLink"] = selfLink(r, "compute", "v1", "projects", project, "regions", region, "routers", routerName, "nats", name)

	created, err := app.repo.CreateRouterNAT(project, region, routerName, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", region, getString(created, "selfLink"), "insert"))
}

func (app *Application) ListRouterNATs(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	region := chi.URLParam(r, "region")
	routerName := chi.URLParam(r, "router")

	items, err := app.repo.ListRouterNATs(project, region, routerName)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	resp := map[string]any{
		"items": items,
	}
	if items == nil {
		resp["items"] = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (app *Application) GetRouterNAT(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	region := chi.URLParam(r, "region")
	routerName := chi.URLParam(r, "router")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetRouterNAT(project, region, routerName, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) UpdateRouterNAT(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	region := chi.URLParam(r, "region")
	routerName := chi.URLParam(r, "router")
	name := chi.URLParam(r, "name")

	patch, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}
	if _, ok := patch["sourceSubnetworkIpRangesToNat"]; !ok {
		patch["sourceSubnetworkIpRangesToNat"] = "ALL_SUBNETWORKS_ALL_IP_RANGES"
	}
	updated, err := app.repo.UpdateRouterNAT(project, region, routerName, name, patch)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", region, getString(updated, "selfLink"), "patch"))
}

func (app *Application) DeleteRouterNAT(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	region := chi.URLParam(r, "region")
	routerName := chi.URLParam(r, "router")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetRouterNAT(project, region, routerName, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := app.repo.DeleteRouterNAT(project, region, routerName, name); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", region, getString(item, "selfLink"), "delete"))
}
