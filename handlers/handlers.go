package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/redscaresu/fakegcp/models"
	"github.com/redscaresu/fakegcp/repository"
)

type Application struct {
	repo *repository.Repository
}

func NewApplication(repo *repository.Repository) *Application {
	return &Application{repo: repo}
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
	case errors.Is(err, models.ErrConflict):
		writeGCPError(w, http.StatusConflict, "The resource has dependents and cannot be deleted", "resourceInUseByAnotherResource")
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

				r.Get("/healthChecks", app.ListHealthChecks)
				r.Post("/healthChecks", app.CreateHealthCheck)
				r.Get("/healthChecks/{name}", app.GetHealthCheck)
				r.Patch("/healthChecks/{name}", app.UpdateHealthCheck)
				r.Delete("/healthChecks/{name}", app.DeleteHealthCheck)

				r.Get("/backendServices", app.ListBackendServices)
				r.Post("/backendServices", app.CreateBackendService)
				r.Get("/backendServices/{name}", app.GetBackendService)
				r.Patch("/backendServices/{name}", app.UpdateBackendService)
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

				r.Get("/operations/{name}", app.GetGlobalOperation)
			})

			// Zonal resources
			r.Route("/zones/{zone}", func(r chi.Router) {
				r.Get("/instances", app.ListInstances)
				r.Post("/instances", app.CreateInstance)
				r.Get("/instances/{name}", app.GetInstance)
				r.Delete("/instances/{name}", app.DeleteInstance)

				r.Get("/disks", app.ListDisks)
				r.Post("/disks", app.CreateDisk)
				r.Get("/disks/{name}", app.GetDisk)
				r.Delete("/disks/{name}", app.DeleteDisk)

				r.Get("/operations/{name}", app.GetZoneOperation)
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

		// IAM
		r.Route("/v1/projects/{project}", func(r chi.Router) {
			r.Get("/serviceAccounts", app.ListServiceAccounts)
			r.Post("/serviceAccounts", app.CreateServiceAccount)
			r.Get("/serviceAccounts/{email}", app.GetServiceAccount)
			r.Delete("/serviceAccounts/{email}", app.DeleteServiceAccount)

			r.Post("/serviceAccounts/{email}/keys", app.CreateSAKey)
			r.Get("/serviceAccounts/{email}/keys", app.ListSAKeys)
			r.Get("/serviceAccounts/{email}/keys/{keyId}", app.GetSAKey)
			r.Delete("/serviceAccounts/{email}/keys/{keyId}", app.DeleteSAKey)

			r.Post("/secrets", app.CreateSecret)
			r.Get("/secrets", app.ListSecrets)
			r.Get("/secrets/{secret}", app.GetSecret)
			r.Delete("/secrets/{secret}", app.DeleteSecret)
			r.Post("/secrets/{secret}:addVersion", app.CreateSecretVersion)
			r.Get("/secrets/{secret}/versions", app.ListSecretVersions)
			r.Get("/secrets/{secret}/versions/{version}", app.GetSecretVersion)
			r.Post("/secrets/{secret}/versions/{version}:destroy", app.DestroySecretVersion)

			r.Put("/topics/{topic}", app.CreateTopic)
			r.Get("/topics", app.ListTopics)
			r.Get("/topics/{topic}", app.GetTopic)
			r.Delete("/topics/{topic}", app.DeleteTopic)

			r.Put("/subscriptions/{subscription}", app.CreateSubscription)
			r.Get("/subscriptions", app.ListSubscriptions)
			r.Get("/subscriptions/{subscription}", app.GetSubscription)
			r.Patch("/subscriptions/{subscription}", app.UpdateSubscription)
			r.Delete("/subscriptions/{subscription}", app.DeleteSubscription)
		})

		// DNS
		r.Route("/dns/v1/projects/{project}", func(r chi.Router) {
			r.Post("/managedZones", app.CreateDNSZone)
			r.Get("/managedZones", app.ListDNSZones)
			r.Get("/managedZones/{zone}", app.GetDNSZone)
			r.Patch("/managedZones/{zone}", app.UpdateDNSZone)
			r.Delete("/managedZones/{zone}", app.DeleteDNSZone)

			r.Post("/managedZones/{zone}/rrsets", app.CreateDNSRecordSet)
			r.Get("/managedZones/{zone}/rrsets", app.ListDNSRecordSets)
			r.Get("/managedZones/{zone}/rrsets/{name}/{type}", app.GetDNSRecordSet)
			r.Delete("/managedZones/{zone}/rrsets/{name}/{type}", app.DeleteDNSRecordSet)
		})

		// Cloud Run v2
		r.Route("/v2/projects/{project}/locations/{location}", func(r chi.Router) {
			r.Post("/services", app.CreateCloudRunService)
			r.Get("/services", app.ListCloudRunServices)
			r.Get("/services/{service}", app.GetCloudRunService)
			r.Patch("/services/{service}", app.UpdateCloudRunService)
			r.Delete("/services/{service}", app.DeleteCloudRunService)
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
