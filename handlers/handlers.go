package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/redscaresu/fakegcp/models"
	"github.com/redscaresu/fakegcp/repository"
)

type Application struct {
	repo *repository.Repository

	dnsChangesMu       sync.RWMutex
	dnsChanges         map[string]map[string]any
	dnsChangesSnapshot map[string]map[string]any

	// Service Networking connections (private services access for
	// google_service_networking_connection). The provider's Read calls
	// List(parentService).Network(name) so we key the in-memory store
	// by (service, network) for cheap filter. Stored verbatim from the
	// create body so subsequent List reflects the same shape.
	snConnectionsMu       sync.RWMutex
	snConnections         map[string]map[string]any
	snConnectionsSnapshot map[string]map[string]any
}

// dnsChangeKey scopes a cached DNS change record by the
// (project, zone, id) tuple it was created under, matching the
// real Cloud DNS API contract.
func dnsChangeKey(project, zone, id string) string {
	return project + "/" + zone + "/" + id
}

func NewApplication(repo *repository.Repository) *Application {
	return &Application{
		repo:          repo,
		dnsChanges:    map[string]map[string]any{},
		snConnections: map[string]map[string]any{},
	}
}

// recordDNSChange caches a change record by (project, zone, id) so
// GetDNSChange can return it later. Cloud DNS retains change history
// per zone forever in production; the in-memory cache here is good
// enough for tests.
func (app *Application) recordDNSChange(project, zone string, change map[string]any) {
	id, _ := change["id"].(string)
	if id == "" {
		return
	}
	app.dnsChangesMu.Lock()
	defer app.dnsChangesMu.Unlock()
	app.dnsChanges[dnsChangeKey(project, zone, id)] = change
}

func (app *Application) lookupDNSChange(project, zone, id string) map[string]any {
	app.dnsChangesMu.RLock()
	defer app.dnsChangesMu.RUnlock()
	return app.dnsChanges[dnsChangeKey(project, zone, id)]
}

// resetDNSChanges clears the cached change history *and* the
// snapshot baseline. Called from the /mock/reset admin path so a
// reset wipes both the SQLite repo and the in-memory change
// caches; the repo's Reset() drops its .snapshot file too, so
// keeping the snapshot here would leave a stale baseline that
// could re-emerge on a later /mock/restore.
func (app *Application) resetDNSChanges() {
	app.dnsChangesMu.Lock()
	defer app.dnsChangesMu.Unlock()
	app.dnsChanges = map[string]map[string]any{}
	app.dnsChangesSnapshot = nil
}

// snapshotDNSChanges captures the current cache so a later
// restoreDNSChanges can roll it back. Mirrors the repo's
// VACUUM-INTO snapshot/restore pair.
func (app *Application) snapshotDNSChanges() {
	app.dnsChangesMu.Lock()
	defer app.dnsChangesMu.Unlock()
	snap := make(map[string]map[string]any, len(app.dnsChanges))
	for k, v := range app.dnsChanges {
		snap[k] = v
	}
	app.dnsChangesSnapshot = snap
}

func (app *Application) restoreDNSChanges() {
	app.dnsChangesMu.Lock()
	defer app.dnsChangesMu.Unlock()
	if app.dnsChangesSnapshot == nil {
		return
	}
	restored := make(map[string]map[string]any, len(app.dnsChangesSnapshot))
	for k, v := range app.dnsChangesSnapshot {
		restored[k] = v
	}
	app.dnsChanges = restored
}

// snConnectionKey scopes a Service Networking connection by
// (parent service, network). The provider's Read filters by network,
// so this is the unique identity the response shape needs to match.
func snConnectionKey(service, network string) string {
	return service + "/" + network
}

// resetServiceNetworkingConnections clears the cache + snapshot.
// Mirrors resetDNSChanges so /mock/reset wipes SN state alongside
// DNS state — without this, connections leak across scenarios and
// a stale connection from scenario N would surface as a phantom on
// scenario N+1's Read.
func (app *Application) resetServiceNetworkingConnections() {
	app.snConnectionsMu.Lock()
	defer app.snConnectionsMu.Unlock()
	app.snConnections = map[string]map[string]any{}
	app.snConnectionsSnapshot = nil
}

