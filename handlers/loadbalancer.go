package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redscaresu/fakegcp/models"
)

func globalResourceLink(r *http.Request, project, collection, name string) string {
	return selfLink(r, "compute", "v1", "projects", project, "global", collection, name)
}

func (app *Application) CreateGlobalAddress(w http.ResponseWriter, r *http.Request) {
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
	body["kind"] = "compute#address"
	body["selfLink"] = globalResourceLink(r, project, "addresses", name)
	body["creationTimestamp"] = time.Now().Format(time.RFC3339)
	if _, ok := body["address"]; !ok {
		body["address"] = fmt.Sprintf("34.%d.%d.%d", randomIPv4Octet(), randomIPv4Octet(), randomIPv4Octet())
	}
	created, err := app.repo.CreateGlobalAddress(project, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(created, "selfLink"), "insert"))
}

func (app *Application) ListGlobalAddresses(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	items, err := app.repo.ListGlobalAddresses(project)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	resp := map[string]any{
		"kind": "compute#addressList",
	}
	if len(items) > 0 {
		resp["items"] = items
	}
	writeJSON(w, http.StatusOK, resp)
}

func (app *Application) GetGlobalAddress(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")
	item, err := app.repo.GetGlobalAddress(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) DeleteGlobalAddress(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")
	item, err := app.repo.GetGlobalAddress(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := app.repo.DeleteGlobalAddress(project, name); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(item, "selfLink"), "delete"))
}

func (app *Application) CreateHealthCheck(w http.ResponseWriter, r *http.Request) {
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
	body["kind"] = "compute#healthCheck"
	body["selfLink"] = globalResourceLink(r, project, "healthChecks", name)
	body["creationTimestamp"] = time.Now().Format(time.RFC3339)
	if _, ok := body["checkIntervalSec"]; !ok {
		body["checkIntervalSec"] = float64(5)
	}
	if _, ok := body["timeoutSec"]; !ok {
		body["timeoutSec"] = float64(5)
	}
	if _, ok := body["unhealthyThreshold"]; !ok {
		body["unhealthyThreshold"] = float64(2)
	}
	if _, ok := body["healthyThreshold"]; !ok {
		body["healthyThreshold"] = float64(2)
	}
	created, err := app.repo.CreateHealthCheck(project, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(created, "selfLink"), "insert"))
}

func (app *Application) ListHealthChecks(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	items, err := app.repo.ListHealthChecks(project)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	resp := map[string]any{
		"kind": "compute#healthCheckList",
	}
	if len(items) > 0 {
		resp["items"] = items
	}
	writeJSON(w, http.StatusOK, resp)
}

func (app *Application) GetHealthCheck(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")
	item, err := app.repo.GetHealthCheck(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) UpdateHealthCheck(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")
	patch, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}
	updated, err := app.repo.UpdateHealthCheck(project, name, patch)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(updated, "selfLink"), "patch"))
}

func (app *Application) DeleteHealthCheck(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")
	item, err := app.repo.GetHealthCheck(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := app.repo.DeleteHealthCheck(project, name); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(item, "selfLink"), "delete"))
}

func (app *Application) CreateBackendService(w http.ResponseWriter, r *http.Request) {
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
	body["kind"] = "compute#backendService"
	body["selfLink"] = globalResourceLink(r, project, "backendServices", name)
	body["creationTimestamp"] = time.Now().Format(time.RFC3339)
	if _, ok := body["protocol"]; !ok {
		body["protocol"] = "HTTP"
	}
	if _, ok := body["loadBalancingScheme"]; !ok {
		body["loadBalancingScheme"] = "EXTERNAL"
	}
	created, err := app.repo.CreateBackendService(project, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(created, "selfLink"), "insert"))
}

func (app *Application) ListBackendServices(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	items, err := app.repo.ListBackendServices(project)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	resp := map[string]any{
		"kind": "compute#backendServiceList",
	}
	if len(items) > 0 {
		resp["items"] = items
	}
	writeJSON(w, http.StatusOK, resp)
}

func (app *Application) GetBackendService(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")
	item, err := app.repo.GetBackendService(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) UpdateBackendService(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")
	patch, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}
	updated, err := app.repo.UpdateBackendService(project, name, patch)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(updated, "selfLink"), "patch"))
}

func (app *Application) DeleteBackendService(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")
	item, err := app.repo.GetBackendService(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := app.repo.DeleteBackendService(project, name); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(item, "selfLink"), "delete"))
}

func (app *Application) CreateSSLCertificate(w http.ResponseWriter, r *http.Request) {
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
	body["kind"] = "compute#sslCertificate"
	body["selfLink"] = globalResourceLink(r, project, "sslCertificates", name)
	body["creationTimestamp"] = time.Now().Format(time.RFC3339)
	created, err := app.repo.CreateSSLCertificate(project, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(created, "selfLink"), "insert"))
}

func (app *Application) ListSSLCertificates(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	items, err := app.repo.ListSSLCertificates(project)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	resp := map[string]any{
		"kind": "compute#sslCertificateList",
	}
	if len(items) > 0 {
		resp["items"] = items
	}
	writeJSON(w, http.StatusOK, resp)
}

func (app *Application) GetSSLCertificate(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")
	item, err := app.repo.GetSSLCertificate(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) DeleteSSLCertificate(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")
	item, err := app.repo.GetSSLCertificate(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := app.repo.DeleteSSLCertificate(project, name); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(item, "selfLink"), "delete"))
}

func (app *Application) CreateTargetHTTPSProxy(w http.ResponseWriter, r *http.Request) {
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
	body["kind"] = "compute#targetHttpsProxy"
	body["selfLink"] = globalResourceLink(r, project, "targetHttpsProxies", name)
	body["creationTimestamp"] = time.Now().Format(time.RFC3339)
	created, err := app.repo.CreateTargetHTTPSProxy(project, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(created, "selfLink"), "insert"))
}

