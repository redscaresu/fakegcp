package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

func randomHexString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405")))
	}
	return hex.EncodeToString(b)
}

func (app *Application) CreateBucket(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	if project == "" {
		writeGCPError(w, http.StatusBadRequest, "Missing required query parameter: project", "required")
		return
	}

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

	now := time.Now().Format(time.RFC3339)
	body["kind"] = "storage#bucket"
	body["id"] = name
	body["selfLink"] = selfLink(r, "storage", "v1", "b", name)
	body["timeCreated"] = now
	body["updated"] = now
	body["etag"] = randomHexString(8)
	if _, ok := body["location"]; !ok {
		body["location"] = "US"
	}
	// M47 close-out — terraform-provider-google expects:
	// (a) location normalized to UPPER_CASE (real Cloud Storage
	//     normalizes "us-central1" → "US-CENTRAL1" on create).
	// (b) projectNumber set to a positive int (provider state-loader
	//     treats 0 as "needs refresh" → spurious drift).
	// (c) publicAccessPrevention, rpo, locationType populated with
	//     defaults — provider treats `(known after apply)` as
	//     planned-change, forcing the bucket to be replaced.
	// (d) softDeletePolicy default (7-day retention, real GCP default).
	// (e) decorative-empty subobjects stripped (hierarchicalNamespace
	//     with all-default fields, etc.) so the read shape matches
	//     what the HCL would round-trip back to.
	if loc, ok := body["location"].(string); ok && loc != "" {
		body["location"] = strings.ToUpper(loc)
	}
	if _, ok := body["projectNumber"]; !ok {
		body["projectNumber"] = "100000000001"
	}
	if _, ok := body["locationType"]; !ok {
		body["locationType"] = "region"
	}
	if _, ok := body["publicAccessPrevention"]; !ok {
		body["publicAccessPrevention"] = "inherited"
	}
	if _, ok := body["rpo"]; !ok {
		body["rpo"] = "DEFAULT"
	}
	if _, ok := body["softDeletePolicy"]; !ok {
		body["softDeletePolicy"] = map[string]any{
			"retentionDurationSeconds": "604800",
			"effectiveTime":            now,
		}
	}
	stripEmptyBucketSubObjects(body)

	created, err := app.repo.CreateBucket(project, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, created)
}

func (app *Application) GetBucket(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")

	item, err := app.repo.GetBucket(bucket)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) ListBuckets(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	if project == "" {
		writeGCPError(w, http.StatusBadRequest, "Missing required query parameter: project", "required")
		return
	}

	items, err := app.repo.ListBuckets(project)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"kind":  "storage#buckets",
		"items": items,
	})
}

func (app *Application) UpdateBucket(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")

	patch, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}

	updated, err := app.repo.UpdateBucket(bucket, patch)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

func (app *Application) DeleteBucket(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")

	if err := app.repo.DeleteBucket(bucket); err != nil {
		writeDomainError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// stripEmptyBucketSubObjects removes empty/all-default sub-object
// fields that terraform-provider-google emits as positional
// placeholders on the create body but would interpret as "configured"
// if echoed back on read. Same pattern as stripEmptyDNSZoneSubObjects.
// M47 close-out.
func stripEmptyBucketSubObjects(body map[string]any) {
	for _, key := range []string{
		"hierarchicalNamespace",
		"versioning",
		"website",
		"cors",
		"lifecycle",
		"encryption",
		"logging",
		"retentionPolicy",
		"iamConfiguration",
		"customPlacementConfig",
		"autoclass",
		"objectRetention",
		"ipFilter",
		"defaultObjectAcl",
		"labels",
	} {
		v, ok := body[key]
		if !ok {
			continue
		}
		// Don't strip encryption — user explicitly configured it in HCL
		// and the response shape matters for drift checks. Only strip
		// when shallow-empty per isShallowEmpty (defined in dns.go).
		if isShallowEmpty(v) {
			delete(body, key)
		}
	}
}