func (app *Application) snapshotServiceNetworkingConnections() {
	app.snConnectionsMu.Lock()
	defer app.snConnectionsMu.Unlock()
	snap := make(map[string]map[string]any, len(app.snConnections))
	for k, v := range app.snConnections {
		snap[k] = v
	}
	app.snConnectionsSnapshot = snap
}

func (app *Application) restoreServiceNetworkingConnections() {
	app.snConnectionsMu.Lock()
	defer app.snConnectionsMu.Unlock()
	if app.snConnectionsSnapshot == nil {
		return
	}
	restored := make(map[string]map[string]any, len(app.snConnectionsSnapshot))
	for k, v := range app.snConnectionsSnapshot {
		restored[k] = v
	}
	app.snConnections = restored
}

func decodeBody(r *http.Request) (map[string]any, error) {
	out := map[string]any{}
	if r == nil || r.Body == nil {
		return out, nil
	}
	defer r.Body.Close()

	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(raw)) == "" {
		return out, nil
	}

	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("json encode error: %v", err)
	}
}

func writeGCPError(w http.ResponseWriter, code int, message, reason string) {
	writeJSON(w, code, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
			"errors": []map[string]any{
				{
					"message": message,
					"domain":  "global",
					"reason":  reason,
				},
			},
		},
	})
}

func writeCreateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, models.ErrNotFound):
		writeGCPError(w, http.StatusNotFound, "Referenced resource not found", "notFound")
	case errors.Is(err, models.ErrAlreadyExists):
		writeGCPError(w, http.StatusConflict, "Resource already exists", "alreadyExists")
	default:
		writeGCPError(w, http.StatusInternalServerError, "Internal error", "internalError")
	}
}

func writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, models.ErrNotFound):
		writeGCPError(w, http.StatusNotFound, "The resource was not found", "notFound")
	case errors.Is(err, models.ErrInUse):
		writeGCPError(w, http.StatusConflict, "The resource has dependents and cannot be deleted", "resourceInUseByAnotherResource")
	case errors.Is(err, models.ErrTerminalState):
		writeGCPError(w, http.StatusConflict, "The resource is in a terminal state", "conflict")
	case errors.Is(err, models.ErrConflict):
		// Generic catch-all for callers that still raise the
		// unspecialised sentinel. Prefer ErrInUse or
		// ErrTerminalState so the 409 payload matches the real
		// Cloud API for the specific class of conflict.
		writeGCPError(w, http.StatusConflict, "The resource is in a conflicting state", "conflict")
	default:
		writeGCPError(w, http.StatusInternalServerError, "Internal error", "internalError")
	}
}

func selfLink(r *http.Request, pathParts ...string) string {
	parts := make([]string, 0, len(pathParts))
	for _, p := range pathParts {
		trimmed := strings.Trim(p, "/")
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("http://%s/", r.Host)
	}
	return fmt.Sprintf("http://%s/%s", r.Host, strings.Join(parts, "/"))
}

func zoneSelfLink(r *http.Request, project, zone string) string {
	return selfLink(r, "compute", "v1", "projects", project, "zones", zone)
}

func regionSelfLink(r *http.Request, project, region string) string {
	return selfLink(r, "compute", "v1", "projects", project, "regions", region)
}

func requireBearerToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if auth == "" {
			writeGCPError(w, http.StatusUnauthorized, "Request is missing required authentication credential.", "required")
			return
		}

		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			writeGCPError(w, http.StatusUnauthorized, "Request is missing required authentication credential.", "required")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (app *Application) RegisterRoutes(r chi.Router) {
	// Admin (no auth)
	r.Post("/mock/reset", app.ResetState)
	r.Post("/mock/snapshot", app.SnapshotState)
	r.Post("/mock/restore", app.RestoreState)
	r.Get("/mock/state", app.FullState)
	r.Get("/mock/state/{service}", app.ServiceState)

	// GCP routes (auth required)
	r.Group(func(r chi.Router) {
		r.Use(requireBearerToken)

		// Compute Engine
		r.Route("/compute/v1/projects/{project}", func(r chi.Router) {
			// Global resources
			r.Route("/global", func(r chi.Router) {
				r.Get("/networks", app.ListNetworks)
				r.Post("/networks", app.CreateNetwork)
				r.Get("/networks/{name}", app.GetNetwork)
				r.Delete("/networks/{name}", app.DeleteNetwork)
				r.Patch("/networks/{name}", app.UpdateNetwork)

				r.Get("/firewalls", app.ListFirewalls)
				r.Post("/firewalls", app.CreateFirewall)
				r.Get("/firewalls/{name}", app.GetFirewall)
				r.Delete("/firewalls/{name}", app.DeleteFirewall)
				r.Patch("/firewalls/{name}", app.UpdateFirewall)

				r.Get("/addresses", app.ListGlobalAddresses)
				r.Post("/addresses", app.CreateGlobalAddress)
				r.Get("/addresses/{name}", app.GetGlobalAddress)
				r.Delete("/addresses/{name}", app.DeleteGlobalAddress)

				// GET /global/images/{image} returns a static image
				// descriptor for any {image} the provider asks about —
				// terraform-provider-google pre-flights public-image
				// lookups (e.g., debian-cloud/global/images/debian-11)
				// before google_compute_instance create. Same shape
				// rationale as M48's GetZone: provider only reads name
				// + status + selfLink. M50 close-out.
				r.Get("/images/{image}", app.GetGlobalImage)
				r.Get("/images/family/{family}", app.GetGlobalImageFromFamily)

				r.Get("/healthChecks", app.ListHealthChecks)
				r.Post("/healthChecks", app.CreateHealthCheck)
				r.Get("/healthChecks/{name}", app.GetHealthCheck)
				r.Patch("/healthChecks/{name}", app.UpdateHealthCheck)
				r.Delete("/healthChecks/{name}", app.DeleteHealthCheck)

				r.Get("/backendServices", app.ListBackendServices)
				r.Post("/backendServices", app.CreateBackendService)
				r.Get("/backendServices/{name}", app.GetBackendService)
				r.Patch("/backendServices/{name}", app.UpdateBackendService)
				r.Put("/backendServices/{name}", app.UpdateBackendService)
				r.Delete("/backendServices/{name}", app.DeleteBackendService)

				r.Get("/sslCertificates", app.ListSSLCertificates)
				r.Post("/sslCertificates", app.CreateSSLCertificate)
				r.Get("/sslCertificates/{name}", app.GetSSLCertificate)
				r.Delete("/sslCertificates/{name}", app.DeleteSSLCertificate)

				r.Get("/targetHttpsProxies", app.ListTargetHTTPSProxies)
				r.Post("/targetHttpsProxies", app.CreateTargetHTTPSProxy)
				r.Get("/targetHttpsProxies/{name}", app.GetTargetHTTPSProxy)
				r.Patch("/targetHttpsProxies/{name}", app.UpdateTargetHTTPSProxy)
				r.Delete("/targetHttpsProxies/{name}", app.DeleteTargetHTTPSProxy)

				r.Get("/urlMaps", app.ListURLMaps)
				r.Post("/urlMaps", app.CreateURLMap)
				r.Get("/urlMaps/{name}", app.GetURLMap)
				r.Patch("/urlMaps/{name}", app.UpdateURLMap)
				r.Delete("/urlMaps/{name}", app.DeleteURLMap)

				r.Get("/forwardingRules", app.ListGlobalForwardingRules)
				r.Post("/forwardingRules", app.CreateGlobalForwardingRule)
				r.Get("/forwardingRules/{name}", app.GetGlobalForwardingRule)
				r.Delete("/forwardingRules/{name}", app.DeleteGlobalForwardingRule)

				// Catch-all setLabels for global resources. terraform-provider-
				// google issues this on every global compute resource even
				// when there are no labels configured.
				r.Post("/{collection}/{name}/setLabels", app.SetLabelsGlobal)

				r.Get("/operations/{name}", app.GetGlobalOperation)
			})

			// Zonal resources.
			r.Route("/zones/{zone}", func(r chi.Router) {
				// GET /zones/{zone} returns a static zone descriptor
				// so terraform-provider-google's pre-flight zone lookup
				// (which fires before google_compute_instance create)
				// succeeds instead of 501ing. M48 close-out. Must live
				// inside the Route block — a top-level r.Get on the
				// same prefix is shadowed by the Route's subrouter.
				r.Get("/", app.GetZone)

				r.Get("/instances", app.ListInstances)
				r.Post("/instances", app.CreateInstance)
				r.Get("/instances/{name}", app.GetInstance)
				r.Delete("/instances/{name}", app.DeleteInstance)

				r.Get("/disks", app.ListDisks)
				r.Post("/disks", app.CreateDisk)
				r.Get("/disks/{name}", app.GetDisk)
				r.Delete("/disks/{name}", app.DeleteDisk)

				r.Get("/operations/{name}", app.GetZoneOperation)

				// Synthetic instance-group-manager endpoint used by the
				// GKE provider to compute node_count from TargetSize.
				// See container.go GetInstanceGroupManager (M45).
				r.Get("/instanceGroupManagers", app.ListInstanceGroupManagers)
				r.Get("/instanceGroupManagers/{name}", app.GetInstanceGroupManager)
			})

			// Regional resources
			r.Route("/regions/{region}", func(r chi.Router) {
				r.Get("/subnetworks", app.ListSubnetworks)
				r.Post("/subnetworks", app.CreateSubnetwork)
				r.Get("/subnetworks/{name}", app.GetSubnetwork)
				r.Delete("/subnetworks/{name}", app.DeleteSubnetwork)
				r.Patch("/subnetworks/{name}", app.UpdateSubnetwork)

				r.Get("/addresses", app.ListAddresses)
				r.Post("/addresses", app.CreateAddress)
				r.Get("/addresses/{name}", app.GetAddress)
				r.Delete("/addresses/{name}", app.DeleteAddress)

				r.Get("/routers", app.ListRouters)
				r.Post("/routers", app.CreateRouter)
				r.Get("/routers/{name}", app.GetRouter)
				r.Patch("/routers/{name}", app.UpdateRouter)
				r.Delete("/routers/{name}", app.DeleteRouter)

				r.Get("/routers/{router}/nats", app.ListRouterNATs)
				r.Post("/routers/{router}/nats", app.CreateRouterNAT)
				r.Get("/routers/{router}/nats/{name}", app.GetRouterNAT)
				r.Patch("/routers/{router}/nats/{name}", app.UpdateRouterNAT)
				r.Delete("/routers/{router}/nats/{name}", app.DeleteRouterNAT)

				r.Get("/operations/{name}", app.GetRegionOperation)
			})
		})

		// Cloud KMS — minimal stubs so encryption-required policies
		// (gcp.encryption: CMEK on google_storage_bucket, etc.) can
		// be satisfied by declaring KMS resources in HCL. We don't
		// model key material or rotation; just enough to round-trip
		// google_kms_key_ring + google_kms_crypto_key + their data
		// sources, plus the IAM bindings the provider issues on read.
		r.Route("/v1/projects/{project}/locations/{location}/keyRings", func(r chi.Router) {
			r.Post("/", app.KMSCreateKeyRing)
			r.Get("/", app.KMSListKeyRings)
			r.Get("/{keyRing}", app.KMSGetKeyRing)
			r.Post("/{keyRing}:getIamPolicy", app.KMSGetIamPolicy)
			r.Post("/{keyRing}:setIamPolicy", app.KMSSetIamPolicy)
			r.Post("/{keyRing}/cryptoKeys", app.KMSCreateCryptoKey)
			r.Get("/{keyRing}/cryptoKeys", app.KMSListCryptoKeys)
			r.Get("/{keyRing}/cryptoKeys/{cryptoKey}", app.KMSGetCryptoKey)
			r.Patch("/{keyRing}/cryptoKeys/{cryptoKey}", app.KMSUpdateCryptoKey)
			r.Post("/{keyRing}/cryptoKeys/{cryptoKey}:getIamPolicy", app.KMSGetIamPolicy)
			r.Post("/{keyRing}/cryptoKeys/{cryptoKey}:setIamPolicy", app.KMSSetIamPolicy)
		})

		// Container (GKE)
		r.Route("/v1/projects/{project}/locations/{location}", func(r chi.Router) {
			r.Get("/clusters", app.ListClusters)
			r.Post("/clusters", app.CreateCluster)
			r.Get("/clusters/{name}", app.GetCluster)
			r.Delete("/clusters/{name}", app.DeleteCluster)
			r.Put("/clusters/{name}", app.UpdateCluster)

			r.Get("/clusters/{cluster}/nodePools", app.ListNodePools)
			r.Post("/clusters/{cluster}/nodePools", app.CreateNodePool)
			r.Get("/clusters/{cluster}/nodePools/{name}", app.GetNodePool)
			r.Delete("/clusters/{cluster}/nodePools/{name}", app.DeleteNodePool)
		})

		// Cloud SQL
		r.Route("/sql/v1beta4/projects/{project}", func(r chi.Router) {
			r.Get("/instances", app.ListSQLInstances)
			r.Post("/instances", app.CreateSQLInstance)
			r.Get("/instances/{name}", app.GetSQLInstance)
			r.Delete("/instances/{name}", app.DeleteSQLInstance)
			r.Patch("/instances/{name}", app.UpdateSQLInstance)

			r.Get("/instances/{instance}/databases", app.ListSQLDatabases)
			r.Post("/instances/{instance}/databases", app.CreateSQLDatabase)
			r.Get("/instances/{instance}/databases/{name}", app.GetSQLDatabase)
			r.Delete("/instances/{instance}/databases/{name}", app.DeleteSQLDatabase)

			r.Get("/instances/{instance}/users", app.ListSQLUsers)
			r.Post("/instances/{instance}/users", app.CreateSQLUser)
			r.Delete("/instances/{instance}/users", app.DeleteSQLUser)
			r.Put("/instances/{instance}/users", app.UpdateSQLUser)
		})

		r.Post("/v1/projects/{project}:setIamPolicy", app.SetIAMPolicy)
		r.Post("/v1/projects/{project}:getIamPolicy", app.GetIAMPolicy)

		// IAM + Cloud Resource Manager root
		r.Route("/v1/projects/{project}", func(r chi.Router) {
			// Cloud Resource Manager — Projects.GetProject (Ticket C).
			// terraform-provider-google v5's getProject helper preflights
			// many resources (google_project_service, google_service_-
			// networking_connection, google_storage_bucket with project
			// reference) by calling GET /v1/projects/{project} on
			// cloudresourcemanager.googleapis.com. Without this route,
			// fakegcp returned 501 and the provider surfaced a confusing
			// 401-OAuth error (ACCESS_TOKEN_TYPE_UNSUPPORTED) that looked
			// like the request had escaped to real cloud — wasting a
			// session debugging "auth escape" before the real problem
			// was "missing handler". Synthetic always-ACTIVE response.
			r.Get("/", app.GetProject)

			r.Get("/serviceAccounts", app.ListServiceAccounts)
			r.Post("/serviceAccounts", app.CreateServiceAccount)
			r.Get("/serviceAccounts/{email}", app.GetServiceAccount)
			r.Delete("/serviceAccounts/{email}", app.DeleteServiceAccount)
			r.Patch("/serviceAccounts/{email}", app.UpdateServiceAccount)

			r.Post("/serviceAccounts/{email}/keys", app.CreateSAKey)
			r.Get("/serviceAccounts/{email}/keys", app.ListSAKeys)
			r.Get("/serviceAccounts/{email}/keys/{keyId}", app.GetSAKey)
			r.Delete("/serviceAccounts/{email}/keys/{keyId}", app.DeleteSAKey)

			r.Post("/secrets", app.CreateSecret)
			r.Get("/secrets", app.ListSecrets)
			r.Get("/secrets/{secret}", app.GetSecret)
			r.Delete("/secrets/{secret}", app.DeleteSecret)
			r.Patch("/secrets/{secret}", app.UpdateSecret)
			r.Post("/secrets/{secret}:addVersion", app.CreateSecretVersion)
			r.Get("/secrets/{secret}/versions", app.ListSecretVersions)
			r.Get("/secrets/{secret}/versions/{version}", app.GetSecretVersion)
			r.Get("/secrets/{secret}/versions/{version}:access", app.AccessSecretVersion)
			r.Post("/secrets/{secret}/versions/{version}:destroy", app.DestroySecretVersion)
			r.Post("/secrets/{secret}/versions/{version}:enable", app.EnableSecretVersion)
			r.Post("/secrets/{secret}/versions/{version}:disable", app.DisableSecretVersion)

			r.Put("/topics/{topic}", app.CreateTopic)
			r.Get("/topics", app.ListTopics)
			r.Get("/topics/{topic}", app.GetTopic)
			r.Delete("/topics/{topic}", app.DeleteTopic)

			r.Put("/subscriptions/{subscription}", app.CreateSubscription)
			r.Get("/subscriptions", app.ListSubscriptions)
			r.Get("/subscriptions/{subscription}", app.GetSubscription)
			r.Patch("/subscriptions/{subscription}", app.UpdateSubscription)
			r.Delete("/subscriptions/{subscription}", app.DeleteSubscription)

			// Service Usage API stub (M70). terraform-provider-google's
			// google_project_service resource Lists + Reads the
			// services-enabled state and would normally hit
			// serviceusage.googleapis.com. fakegcp doesn't model
			// per-service enable/disable — we treat every service as
			// "always enabled" so apply succeeds without a feedback
			// retry iteration. Real Service Usage returns much more
			// metadata; we emit the minimum the provider's Read flow
			// inspects (name + state).
			r.Get("/services", app.ListProjectServices)
			r.Get("/services/{service}", app.GetProjectService)
			r.Post("/services/{service}:enable", app.EnableProjectService)
			r.Post("/services/{service}:disable", app.DisableProjectService)
			r.Post("/services:batchEnable", app.BatchEnableProjectServices)
		})

		// Service Usage operation polling endpoint (M70). The provider
		// GETs operations/<name> after every enable/disable to confirm
		// completion — without this route, the poll 501s and the
		// provider hangs until its 20m apply timeout. Mounted outside
		// the per-project group because the operation URL is
		// project-less (operations live in a global namespace).
		r.Get("/v1/operations/{name}", app.GetServiceUsageOperation)

		// Service Networking (private services access). The provider's
		// google_service_networking_connection Create POSTs to
		// /v1/services/{service}/connections; Read List-filters by
		// network on the same URL; Patch + DeleteConnection (POST verb)
		// target /v1/services/{service}/connections/{connection}.
		// gcp-cloud-sql + gcp-full-stack both wire this resource for
		// private-IP SQL — without these routes the apply 401s
		// against the real servicenetworking.googleapis.com.
		r.Route("/v1/services/{service}/connections", func(r chi.Router) {
			r.Post("/", app.CreateServiceNetworkingConnection)
			r.Get("/", app.ListServiceNetworkingConnections)
			r.Patch("/{connection}", app.PatchServiceNetworkingConnection)
			r.Post("/{connection}", app.DeleteServiceNetworkingConnection)
		})

		// DNS routes — registered at two prefixes so both
		// terraform-provider-google call patterns land on the same
		// handlers:
		//
		// 1) `/dns/v1/projects/{project}` — used by the generated
		//    google-api-go-client dns/v1 lib (Changes.Create,
		//    ManagedZones.Get, ResourceRecordSets.List, the change
		//    waiter). Path comes from googleapi.ResolveRelative on
		//    the lib's "dns/v1/projects/..." relative URLs.
		//
		// 2) `/projects/{project}` — used by zone CRUD which builds
		//    URLs directly via {{DNSBasePath}}projects/{project}/...
		//    (resource_dns_managed_zone.go). With dns_custom_endpoint
		//    set to host-only (the only value that keeps the lib's
		//    NewDnsClient strip+ReplaceAll from corrupting the port),
		//    this is where zone CRUD now lands.
		registerDNSRoutes := func(r chi.Router) {
			r.Post("/managedZones", app.CreateDNSZone)
			r.Get("/managedZones", app.ListDNSZones)
			r.Get("/managedZones/{zone}", app.GetDNSZone)
			r.Patch("/managedZones/{zone}", app.UpdateDNSZone)
			r.Put("/managedZones/{zone}", app.UpdateDNSZone)
			r.Delete("/managedZones/{zone}", app.DeleteDNSZone)

			r.Post("/managedZones/{zone}/rrsets", app.CreateDNSRecordSet)
			r.Get("/managedZones/{zone}/rrsets", app.ListDNSRecordSets)
			r.Get("/managedZones/{zone}/rrsets/{name}/{type}", app.GetDNSRecordSet)
			r.Delete("/managedZones/{zone}/rrsets/{name}/{type}", app.DeleteDNSRecordSet)

			// google_dns_record_set mutates rrsets via the
			// transactional changes API rather than addressing rrsets
			// directly. Route those calls through to the rrset store.
			r.Post("/managedZones/{zone}/changes", app.CreateDNSChange)
			r.Get("/managedZones/{zone}/changes/{change}", app.GetDNSChange)
		}
		r.Route("/dns/v1/projects/{project}", registerDNSRoutes)
		r.Route("/projects/{project}", registerDNSRoutes)

		// Cloud Run v2
		r.Route("/v2/projects/{project}/locations/{location}", func(r chi.Router) {
			r.Post("/services", app.CreateCloudRunService)
			r.Get("/services", app.ListCloudRunServices)
			r.Get("/services/{service}", app.GetCloudRunService)
			r.Patch("/services/{service}", app.UpdateCloudRunService)
			r.Delete("/services/{service}", app.DeleteCloudRunService)
		})

		// Memorystore (Redis) v1 — google_redis_instance backing.
		// Shares the /v1/projects/{project}/locations/{location}
		// prefix with Cloud SQL but on a different per-collection
		// path so chi routes them independently when
		// redis_custom_endpoint points the provider here.
		r.Route("/v1/projects/{project}/locations/{location}/instances", func(r chi.Router) {
			r.Post("/", app.CreateRedisInstance)
			r.Get("/", app.ListRedisInstances)
			r.Get("/{instance}", app.GetRedisInstance)
			r.Patch("/{instance}", app.UpdateRedisInstance)
			r.Delete("/{instance}", app.DeleteRedisInstance)
		})

		// Storage
		r.Route("/storage/v1", func(r chi.Router) {
			r.Get("/b", app.ListBuckets)
			r.Post("/b", app.CreateBucket)
			r.Get("/b/{bucket}", app.GetBucket)
			r.Delete("/b/{bucket}", app.DeleteBucket)
			r.Patch("/b/{bucket}", app.UpdateBucket)
		})
	})

	r.NotFound(app.Unimplemented)
	r.MethodNotAllowed(app.Unimplemented)
}

func (app *Application) Unimplemented(w http.ResponseWriter, r *http.Request) {
	log.Printf("UNIMPLEMENTED: %s %s", r.Method, r.URL.Path)
	writeGCPError(w, 501, fmt.Sprintf("Not implemented: %s %s", r.Method, r.URL.Path), "notImplemented")
}
