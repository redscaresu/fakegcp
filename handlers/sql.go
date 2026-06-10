package handlers

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// populateSQLInstanceDefaults fills in nested settings sub-blocks
// real Cloud SQL returns by default but that fakegcp's pure echo
// handler would leave nil. terraform-provider-google v5's sqlInstance
// reader derefs these without nil guards, panicking with "Plugin
// did not respond" on ApplyResourceChange. Surfaced in gcp-full-stack
// 2026-06-02 sweep.
//
// Caller values win — defaults only fill keys the caller omitted.
func populateSQLInstanceDefaults(body map[string]any) {
	settings, ok := body["settings"].(map[string]any)
	if !ok {
		settings = map[string]any{}
		body["settings"] = settings
	}
	if _, ok := settings["activationPolicy"]; !ok {
		settings["activationPolicy"] = "ALWAYS"
	}
	if _, ok := settings["availabilityType"]; !ok {
		settings["availabilityType"] = "ZONAL"
	}
	if _, ok := settings["pricingPlan"]; !ok {
		settings["pricingPlan"] = "PER_USE"
	}
	if _, ok := settings["replicationType"]; !ok {
		settings["replicationType"] = "SYNCHRONOUS"
	}
	if _, ok := settings["storageAutoResize"]; !ok {
		settings["storageAutoResize"] = true
	}
	if _, ok := settings["storageAutoResizeLimit"]; !ok {
		settings["storageAutoResizeLimit"] = "0"
	}
	if _, ok := settings["deletionProtectionEnabled"]; !ok {
		settings["deletionProtectionEnabled"] = false
	}
	if _, ok := settings["userLabels"]; !ok {
		settings["userLabels"] = map[string]any{}
	}
	if _, ok := settings["databaseFlags"]; !ok {
		settings["databaseFlags"] = []any{}
	}
	if _, ok := settings["ipConfiguration"].(map[string]any); !ok {
		settings["ipConfiguration"] = map[string]any{
			"ipv4Enabled":      true,
			"authorizedNetworks": []any{},
		}
	} else {
		ipc := settings["ipConfiguration"].(map[string]any)
		if _, ok := ipc["authorizedNetworks"]; !ok {
			ipc["authorizedNetworks"] = []any{}
		}
	}
	if _, ok := settings["backupConfiguration"].(map[string]any); !ok {
		settings["backupConfiguration"] = map[string]any{
			"enabled":                    true,
			"startTime":                  "00:00",
			"binaryLogEnabled":           false,
			"pointInTimeRecoveryEnabled": false,
			"transactionLogRetentionDays": float64(7),
		}
	}
	if _, ok := settings["maintenanceWindow"].(map[string]any); !ok {
		settings["maintenanceWindow"] = map[string]any{
			"day":         float64(0),
			"hour":        float64(0),
			"updateTrack": "stable",
		}
	}
	if _, ok := settings["locationPreference"].(map[string]any); !ok {
		settings["locationPreference"] = map[string]any{}
	}
	if _, ok := settings["dataDiskType"]; !ok {
		settings["dataDiskType"] = "PD_SSD"
	}
	if _, ok := settings["dataDiskSizeGb"]; !ok {
		settings["dataDiskSizeGb"] = "10"
	}
	// settingsVersion is a server-side optimistic-concurrency token;
	// the provider keys planning off this — without it the Update
	// path passes an empty version and Cloud SQL real returns 412.
	if _, ok := settings["settingsVersion"]; !ok {
		settings["settingsVersion"] = "1"
	}

	if _, ok := body["serverCaCert"].(map[string]any); !ok {
		body["serverCaCert"] = map[string]any{
			"kind": "sql#sslCert",
		}
	}
	if _, ok := body["serviceAccountEmailAddress"]; !ok {
		body["serviceAccountEmailAddress"] = "p-cloud-sql@gserviceaccount.com"
	}
}

func sqlOperation(project, targetLink, opType string) map[string]any {
	return map[string]any{
		"kind":          "sql#operation",
		"status":        "DONE",
		"operationType": opType,
		"targetProject": project,
		"targetLink":    targetLink,
		"name":          uuid.NewString(),
	}
}

func (app *Application) CreateSQLInstance(w http.ResponseWriter, r *http.Request) {
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

	region, _ := body["region"].(string)
	if region == "" {
		region = "us-central1"
	}

	body["kind"] = "sql#instance"
	body["state"] = "RUNNABLE"
	body["connectionName"] = project + ":" + region + ":" + name
	body["ipAddresses"] = []map[string]any{{
		"type":      "PRIMARY",
		"ipAddress": fmt.Sprintf("10.%d.%d.%d", randomIPv4Octet(), randomIPv4Octet(), randomIPv4Octet()),
	}}
	body["selfLink"] = selfLink(r, "sql", "v1beta4", "projects", project, "instances", name)

	// Populate server-side defaults real Cloud SQL would emit. The v5
	// provider's sqlInstance reader derefs nested fields without nil
	// guards (settings.activationPolicy, settings.backupConfiguration.*,
	// settings.ipConfiguration.*, settings.maintenanceWindow.*) —
	// without these defaults the provider panics with "Plugin did
	// not respond" on ApplyResourceChange.
	populateSQLInstanceDefaults(body)

	created, err := app.repo.CreateSQLInstance(project, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, sqlOperation(project, getString(created, "selfLink"), "CREATE"))
}

