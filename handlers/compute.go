package handlers

import (
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	mrand "math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// populateComputeInstanceDefaults fills in nested fields real
// Compute API would populate server-side but that fakegcp's pure
// echo handler leaves nil. v5 provider's compute_instance reader
// derefs these without nil guards, panicking with "Plugin did not
// respond" on ApplyResourceChange. Surfaced in gcp-full-stack
// 2026-06-02 sweep.
func populateComputeInstanceDefaults(body map[string]any) {
	if _, ok := body["id"]; !ok {
		body["id"] = numericID()
	}
	if _, ok := body["fingerprint"]; !ok {
		body["fingerprint"] = "abcd1234efgh"
	}
	if _, ok := body["cpuPlatform"]; !ok {
		body["cpuPlatform"] = "Intel Broadwell"
	}
	if _, ok := body["canIpForward"]; !ok {
		body["canIpForward"] = false
	}
	if _, ok := body["deletionProtection"]; !ok {
		body["deletionProtection"] = false
	}
	if _, ok := body["startRestricted"]; !ok {
		body["startRestricted"] = false
	}
	if _, ok := body["labelFingerprint"]; !ok {
		body["labelFingerprint"] = "labelfp1234"
	}
	if _, ok := body["labels"]; !ok {
		body["labels"] = map[string]any{}
	}
	if _, ok := body["params"]; !ok {
		body["params"] = map[string]any{}
	}

	// metadata sub-block — provider iterates over items and reads
	// fingerprint. Populate both.
	meta, ok := body["metadata"].(map[string]any)
	if !ok {
		meta = map[string]any{}
		body["metadata"] = meta
	}
	if _, ok := meta["fingerprint"]; !ok {
		meta["fingerprint"] = "metafp1234"
	}
	if _, ok := meta["items"]; !ok {
		meta["items"] = []any{}
	}
	if _, ok := meta["kind"]; !ok {
		meta["kind"] = "compute#metadata"
	}

	// tags sub-block — provider reads fingerprint + items.
	tags, ok := body["tags"].(map[string]any)
	if !ok {
		tags = map[string]any{}
		body["tags"] = tags
	}
	if _, ok := tags["fingerprint"]; !ok {
		tags["fingerprint"] = "tagsfp1234"
	}
	if _, ok := tags["items"]; !ok {
		tags["items"] = []any{}
	}

	// scheduling sub-block — provider reads automaticRestart,
	// onHostMaintenance, preemptible.
	sched, ok := body["scheduling"].(map[string]any)
	if !ok {
		sched = map[string]any{}
		body["scheduling"] = sched
	}
	if _, ok := sched["automaticRestart"]; !ok {
		sched["automaticRestart"] = true
	}
	if _, ok := sched["onHostMaintenance"]; !ok {
		sched["onHostMaintenance"] = "MIGRATE"
	}
	if _, ok := sched["preemptible"]; !ok {
		sched["preemptible"] = false
	}
	if _, ok := sched["provisioningModel"]; !ok {
		sched["provisioningModel"] = "STANDARD"
	}

	// shieldedInstanceConfig sub-block.
	if _, ok := body["shieldedInstanceConfig"].(map[string]any); !ok {
		body["shieldedInstanceConfig"] = map[string]any{
			"enableSecureBoot":          false,
			"enableVtpm":                true,
			"enableIntegrityMonitoring": true,
		}
	}

	// networkInterfaces[] — each interface needs a fingerprint +
	// kind. Provider also reads accessConfigs[] but if the caller
	// omitted them, leaving as nil array is fine (provider handles
	// empty).
	if rawNICs, ok := body["networkInterfaces"].([]any); ok {
		for i, raw := range rawNICs {
			nic, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if _, ok := nic["fingerprint"]; !ok {
				nic["fingerprint"] = fmt.Sprintf("nic%dfp", i)
			}
			if _, ok := nic["kind"]; !ok {
				nic["kind"] = "compute#networkInterface"
			}
			if _, ok := nic["name"]; !ok {
				nic["name"] = fmt.Sprintf("nic%d", i)
			}
			if _, ok := nic["stackType"]; !ok {
				nic["stackType"] = "IPV4_ONLY"
			}
		}
	}

	// disks[] — each disk needs interface + kind + mode + boot flag
	// on the first if not set.
	if rawDisks, ok := body["disks"].([]any); ok {
		for i, raw := range rawDisks {
			disk, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if _, ok := disk["kind"]; !ok {
				disk["kind"] = "compute#attachedDisk"
			}
			if _, ok := disk["mode"]; !ok {
				disk["mode"] = "READ_WRITE"
			}
			if _, ok := disk["interface"]; !ok {
				disk["interface"] = "SCSI"
			}
			if _, ok := disk["type"]; !ok {
				disk["type"] = "PERSISTENT"
			}
			if i == 0 {
				if _, ok := disk["boot"]; !ok {
					disk["boot"] = true
				}
			}
		}
	}
}

// numericID generates a random 18-digit numeric string. We use 18
// (not 19) because terraform-provider-google parses the GCP `id`
// field as int64; 19-digit values can exceed int64 max (9.22e18) and
// the provider returns "expected type 'int', got unconvertible type
// 'string'".
func numericID() string {
	buf := make([]byte, 18)
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

	// Populate server-side defaults real Compute returns by default.
	// The v5 provider's compute_instance reader derefs nested fields
	// (networkInterfaces[].fingerprint, metadata.fingerprint, tags.fingerprint,
	// scheduling.*, shieldedInstanceConfig.*) without nil guards —
	// without these defaults the provider panics with "Plugin did not
	// respond" on ApplyResourceChange. Surfaced in gcp-full-stack
	// 2026-06-02 sweep.
	populateComputeInstanceDefaults(body)

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

// GetZone returns a static zone descriptor for any {zone} the provider
// asks about. terraform-provider-google runs a pre-flight GET on the
// zone before creating compute instances; pre-M48 fakegcp returned 501
// for this path and the instance create errored out before reaching
// the /instances handler. We don't model multiple regions/AZ tiers;
// one shape fits all.
func (app *Application) GetZone(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	zone := chi.URLParam(r, "zone")
	region := regionFromZone(zone)
	writeJSON(w, http.StatusOK, map[string]any{
		"kind":     "compute#zone",
		"name":     zone,
		"region":   "projects/" + project + "/regions/" + region,
		"status":   "UP",
		"selfLink": "projects/" + project + "/zones/" + zone,
	})
}

// regionFromZone extracts the region from a zone id
// (us-central1-a → us-central1). Conservative: if the input doesn't
// look like <region>-<az>, return it unchanged.
func regionFromZone(zone string) string {
	idx := strings.LastIndex(zone, "-")
	if idx <= 0 || idx == len(zone)-1 {
		return zone
	}
	return zone[:idx]
}

// GetGlobalImage returns a static image descriptor for any {image}.
// terraform-provider-google pre-flights public-image lookups (e.g.
// debian-cloud/global/images/debian-11) before google_compute_instance
// create — pre-M50 fakegcp 501s on this path and the instance create
// errors out before reaching the /instances handler. Provider consumes
// name + selfLink + status from the response; the minimal shape works
// for any project/{image} combination.
func (app *Application) GetGlobalImage(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	image := chi.URLParam(r, "image")
	writeJSON(w, http.StatusOK, staticImageDescriptor(project, image))
}

// GetGlobalImageFromFamily mirrors GetGlobalImage for the "from-family"
// lookup the provider sometimes uses instead (e.g. debian-cloud/global/
// images/family/debian-11). Returns a synthetic image id derived from
// the family name so the provider has something stable to reference.
func (app *Application) GetGlobalImageFromFamily(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	family := chi.URLParam(r, "family")
	writeJSON(w, http.StatusOK, staticImageDescriptor(project, family+"-latest"))
}

func staticImageDescriptor(project, image string) map[string]any {
	return map[string]any{
		"kind":              "compute#image",
		"name":              image,
		"status":            "READY",
		"selfLink":          "projects/" + project + "/global/images/" + image,
		"diskSizeGb":        "10",
		"sourceType":        "RAW",
		"creationTimestamp": "2025-01-01T00:00:00Z",
	}
}
