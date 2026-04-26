package handlers

import (
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	mrand "math/rand"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// numericID generates a random 19-digit numeric string
func numericID() string {
	buf := make([]byte, 19)
	buf[0] = byte('1' + mrand.Intn(9))
	for i := 1; i < len(buf); i++ {
		buf[i] = byte('0' + mrand.Intn(10))
	}
	return string(buf)
}

// newOperation creates a DONE compute operation and stores it
func (app *Application) newOperation(r *http.Request, project, zone, region, targetLink, opType string) map[string]any {
	name := "operation-" + uuid.NewString()
	now := time.Now().Format(time.RFC3339)

	// Build operation selfLink based on scope
	var opSelfLink string
	if zone != "" {
		opSelfLink = selfLink(r, "compute", "v1", "projects", project, "zones", zone, "operations", name)
	} else if region != "" {
		opSelfLink = selfLink(r, "compute", "v1", "projects", project, "regions", region, "operations", name)
	} else {
		opSelfLink = selfLink(r, "compute", "v1", "projects", project, "global", "operations", name)
	}

	op := map[string]any{
		"kind":          "compute#operation",
		"id":            numericID(),
		"name":          name,
		"status":        "DONE",
		"targetLink":    targetLink,
		"operationType": opType,
		"progress":      float64(100),
		"startTime":     now,
		"endTime":       now,
		"selfLink":      opSelfLink,
	}
	if zone != "" {
		op["zone"] = zoneSelfLink(r, project, zone)
	}
	if region != "" {
		op["region"] = regionSelfLink(r, project, region)
	}
	if err := app.repo.StoreOperation(project, zone, region, name, op); err != nil {
		log.Printf("WARNING: failed to store operation %s: %v", name, err)
	}
	return op
}

func (app *Application) CreateInstance(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	zone := chi.URLParam(r, "zone")

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

	body["kind"] = "compute#instance"
	body["selfLink"] = selfLink(r, "compute", "v1", "projects", project, "zones", zone, "instances", name)
	body["status"] = "RUNNING"
	body["creationTimestamp"] = time.Now().Format(time.RFC3339)
	body["zone"] = zoneSelfLink(r, project, zone)

	created, err := app.repo.CreateInstance(project, zone, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, app.newOperation(r, project, zone, "", getString(created, "selfLink"), "insert"))
}

func (app *Application) GetInstance(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	zone := chi.URLParam(r, "zone")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetInstance(project, zone, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) ListInstances(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	zone := chi.URLParam(r, "zone")

	items, err := app.repo.ListInstances(project, zone)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	resp := map[string]any{
		"kind":     "compute#instanceList",
		"selfLink": selfLink(r, "compute", "v1", "projects", project, "zones", zone, "instances"),
	}
	if len(items) > 0 {
		resp["items"] = items
	}
	writeJSON(w, http.StatusOK, resp)
}

func (app *Application) DeleteInstance(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	zone := chi.URLParam(r, "zone")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetInstance(project, zone, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := app.repo.DeleteInstance(project, zone, name); err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, app.newOperation(r, project, zone, "", getString(item, "selfLink"), "delete"))
}

func (app *Application) CreateDisk(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	zone := chi.URLParam(r, "zone")

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

	body["kind"] = "compute#disk"
	body["selfLink"] = selfLink(r, "compute", "v1", "projects", project, "zones", zone, "disks", name)
	body["status"] = "READY"
	body["creationTimestamp"] = time.Now().Format(time.RFC3339)
	body["zone"] = zoneSelfLink(r, project, zone)

	created, err := app.repo.CreateDisk(project, zone, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, app.newOperation(r, project, zone, "", getString(created, "selfLink"), "insert"))
}

func (app *Application) GetDisk(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	zone := chi.URLParam(r, "zone")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetDisk(project, zone, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) ListDisks(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	zone := chi.URLParam(r, "zone")

	items, err := app.repo.ListDisks(project, zone)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	resp := map[string]any{
		"kind":     "compute#diskList",
		"selfLink": selfLink(r, "compute", "v1", "projects", project, "zones", zone, "disks"),
	}
	if len(items) > 0 {
		resp["items"] = items
	}
	writeJSON(w, http.StatusOK, resp)
}

func (app *Application) DeleteDisk(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	zone := chi.URLParam(r, "zone")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetDisk(project, zone, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := app.repo.DeleteDisk(project, zone, name); err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, app.newOperation(r, project, zone, "", getString(item, "selfLink"), "delete"))
}

func randomIPv4Octet() int64 {
	n, err := rand.Int(rand.Reader, big.NewInt(256))
	if err != nil {
		return int64(mrand.Intn(256))
	}
	return n.Int64()
}

func (app *Application) CreateAddress(w http.ResponseWriter, r *http.Request) {
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

	body["kind"] = "compute#address"
	body["selfLink"] = selfLink(r, "compute", "v1", "projects", project, "regions", region, "addresses", name)
	body["status"] = "RESERVED"
	body["creationTimestamp"] = time.Now().Format(time.RFC3339)
	body["region"] = regionSelfLink(r, project, region)
	if _, ok := body["address"]; !ok {
		body["address"] = fmt.Sprintf("34.%d.%d.%d", randomIPv4Octet(), randomIPv4Octet(), randomIPv4Octet())
	}

	created, err := app.repo.CreateAddress(project, region, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", region, getString(created, "selfLink"), "insert"))
}

func (app *Application) GetAddress(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	region := chi.URLParam(r, "region")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetAddress(project, region, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) ListAddresses(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	region := chi.URLParam(r, "region")

	items, err := app.repo.ListAddresses(project, region)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	resp := map[string]any{
		"kind":     "compute#addressList",
		"selfLink": selfLink(r, "compute", "v1", "projects", project, "regions", region, "addresses"),
	}
	if len(items) > 0 {
		resp["items"] = items
	}
	writeJSON(w, http.StatusOK, resp)
}

func (app *Application) DeleteAddress(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	region := chi.URLParam(r, "region")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetAddress(project, region, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := app.repo.DeleteAddress(project, region, name); err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", region, getString(item, "selfLink"), "delete"))
}

func (app *Application) GetZoneOperation(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	zone := chi.URLParam(r, "zone")
	name := chi.URLParam(r, "name")

	op, err := app.repo.GetOperation(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if getString(op, "zone") != "" && getString(op, "zone") != zone && getString(op, "zone") != zoneSelfLink(r, project, zone) {
		writeGCPError(w, http.StatusNotFound, "The resource was not found", "notFound")
		return
	}
	writeJSON(w, http.StatusOK, op)
}

func (app *Application) GetRegionOperation(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	region := chi.URLParam(r, "region")
	name := chi.URLParam(r, "name")

	op, err := app.repo.GetOperation(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if getString(op, "region") != "" && getString(op, "region") != region && getString(op, "region") != regionSelfLink(r, project, region) {
		writeGCPError(w, http.StatusNotFound, "The resource was not found", "notFound")
		return
	}
	writeJSON(w, http.StatusOK, op)
}

func (app *Application) GetGlobalOperation(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")

	op, err := app.repo.GetOperation(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, op)
}

func getString(data map[string]any, key string) string {
	if v, ok := data[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