func (app *Application) GetSQLInstance(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetSQLInstance(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) ListSQLInstances(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")

	items, err := app.repo.ListSQLInstances(project)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"kind":  "sql#instancesList",
		"items": items,
	})
}

func (app *Application) UpdateSQLInstance(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")

	patch, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}

	updated, err := app.repo.UpdateSQLInstance(project, name, patch)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, sqlOperation(project, getString(updated, "selfLink"), "UPDATE"))
}

// CRITICAL[sql-instance-cascade-deletes-databases]: deleting a Cloud
// SQL instance MUST cascade-delete all of its databases. Real Cloud
// SQL drops the whole instance atomically; the terraform-provider-
// google's destroy reads /databases after the parent delete and
// expects 404 on each child. Leaving orphan database rows = destroys
// hang until timeout. Locked in by
// TestContract_sql_instance_cascade_deletes_databases.
//
// CRITICAL[sql-instance-cascade-deletes-users]: same invariant for
// the users sub-resource. Locked in by
// TestContract_sql_instance_cascade_deletes_users.
func (app *Application) DeleteSQLInstance(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetSQLInstance(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := app.repo.DeleteSQLInstance(project, name); err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, sqlOperation(project, getString(item, "selfLink"), "DELETE"))
}

func (app *Application) CreateSQLDatabase(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	instance := chi.URLParam(r, "instance")

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

	body["kind"] = "sql#database"
	body["instance"] = instance
	body["selfLink"] = selfLink(r, "sql", "v1beta4", "projects", project, "instances", instance, "databases", name)

	created, err := app.repo.CreateSQLDatabase(project, instance, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, sqlOperation(project, getString(created, "selfLink"), "CREATE_DATABASE"))
}

func (app *Application) GetSQLDatabase(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	instance := chi.URLParam(r, "instance")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetSQLDatabase(project, instance, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) ListSQLDatabases(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	instance := chi.URLParam(r, "instance")

	items, err := app.repo.ListSQLDatabases(project, instance)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"kind":  "sql#databasesList",
		"items": items,
	})
}

func (app *Application) DeleteSQLDatabase(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	instance := chi.URLParam(r, "instance")
	name := chi.URLParam(r, "name")

	item, err := app.repo.GetSQLDatabase(project, instance, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := app.repo.DeleteSQLDatabase(project, instance, name); err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, sqlOperation(project, getString(item, "selfLink"), "DELETE_DATABASE"))
}

func (app *Application) CreateSQLUser(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	instance := chi.URLParam(r, "instance")

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

	body["kind"] = "sql#user"
	body["instance"] = instance

	created, err := app.repo.CreateSQLUser(project, instance, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}

	targetLink := selfLink(r, "sql", "v1beta4", "projects", project, "instances", instance, "users", getString(created, "name"))
	writeJSON(w, http.StatusOK, sqlOperation(project, targetLink, "CREATE_USER"))
}

func (app *Application) ListSQLUsers(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	instance := chi.URLParam(r, "instance")

	// Real Cloud SQL returns 404 (instanceNotFound) when listing users
	// against a missing instance. Match that rather than silently
	// returning an empty list.
	if _, err := app.repo.GetSQLInstance(project, instance); err != nil {
		writeDomainError(w, err)
		return
	}

	items, err := app.repo.ListSQLUsers(project, instance)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"kind":  "sql#usersList",
		"items": items,
	})
}

func (app *Application) UpdateSQLUser(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	instance := chi.URLParam(r, "instance")

	patch, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		name, _ = patch["name"].(string)
	}
	if name == "" {
		writeGCPError(w, http.StatusBadRequest, "Missing required field: name", "required")
		return
	}

	updated, err := app.repo.UpdateSQLUser(project, instance, name, patch)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	targetLink := selfLink(r, "sql", "v1beta4", "projects", project, "instances", instance, "users", getString(updated, "name"))
	writeJSON(w, http.StatusOK, sqlOperation(project, targetLink, "UPDATE_USER"))
}

func (app *Application) DeleteSQLUser(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	instance := chi.URLParam(r, "instance")
	name := r.URL.Query().Get("name")
	if name == "" {
		writeGCPError(w, http.StatusBadRequest, "Missing required query parameter: name", "required")
		return
	}

	targetLink := selfLink(r, "sql", "v1beta4", "projects", project, "instances", instance, "users", name)
	if err := app.repo.DeleteSQLUser(project, instance, name); err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, sqlOperation(project, targetLink, "DELETE_USER"))
}
