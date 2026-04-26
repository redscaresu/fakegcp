package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func gkeOperation(r *http.Request, project, location, targetKind, targetName, opType string) map[string]any {
	opName := "operation-" + uuid.NewString()
	return map[string]any{
		"name":          opName,
		"status":        "DONE",
		"operationType": opType,
		"selfLink":      selfLink(r, "v1", "projects", project, "locations", location, "operations", opName),
		"targetLink":    selfLink(r, "v1", "projects", project, "locations", location, targetKind, targetName),
	}
}

func (app *Application) CreateCluster(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")

	body, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}

	clusterData, ok := body["cluster"].(map[string]any)
	if !ok {
		writeGCPError(w, http.StatusBadRequest, "Missing required field: cluster", "required")
		return
	}

	name, _ := clusterData["name"].(string)
	if name == "" {
		writeGCPError(w, http.StatusBadRequest, "Missing required field: name", "required")
		return
	}

	clusterData["status"] = "RUNNING"
	clusterData["endpoint"] = "10.0.0.1"
	clusterData["location"] = location
	clusterData["selfLink"] = "v1/projects/" + project + "/locations/" + location + "/clusters/" + name

	if _, err := app.repo.CreateCluster(project, location, clusterData); err != nil {
		writeCreateError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, gkeOperation(r, project, location, "clusters", name, "CREATE_CLUSTER"))
}

func (app *Application) GetCluster(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetCluster(project, location, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) ListClusters(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")

	items, err := app.repo.ListClusters(project, location)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"clusters": items})
}

func (app *Application) UpdateCluster(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	name := chi.URLParam(r, "name")

	patch, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}

	if _, err := app.repo.UpdateCluster(project, location, name, patch); err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, gkeOperation(r, project, location, "clusters", name, "UPDATE_CLUSTER"))
}

func (app *Application) DeleteCluster(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	name := chi.URLParam(r, "name")

	if err := app.repo.DeleteCluster(project, location, name); err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, gkeOperation(r, project, location, "clusters", name, "DELETE_CLUSTER"))
}

func (app *Application) CreateNodePool(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	cluster := chi.URLParam(r, "cluster")

	body, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}

	poolData, ok := body["nodePool"].(map[string]any)
	if !ok {
		writeGCPError(w, http.StatusBadRequest, "Missing required field: nodePool", "required")
		return
	}

	name, _ := poolData["name"].(string)
	if name == "" {
		writeGCPError(w, http.StatusBadRequest, "Missing required field: name", "required")
		return
	}

	poolData["status"] = "RUNNING"

	if _, err := app.repo.CreateNodePool(project, location, cluster, poolData); err != nil {
		writeCreateError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, gkeOperation(r, project, location, "clusters/"+cluster+"/nodePools", name, "CREATE_NODE_POOL"))
}

func (app *Application) GetNodePool(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	cluster := chi.URLParam(r, "cluster")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetNodePool(project, location, cluster, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) ListNodePools(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	cluster := chi.URLParam(r, "cluster")

	items, err := app.repo.ListNodePools(project, location, cluster)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"nodePools": items})
}

func (app *Application) DeleteNodePool(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	location := chi.URLParam(r, "location")
	cluster := chi.URLParam(r, "cluster")
	name := chi.URLParam(r, "name")

	if err := app.repo.DeleteNodePool(project, location, cluster, name); err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, gkeOperation(r, project, location, "clusters/"+cluster+"/nodePools", name, "DELETE_NODE_POOL"))
}