func (app *Application) ListTargetHTTPSProxies(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	items, err := app.repo.ListTargetHTTPSProxies(project)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	resp := map[string]any{
		"kind": "compute#targetHttpsProxyList",
	}
	if len(items) > 0 {
		resp["items"] = items
	}
	writeJSON(w, http.StatusOK, resp)
}

func (app *Application) GetTargetHTTPSProxy(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")
	item, err := app.repo.GetTargetHTTPSProxy(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) UpdateTargetHTTPSProxy(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")
	patch, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}
	updated, err := app.repo.UpdateTargetHTTPSProxy(project, name, patch)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(updated, "selfLink"), "patch"))
}

func (app *Application) DeleteTargetHTTPSProxy(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")
	item, err := app.repo.GetTargetHTTPSProxy(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := app.repo.DeleteTargetHTTPSProxy(project, name); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(item, "selfLink"), "delete"))
}

func (app *Application) CreateURLMap(w http.ResponseWriter, r *http.Request) {
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
	body["kind"] = "compute#urlMap"
	body["selfLink"] = globalResourceLink(r, project, "urlMaps", name)
	body["creationTimestamp"] = time.Now().Format(time.RFC3339)
	created, err := app.repo.CreateURLMap(project, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(created, "selfLink"), "insert"))
}

func (app *Application) ListURLMaps(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	items, err := app.repo.ListURLMaps(project)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	resp := map[string]any{
		"kind": "compute#urlMapList",
	}
	if len(items) > 0 {
		resp["items"] = items
	}
	writeJSON(w, http.StatusOK, resp)
}

func (app *Application) GetURLMap(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")
	item, err := app.repo.GetURLMap(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) UpdateURLMap(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")
	patch, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}
	updated, err := app.repo.UpdateURLMap(project, name, patch)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(updated, "selfLink"), "patch"))
}

func (app *Application) DeleteURLMap(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")
	item, err := app.repo.GetURLMap(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := app.repo.DeleteURLMap(project, name); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(item, "selfLink"), "delete"))
}

func (app *Application) CreateGlobalForwardingRule(w http.ResponseWriter, r *http.Request) {
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
	body["kind"] = "compute#forwardingRule"
	body["selfLink"] = globalResourceLink(r, project, "forwardingRules", name)
	body["creationTimestamp"] = time.Now().Format(time.RFC3339)
	if _, ok := body["IPProtocol"]; !ok {
		body["IPProtocol"] = "TCP"
	}
	if _, ok := body["loadBalancingScheme"]; !ok {
		body["loadBalancingScheme"] = "EXTERNAL"
	}
	created, err := app.repo.CreateGlobalForwardingRule(project, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(created, "selfLink"), "insert"))
}

func (app *Application) ListGlobalForwardingRules(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	items, err := app.repo.ListGlobalForwardingRules(project)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	resp := map[string]any{
		"kind": "compute#forwardingRuleList",
	}
	if len(items) > 0 {
		resp["items"] = items
	}
	writeJSON(w, http.StatusOK, resp)
}

func (app *Application) GetGlobalForwardingRule(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")
	item, err := app.repo.GetGlobalForwardingRule(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) DeleteGlobalForwardingRule(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	name := chi.URLParam(r, "name")
	item, err := app.repo.GetGlobalForwardingRule(project, name)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if err := app.repo.DeleteGlobalForwardingRule(project, name); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", getString(item, "selfLink"), "delete"))
}

// SetLabelsGlobal handles the {resource}/{name}/setLabels POST that
// terraform-provider-google issues for every global compute resource
// it creates, even when no labels are configured. Body shape is
// {labels: {...}, labelFingerprint: "..."}; we don't yet model labels
// per resource, but we verify the target exists so a typo'd self-link
// or wrong-collection URL surfaces as a 404 instead of a misleading
// success — same as the real Compute API.
func (app *Application) SetLabelsGlobal(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	collection := chi.URLParam(r, "collection")
	name := chi.URLParam(r, "name")
	if _, err := decodeBody(r); err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}
	if err := app.assertGlobalResourceExists(project, collection, name); err != nil {
		writeDomainError(w, err)
		return
	}
	target := globalResourceLink(r, project, collection, name)
	writeJSON(w, http.StatusOK, app.newOperation(r, project, "", "", target, "setLabels"))
}

// assertGlobalResourceExists returns models.ErrNotFound if the named
// global compute resource does not exist. Only collections that
// already have a Get/Delete handler are listed here — anything else
// (including a typo'd collection) falls through to ErrNotFound so
// the caller surfaces a 404.
func (app *Application) assertGlobalResourceExists(project, collection, name string) error {
	switch collection {
	case "addresses":
		_, err := app.repo.GetGlobalAddress(project, name)
		return err
	case "healthChecks":
		_, err := app.repo.GetHealthCheck(project, name)
		return err
	case "backendServices":
		_, err := app.repo.GetBackendService(project, name)
		return err
	case "urlMaps":
		_, err := app.repo.GetURLMap(project, name)
		return err
	case "sslCertificates":
		_, err := app.repo.GetSSLCertificate(project, name)
		return err
	case "targetHttpsProxies":
		_, err := app.repo.GetTargetHTTPSProxy(project, name)
		return err
	case "forwardingRules":
		_, err := app.repo.GetGlobalForwardingRule(project, name)
		return err
	case "networks":
		_, err := app.repo.GetNetwork(project, name)
		return err
	case "firewalls":
		_, err := app.repo.GetFirewall(project, name)
		return err
	}
	return models.ErrNotFound
}
