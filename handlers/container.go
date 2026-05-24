package handlers

import (
	"net/http"
	"strings"

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

	// terraform-provider-google reads network + subnetwork from
	// networkConfig.{network,subnetwork} in the GET response (this is
	// where real GCP returns them, in addition to top-level). Without
	// this, state ends up with empty network/subnetwork and the next
	// plan forces a replacement.
	netCfg, _ := clusterData["networkConfig"].(map[string]any)
	if netCfg == nil {
		netCfg = map[string]any{}
		clusterData["networkConfig"] = netCfg
	}
	if v, ok := clusterData["network"].(string); ok && v != "" {
		netCfg["network"] = v
	}
	if v, ok := clusterData["subnetwork"].(string); ok && v != "" {
		netCfg["subnetwork"] = v
	}

	stripEmptyClusterSubObjects(clusterData)

	if _, err := app.repo.CreateCluster(project, location, clusterData); err != nil {
		writeCreateError(w, err)
		return
	}

	// M45 — real GKE auto-creates a `default-pool` node pool alongside
	// cluster creation. terraform-provider-google's `remove_default_node_pool = true`
	// flow expects to delete it post-create; without this, DELETE 404s.
	// Skip if caller already supplied nodePools or set initialNodeCount=0.
	if _, hasPools := clusterData["nodePools"]; !hasPools {
		if initial, ok := clusterData["initialNodeCount"]; !(ok && isZeroNumber(initial)) {
			defaultPool := map[string]any{
				"name":             "default-pool",
				"initialNodeCount": 1,
				"status":           "RUNNING",
				"selfLink":         clusterData["selfLink"].(string) + "/nodePools/default-pool",
			}
			_, _ = app.repo.CreateNodePool(project, location, name, defaultPool)
		}
	}

	writeJSON(w, http.StatusOK, gkeOperation(r, project, location, "clusters", name, "CREATE_CLUSTER"))
}

// deriveZoneFromLocation returns a zone usable in a synthesized IGM URL.
// If location is already zonal (us-central1-a) returns it unchanged;
// regional locations (us-central1) get an "-a" suffix.
func deriveZoneFromLocation(location string) string {
	parts := strings.Split(location, "-")
	if len(parts) >= 3 && len(parts[len(parts)-1]) <= 2 {
		return location
	}
	return location + "-a"
}

// ListInstanceGroupManagers returns synthetic IGM records for every
// node pool in the project (filtered loosely to the requested zone is
// not required for mock purposes — provider validates fields it reads).
func (app *Application) ListInstanceGroupManagers(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	zone := chi.URLParam(r, "zone")
	zoneURL := "https://www.googleapis.com/compute/v1/projects/" + project + "/zones/" + zone

	items := []map[string]any{}
	if pools, err := app.repo.ListAllNodePools(project); err == nil {
		for _, p := range pools {
			name, _ := p["name"].(string)
			if name == "" {
				continue
			}
			igmName := name + "-grp"
			targetSize := extractTargetSize(p)
			selfLinkURL := zoneURL + "/instanceGroupManagers/" + igmName
			items = append(items, map[string]any{
				"kind":          "compute#instanceGroupManager",
				"name":          igmName,
				"zone":          zoneURL,
				"targetSize":    targetSize,
				"selfLink":      selfLinkURL,
				"instanceGroup": zoneURL + "/instanceGroups/" + igmName,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"kind":  "compute#instanceGroupManagerList",
		"items": items,
	})
}

func extractTargetSize(pool map[string]any) int {
	if v, ok := pool["nodeCount"]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case int64:
			return int(n)
		}
	}
	return 1
}

// GetInstanceGroupManager returns a synthetic IGM record used by the
// GKE provider's node_count computation. We look up the node pool by
// stripping the "-grp" suffix from the IGM name and return its
// nodeCount as targetSize.
func (app *Application) GetInstanceGroupManager(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	zone := chi.URLParam(r, "zone")
	name := chi.URLParam(r, "name")

	poolName := strings.TrimSuffix(name, "-grp")
	targetSize := 1
	if pool, err := app.repo.FindNodePoolByName(project, poolName); err == nil && pool != nil {
		if v, ok := pool["nodeCount"]; ok {
			switch n := v.(type) {
			case float64:
				targetSize = int(n)
			case int:
				targetSize = n
			case int64:
				targetSize = int(n)
			}
		}
	}

	zoneURL := "https://www.googleapis.com/compute/v1/projects/" + project + "/zones/" + zone
	selfLinkURL := zoneURL + "/instanceGroupManagers/" + name
	igURL := zoneURL + "/instanceGroups/" + name

	writeJSON(w, http.StatusOK, map[string]any{
		"kind":          "compute#instanceGroupManager",
		"name":          name,
		"zone":          zoneURL,
		"targetSize":    targetSize,
		"selfLink":      selfLinkURL,
		"instanceGroup": igURL,
	})
}

func isZeroNumber(v any) bool {
	switch n := v.(type) {
	case float64:
		return n == 0
	case int:
		return n == 0
	case int64:
		return n == 0
	}
	return false
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

	// terraform-provider-google reads `node_count` from `nodeCount`,
	// not `initialNodeCount`. Provider also unconditionally POSTs
	// nodeCount=0 in the create body — so we override with
	// initialNodeCount whenever it's set, not only when nodeCount is
	// absent.
	if v, ok := poolData["initialNodeCount"]; ok {
		if existing, hasExisting := poolData["nodeCount"]; !hasExisting || isZeroNumber(existing) {
			poolData["nodeCount"] = v
		}
	}

	// Populate instanceGroupUrls — the provider's Read function uses
	// these URLs to call compute.instanceGroupManagers.Get(...) and
	// sums TargetSize across the responses to set `node_count` in
	// state. Without IGM URLs, node_count ends up 0 and the next plan
	// shows phantom `0 -> N` drift.
	igmZone := deriveZoneFromLocation(location)
	igmName := name + "-grp"
	igmURL := "https://www.googleapis.com/compute/v1/projects/" + project +
		"/zones/" + igmZone + "/instanceGroupManagers/" + igmName
	poolData["instanceGroupUrls"] = []any{igmURL}

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

// stripEmptyClusterSubObjects mirrors stripEmptyDNSZoneSubObjects /
// stripEmptyBucketSubObjects — terraform-provider-google POSTs default
// placeholder sub-blocks like `{"autopilot": {"enabled": false}}` and
// `{"networkConfig": {}}`. Echoing them back makes the provider think
// the user configured those blocks, which produces phantom drift on
// the next plan. Strip them before persisting; isShallowEmpty lives
// in dns.go and recognizes nil / empty / all-default values.
func stripEmptyClusterSubObjects(body map[string]any) {
	candidates := []string{
		"addonsConfig",
		"autopilot",
		"autoscaling",
		"binaryAuthorization",
		"controlPlaneEndpointsConfig",
		"databaseEncryption",
		"defaultSnatStatus",
		"loggingConfig",
		"maintenancePolicy",
		"masterAuth",
		"monitoringConfig",
		"networkPolicy",
		"notificationConfig",
		"privateClusterConfig",
		"releaseChannel",
		"resourceUsageExportConfig",
		"shieldedNodes",
		"verticalPodAutoscaling",
		"workloadIdentityConfig",
	}
	for _, key := range candidates {
		if v, ok := body[key]; ok && isShallowEmpty(v) {
			delete(body, key)
		}
	}
}
