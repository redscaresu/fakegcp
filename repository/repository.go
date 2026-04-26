package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redscaresu/fakegcp/models"
	_ "modernc.org/sqlite"
)

type Repository struct {
	db *sql.DB
}

func New(dbPath string) (*Repository, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, err
	}

	r := &Repository{db: db}
	if err := r.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return r, nil
}

func (r *Repository) migrate() error {
	schema := []string{
		`CREATE TABLE IF NOT EXISTS compute_networks (
			name TEXT NOT NULL, project TEXT NOT NULL, data TEXT NOT NULL,
			PRIMARY KEY (project, name)
		)`,
		`CREATE TABLE IF NOT EXISTS compute_subnetworks (
			name TEXT NOT NULL, project TEXT NOT NULL, region TEXT NOT NULL,
			network_name TEXT NOT NULL, data TEXT NOT NULL,
			PRIMARY KEY (project, region, name),
			FOREIGN KEY (project, network_name) REFERENCES compute_networks(project, name)
		)`,
		`CREATE TABLE IF NOT EXISTS compute_firewalls (
			name TEXT NOT NULL, project TEXT NOT NULL, network_name TEXT NOT NULL,
			data TEXT NOT NULL, PRIMARY KEY (project, name),
			FOREIGN KEY (project, network_name) REFERENCES compute_networks(project, name)
		)`,
		`CREATE TABLE IF NOT EXISTS compute_disks (
			name TEXT NOT NULL, project TEXT NOT NULL, zone TEXT NOT NULL,
			data TEXT NOT NULL, PRIMARY KEY (project, zone, name)
		)`,
		`CREATE TABLE IF NOT EXISTS compute_instances (
			name TEXT NOT NULL, project TEXT NOT NULL, zone TEXT NOT NULL,
			data TEXT NOT NULL, PRIMARY KEY (project, zone, name)
		)`,
		`CREATE TABLE IF NOT EXISTS compute_addresses (
			name TEXT NOT NULL, project TEXT NOT NULL, region TEXT NOT NULL,
			data TEXT NOT NULL, PRIMARY KEY (project, region, name)
		)`,
		`CREATE TABLE IF NOT EXISTS operations (
			name TEXT NOT NULL, project TEXT NOT NULL, zone TEXT DEFAULT NULL,
			region TEXT DEFAULT NULL, data TEXT NOT NULL,
			PRIMARY KEY (project, name)
		)`,
		`CREATE TABLE IF NOT EXISTS container_clusters (
			name TEXT NOT NULL, project TEXT NOT NULL, location TEXT NOT NULL,
			data TEXT NOT NULL, PRIMARY KEY (project, location, name)
		)`,
		`CREATE TABLE IF NOT EXISTS container_node_pools (
			name TEXT NOT NULL, project TEXT NOT NULL, location TEXT NOT NULL,
			cluster_name TEXT NOT NULL, data TEXT NOT NULL,
			PRIMARY KEY (project, location, cluster_name, name),
			FOREIGN KEY (project, location, cluster_name) REFERENCES container_clusters(project, location, name) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS sql_instances (
			name TEXT NOT NULL, project TEXT NOT NULL, data TEXT NOT NULL,
			PRIMARY KEY (project, name)
		)`,
		`CREATE TABLE IF NOT EXISTS sql_databases (
			name TEXT NOT NULL, project TEXT NOT NULL, instance_name TEXT NOT NULL,
			data TEXT NOT NULL, PRIMARY KEY (project, instance_name, name),
			FOREIGN KEY (project, instance_name) REFERENCES sql_instances(project, name) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS sql_users (
			name TEXT NOT NULL, project TEXT NOT NULL, instance_name TEXT NOT NULL,
			data TEXT NOT NULL, PRIMARY KEY (project, instance_name, name),
			FOREIGN KEY (project, instance_name) REFERENCES sql_instances(project, name) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS iam_service_accounts (
			unique_id TEXT NOT NULL, project TEXT NOT NULL, email TEXT NOT NULL UNIQUE,
			data TEXT NOT NULL, PRIMARY KEY (project, unique_id)
		)`,
		`CREATE TABLE IF NOT EXISTS iam_sa_keys (
			name TEXT NOT NULL PRIMARY KEY, project TEXT NOT NULL,
			service_account_email TEXT NOT NULL, data TEXT NOT NULL,
			FOREIGN KEY (service_account_email) REFERENCES iam_service_accounts(email) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS storage_buckets (
			name TEXT NOT NULL PRIMARY KEY, project TEXT NOT NULL,
			location TEXT NOT NULL, data TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS iam_bindings (
			project TEXT NOT NULL,
			role TEXT NOT NULL,
			member TEXT NOT NULL,
			data TEXT NOT NULL,
			PRIMARY KEY (project, role, member)
		)`,
		`CREATE TABLE IF NOT EXISTS compute_routers (
			name TEXT NOT NULL, project TEXT NOT NULL, region TEXT NOT NULL,
			network_name TEXT NOT NULL, data TEXT NOT NULL,
			PRIMARY KEY (project, region, name),
			FOREIGN KEY (project, network_name) REFERENCES compute_networks(project, name)
		)`,
		`CREATE TABLE IF NOT EXISTS compute_router_nats (
			name TEXT NOT NULL, project TEXT NOT NULL, region TEXT NOT NULL,
			router_name TEXT NOT NULL, data TEXT NOT NULL,
			PRIMARY KEY (project, region, router_name, name),
			FOREIGN KEY (project, region, router_name) REFERENCES compute_routers(project, region, name) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS dns_managed_zones (
			name TEXT NOT NULL, project TEXT NOT NULL, data TEXT NOT NULL,
			PRIMARY KEY (project, name)
		)`,
		`CREATE TABLE IF NOT EXISTS dns_record_sets (
			name TEXT NOT NULL, project TEXT NOT NULL, managed_zone TEXT NOT NULL,
			type TEXT NOT NULL, data TEXT NOT NULL,
			PRIMARY KEY (project, managed_zone, name, type),
			FOREIGN KEY (project, managed_zone) REFERENCES dns_managed_zones(project, name) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS compute_global_addresses (
			name TEXT NOT NULL, project TEXT NOT NULL, data TEXT NOT NULL,
			PRIMARY KEY (project, name)
		)`,
		`CREATE TABLE IF NOT EXISTS compute_health_checks (
			name TEXT NOT NULL, project TEXT NOT NULL, data TEXT NOT NULL,
			PRIMARY KEY (project, name)
		)`,
		`CREATE TABLE IF NOT EXISTS compute_backend_services (
			name TEXT NOT NULL, project TEXT NOT NULL, data TEXT NOT NULL,
			PRIMARY KEY (project, name)
		)`,
		`CREATE TABLE IF NOT EXISTS compute_ssl_certificates (
			name TEXT NOT NULL, project TEXT NOT NULL, data TEXT NOT NULL,
			PRIMARY KEY (project, name)
		)`,
		`CREATE TABLE IF NOT EXISTS compute_target_https_proxies (
			name TEXT NOT NULL, project TEXT NOT NULL, data TEXT NOT NULL,
			PRIMARY KEY (project, name)
		)`,
		`CREATE TABLE IF NOT EXISTS compute_url_maps (
			name TEXT NOT NULL, project TEXT NOT NULL, data TEXT NOT NULL,
			PRIMARY KEY (project, name)
		)`,
		`CREATE TABLE IF NOT EXISTS compute_global_forwarding_rules (
			name TEXT NOT NULL, project TEXT NOT NULL, data TEXT NOT NULL,
			PRIMARY KEY (project, name)
		)`,
		`CREATE TABLE IF NOT EXISTS secretmanager_secrets (
			name TEXT NOT NULL, project TEXT NOT NULL, data TEXT NOT NULL,
			PRIMARY KEY (project, name)
		)`,
		`CREATE TABLE IF NOT EXISTS secretmanager_versions (
			name TEXT NOT NULL, project TEXT NOT NULL, secret_name TEXT NOT NULL,
			data TEXT NOT NULL, PRIMARY KEY (name),
			FOREIGN KEY (project, secret_name) REFERENCES secretmanager_secrets(project, name) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS pubsub_topics (
			name TEXT NOT NULL, project TEXT NOT NULL, data TEXT NOT NULL,
			PRIMARY KEY (project, name)
		)`,
		`CREATE TABLE IF NOT EXISTS pubsub_subscriptions (
			name TEXT NOT NULL, project TEXT NOT NULL, topic_name TEXT,
			data TEXT NOT NULL, PRIMARY KEY (project, name)
		)`,
		`CREATE TABLE IF NOT EXISTS cloudrun_services (
			name TEXT NOT NULL, project TEXT NOT NULL, location TEXT NOT NULL,
			data TEXT NOT NULL, PRIMARY KEY (project, location, name)
		)`,
	}

	for _, stmt := range schema {
		if _, err := r.db.Exec(stmt); err != nil {
			return err
		}
	}

	// Migration v1: rebuild tables with corrected FK actions for existing DBs
	var version int
	_ = r.db.QueryRow(`PRAGMA user_version`).Scan(&version)
	if version < 1 {
		migrations := []string{
			// Rebuild iam_sa_keys with ON DELETE CASCADE
			`CREATE TABLE IF NOT EXISTS iam_sa_keys_new (
				name TEXT NOT NULL PRIMARY KEY, project TEXT NOT NULL,
				service_account_email TEXT NOT NULL, data TEXT NOT NULL,
				FOREIGN KEY (service_account_email) REFERENCES iam_service_accounts(email) ON DELETE CASCADE
			)`,
			`INSERT OR IGNORE INTO iam_sa_keys_new SELECT * FROM iam_sa_keys`,
			`DROP TABLE IF EXISTS iam_sa_keys`,
			`ALTER TABLE iam_sa_keys_new RENAME TO iam_sa_keys`,
			// Rebuild pubsub_subscriptions without topic FK (subscriptions survive topic deletion)
			`CREATE TABLE IF NOT EXISTS pubsub_subscriptions_new (
				name TEXT NOT NULL, project TEXT NOT NULL, topic_name TEXT,
				data TEXT NOT NULL, PRIMARY KEY (project, name)
			)`,
			`INSERT OR IGNORE INTO pubsub_subscriptions_new SELECT * FROM pubsub_subscriptions`,
			`DROP TABLE IF EXISTS pubsub_subscriptions`,
			`ALTER TABLE pubsub_subscriptions_new RENAME TO pubsub_subscriptions`,
			`PRAGMA user_version = 1`,
		}
		for _, stmt := range migrations {
			if _, err := r.db.Exec(stmt); err != nil {
				return err
			}
		}
	}

	return nil
}

func marshalData(data map[string]any) ([]byte, error) {
	return json.Marshal(data)
}

func unmarshalData(raw []byte) (map[string]any, error) {
	out := map[string]any{}
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func mapInsertError(err error) error {
	if err == nil {
		return nil
	}
	s := strings.ToUpper(err.Error())
	if strings.Contains(s, "FOREIGN KEY") {
		return models.ErrNotFound
	}
	if strings.Contains(s, "UNIQUE") || strings.Contains(s, "PRIMARY KEY") {
		return models.ErrAlreadyExists
	}
	return err
}

func mapDeleteError(err error) error {
	if err == nil {
		return nil
	}
	s := strings.ToUpper(err.Error())
	if strings.Contains(s, "FOREIGN KEY") {
		return models.ErrConflict
	}
	return err
}

func newID() string {
	return uuid.NewString()
}

func numericID() string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	buf := make([]byte, 19)
	buf[0] = byte('1' + r.Intn(9))
	for i := 1; i < len(buf); i++ {
		buf[i] = byte('0' + r.Intn(10))
	}
	return string(buf)
}

func nowRFC3339() string {
	return time.Now().Format(time.RFC3339)
}

func patchMerge(current, patch map[string]any, skip ...string) map[string]any {
	skipSet := map[string]struct{}{}
	for _, k := range skip {
		skipSet[k] = struct{}{}
	}

	out := map[string]any{}
	for k, v := range current {
		if mv, ok := toMap(v); ok {
			c := map[string]any{}
			for mk, mvv := range mv {
				c[mk] = mvv
			}
			out[k] = c
			continue
		}
		out[k] = v
	}

	for k, v := range patch {
		if _, found := skipSet[k]; found {
			continue
		}
		if v == nil {
			continue
		}

		pv, pok := toMap(v)
		cv, cok := toMap(out[k])
		if pok && cok {
			merged := map[string]any{}
			for ck, cvv := range cv {
				merged[ck] = cvv
			}
			for pk, pvv := range pv {
				if pvv == nil {
					continue
				}
				merged[pk] = pvv
			}
			out[k] = merged
			continue
		}

		out[k] = v
	}

	return out
}

func extractNameFromSelfLink(selfLink string) string {
	trimmed := strings.TrimSpace(selfLink)
	trimmed = strings.TrimSuffix(trimmed, "/")
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "/")
	return parts[len(parts)-1]
}

func toMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	if ok {
		return m, true
	}
	return nil, false
}

func getString(data map[string]any, key string) string {
	if v, ok := data[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (r *Repository) loadOne(query string, args ...any) (map[string]any, error) {
	var raw string
	if err := r.db.QueryRow(query, args...).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return nil, models.ErrNotFound
		}
		return nil, err
	}
	return unmarshalData([]byte(raw))
}

func (r *Repository) loadMany(query string, args ...any) ([]map[string]any, error) {
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		item, err := unmarshalData([]byte(raw))
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) deleteWithResult(query string, args ...any) error {
	res, err := r.db.Exec(query, args...)
	if err != nil {
		if mapped := mapDeleteError(err); mapped != err {
			return mapped
		}
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return models.ErrNotFound
	}
	return nil
}

func (r *Repository) dbFilePath() (string, error) {
	rows, err := r.db.Query("PRAGMA database_list")
	if err != nil {
		return "", err
	}
	defer rows.Close()

	for rows.Next() {
		var seq int
		var name, file string
		if err := rows.Scan(&seq, &name, &file); err != nil {
			return "", err
		}
		if name == "main" {
			if file == "" {
				return "", fmt.Errorf("main database has no file path")
			}
			return file, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("main database not found")
}

func (r *Repository) CreateNetwork(project string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	data["name"] = name
	data["project"] = project
	if getString(data, "id") == "" {
		data["id"] = numericID()
	}
	if getString(data, "creationTimestamp") == "" {
		data["creationTimestamp"] = nowRFC3339()
	}

	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}

	_, err = r.db.Exec(`INSERT INTO compute_networks (name, project, data) VALUES (?, ?, ?)`, name, project, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetNetwork(project, name)
}

func (r *Repository) GetNetwork(project, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM compute_networks WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) ListNetworks(project string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM compute_networks WHERE project = ? ORDER BY name`, project)
}

func (r *Repository) UpdateNetwork(project, name string, patch map[string]any) (map[string]any, error) {
	current, err := r.GetNetwork(project, name)
	if err != nil {
		return nil, err
	}
	merged := patchMerge(current, patch, "name", "project")
	merged["name"] = name
	merged["project"] = project

	raw, err := marshalData(merged)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`UPDATE compute_networks SET data = ? WHERE project = ? AND name = ?`, string(raw), project, name)
	if err != nil {
		return nil, err
	}
	return merged, nil
}

func (r *Repository) DeleteNetwork(project, name string) error {
	return r.deleteWithResult(`DELETE FROM compute_networks WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) CreateSubnetwork(project, region string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	networkName := getString(data, "network_name")
	if networkName == "" {
		networkName = extractNameFromSelfLink(getString(data, "network"))
	}
	if networkName == "" {
		return nil, fmt.Errorf("network reference is required")
	}

	data["name"] = name
	data["project"] = project
	data["region"] = region
	data["network_name"] = networkName
	if getString(data, "id") == "" {
		data["id"] = numericID()
	}
	if getString(data, "creationTimestamp") == "" {
		data["creationTimestamp"] = nowRFC3339()
	}

	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO compute_subnetworks (name, project, region, network_name, data) VALUES (?, ?, ?, ?, ?)`, name, project, region, networkName, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetSubnetwork(project, region, name)
}

func (r *Repository) GetSubnetwork(project, region, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM compute_subnetworks WHERE project = ? AND region = ? AND name = ?`, project, region, name)
}

func (r *Repository) ListSubnetworks(project, region string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM compute_subnetworks WHERE project = ? AND region = ? ORDER BY name`, project, region)
}

func (r *Repository) UpdateSubnetwork(project, region, name string, patch map[string]any) (map[string]any, error) {
	current, err := r.GetSubnetwork(project, region, name)
	if err != nil {
		return nil, err
	}
	merged := patchMerge(current, patch, "name", "project", "region", "network_name")
	merged["name"] = name
	merged["project"] = project
	merged["region"] = region

	networkName := getString(merged, "network_name")
	if networkName == "" {
		networkName = extractNameFromSelfLink(getString(merged, "network"))
	}
	if networkName == "" {
		return nil, fmt.Errorf("network reference is required")
	}
	merged["network_name"] = networkName

	raw, err := marshalData(merged)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`UPDATE compute_subnetworks SET network_name = ?, data = ? WHERE project = ? AND region = ? AND name = ?`, networkName, string(raw), project, region, name)
	if err != nil {
		return nil, mapInsertError(err)
	}
	return merged, nil
}

func (r *Repository) DeleteSubnetwork(project, region, name string) error {
	return r.deleteWithResult(`DELETE FROM compute_subnetworks WHERE project = ? AND region = ? AND name = ?`, project, region, name)
}

func (r *Repository) CreateFirewall(project string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	networkName := getString(data, "network_name")
	if networkName == "" {
		networkName = extractNameFromSelfLink(getString(data, "network"))
	}
	if networkName == "" {
		return nil, fmt.Errorf("network reference is required")
	}

	data["name"] = name
	data["project"] = project
	data["network_name"] = networkName
	if getString(data, "id") == "" {
		data["id"] = numericID()
	}
	if getString(data, "creationTimestamp") == "" {
		data["creationTimestamp"] = nowRFC3339()
	}

	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO compute_firewalls (name, project, network_name, data) VALUES (?, ?, ?, ?)`, name, project, networkName, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetFirewall(project, name)
}

func (r *Repository) GetFirewall(project, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM compute_firewalls WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) ListFirewalls(project string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM compute_firewalls WHERE project = ? ORDER BY name`, project)
}

func (r *Repository) UpdateFirewall(project, name string, patch map[string]any) (map[string]any, error) {
	current, err := r.GetFirewall(project, name)
	if err != nil {
		return nil, err
	}
	merged := patchMerge(current, patch, "name", "project", "network_name")
	merged["name"] = name
	merged["project"] = project

	networkName := getString(merged, "network_name")
	if networkName == "" {
		networkName = extractNameFromSelfLink(getString(merged, "network"))
	}
	if networkName == "" {
		return nil, fmt.Errorf("network reference is required")
	}
	merged["network_name"] = networkName

	raw, err := marshalData(merged)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`UPDATE compute_firewalls SET network_name = ?, data = ? WHERE project = ? AND name = ?`, networkName, string(raw), project, name)
	if err != nil {
		return nil, mapInsertError(err)
	}
	return merged, nil
}

func (r *Repository) DeleteFirewall(project, name string) error {
	return r.deleteWithResult(`DELETE FROM compute_firewalls WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) CreateDisk(project, zone string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	data["name"] = name
	data["project"] = project
	data["zone"] = zone
	if getString(data, "id") == "" {
		data["id"] = numericID()
	}
	if getString(data, "creationTimestamp") == "" {
		data["creationTimestamp"] = nowRFC3339()
	}

	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO compute_disks (name, project, zone, data) VALUES (?, ?, ?, ?)`, name, project, zone, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetDisk(project, zone, name)
}

func (r *Repository) GetDisk(project, zone, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM compute_disks WHERE project = ? AND zone = ? AND name = ?`, project, zone, name)
}

func (r *Repository) ListDisks(project, zone string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM compute_disks WHERE project = ? AND zone = ? ORDER BY name`, project, zone)
}

func (r *Repository) DeleteDisk(project, zone, name string) error {
	return r.deleteWithResult(`DELETE FROM compute_disks WHERE project = ? AND zone = ? AND name = ?`, project, zone, name)
}

func (r *Repository) CreateInstance(project, zone string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	data["name"] = name
	data["project"] = project
	data["zone"] = zone
	if getString(data, "id") == "" {
		data["id"] = numericID()
	}
	if getString(data, "creationTimestamp") == "" {
		data["creationTimestamp"] = nowRFC3339()
	}

	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO compute_instances (name, project, zone, data) VALUES (?, ?, ?, ?)`, name, project, zone, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetInstance(project, zone, name)
}

func (r *Repository) GetInstance(project, zone, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM compute_instances WHERE project = ? AND zone = ? AND name = ?`, project, zone, name)
}

func (r *Repository) ListInstances(project, zone string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM compute_instances WHERE project = ? AND zone = ? ORDER BY name`, project, zone)
}

func (r *Repository) DeleteInstance(project, zone, name string) error {
	return r.deleteWithResult(`DELETE FROM compute_instances WHERE project = ? AND zone = ? AND name = ?`, project, zone, name)
}

func (r *Repository) CreateAddress(project, region string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	data["name"] = name
	data["project"] = project
	data["region"] = region
	if getString(data, "id") == "" {
		data["id"] = numericID()
	}
	if getString(data, "creationTimestamp") == "" {
		data["creationTimestamp"] = nowRFC3339()
	}

	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO compute_addresses (name, project, region, data) VALUES (?, ?, ?, ?)`, name, project, region, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetAddress(project, region, name)
}

func (r *Repository) GetAddress(project, region, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM compute_addresses WHERE project = ? AND region = ? AND name = ?`, project, region, name)
}

func (r *Repository) ListAddresses(project, region string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM compute_addresses WHERE project = ? AND region = ? ORDER BY name`, project, region)
}

func (r *Repository) DeleteAddress(project, region, name string) error {
	return r.deleteWithResult(`DELETE FROM compute_addresses WHERE project = ? AND region = ? AND name = ?`, project, region, name)
}

func (r *Repository) StoreOperation(project, zone, region, name string, data map[string]any) error {
	if name == "" {
		name = newID()
	}
	data["name"] = name
	data["project"] = project
	if getString(data, "id") == "" {
		data["id"] = numericID()
	}
	if getString(data, "insertTime") == "" {
		data["insertTime"] = nowRFC3339()
	}
	raw, err := marshalData(data)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`INSERT INTO operations (name, project, zone, region, data) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(project, name) DO UPDATE SET zone = excluded.zone, region = excluded.region, data = excluded.data`, name, project, nullIfEmpty(zone), nullIfEmpty(region), string(raw))
	if err != nil {
		return err
	}
	return nil
}

func (r *Repository) GetOperation(project, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM operations WHERE project = ? AND name = ?`, project, name)
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func (r *Repository) CreateCluster(project, location string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	data["name"] = name
	data["project"] = project
	data["location"] = location
	if getString(data, "id") == "" {
		data["id"] = numericID()
	}
	if getString(data, "createTime") == "" {
		data["createTime"] = nowRFC3339()
	}
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO container_clusters (name, project, location, data) VALUES (?, ?, ?, ?)`, name, project, location, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetCluster(project, location, name)
}

func (r *Repository) GetCluster(project, location, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM container_clusters WHERE project = ? AND location = ? AND name = ?`, project, location, name)
}

func (r *Repository) ListClusters(project, location string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM container_clusters WHERE project = ? AND location = ? ORDER BY name`, project, location)
}

func (r *Repository) UpdateCluster(project, location, name string, patch map[string]any) (map[string]any, error) {
	current, err := r.GetCluster(project, location, name)
	if err != nil {
		return nil, err
	}
	merged := patchMerge(current, patch, "name", "project", "location")
	merged["name"] = name
	merged["project"] = project
	merged["location"] = location

	raw, err := marshalData(merged)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`UPDATE container_clusters SET data = ? WHERE project = ? AND location = ? AND name = ?`, string(raw), project, location, name)
	if err != nil {
		return nil, err
	}
	return merged, nil
}

func (r *Repository) DeleteCluster(project, location, name string) error {
	return r.deleteWithResult(`DELETE FROM container_clusters WHERE project = ? AND location = ? AND name = ?`, project, location, name)
}

func (r *Repository) CreateNodePool(project, location, clusterName string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	data["name"] = name
	data["project"] = project
	data["location"] = location
	data["cluster_name"] = clusterName
	if getString(data, "id") == "" {
		data["id"] = numericID()
	}
	if getString(data, "createTime") == "" {
		data["createTime"] = nowRFC3339()
	}
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO container_node_pools (name, project, location, cluster_name, data) VALUES (?, ?, ?, ?, ?)`, name, project, location, clusterName, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetNodePool(project, location, clusterName, name)
}

func (r *Repository) GetNodePool(project, location, clusterName, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM container_node_pools WHERE project = ? AND location = ? AND cluster_name = ? AND name = ?`, project, location, clusterName, name)
}

func (r *Repository) ListNodePools(project, location, clusterName string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM container_node_pools WHERE project = ? AND location = ? AND cluster_name = ? ORDER BY name`, project, location, clusterName)
}

func (r *Repository) DeleteNodePool(project, location, clusterName, name string) error {
	return r.deleteWithResult(`DELETE FROM container_node_pools WHERE project = ? AND location = ? AND cluster_name = ? AND name = ?`, project, location, clusterName, name)
}

func (r *Repository) CreateSQLInstance(project string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	data["name"] = name
	data["project"] = project
	if getString(data, "id") == "" {
		data["id"] = numericID()
	}
	if getString(data, "createTime") == "" {
		data["createTime"] = nowRFC3339()
	}
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO sql_instances (name, project, data) VALUES (?, ?, ?)`, name, project, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetSQLInstance(project, name)
}

func (r *Repository) GetSQLInstance(project, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM sql_instances WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) ListSQLInstances(project string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM sql_instances WHERE project = ? ORDER BY name`, project)
}

func (r *Repository) UpdateSQLInstance(project, name string, patch map[string]any) (map[string]any, error) {
	current, err := r.GetSQLInstance(project, name)
	if err != nil {
		return nil, err
	}
	merged := patchMerge(current, patch, "name", "project")
	merged["name"] = name
	merged["project"] = project
	raw, err := marshalData(merged)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`UPDATE sql_instances SET data = ? WHERE project = ? AND name = ?`, string(raw), project, name)
	if err != nil {
		return nil, err
	}
	return merged, nil
}

func (r *Repository) DeleteSQLInstance(project, name string) error {
	return r.deleteWithResult(`DELETE FROM sql_instances WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) CreateSQLDatabase(project, instanceName string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	data["name"] = name
	data["project"] = project
	data["instance_name"] = instanceName
	if getString(data, "id") == "" {
		data["id"] = numericID()
	}
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO sql_databases (name, project, instance_name, data) VALUES (?, ?, ?, ?)`, name, project, instanceName, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetSQLDatabase(project, instanceName, name)
}

func (r *Repository) GetSQLDatabase(project, instanceName, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM sql_databases WHERE project = ? AND instance_name = ? AND name = ?`, project, instanceName, name)
}

func (r *Repository) ListSQLDatabases(project, instanceName string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM sql_databases WHERE project = ? AND instance_name = ? ORDER BY name`, project, instanceName)
}

func (r *Repository) DeleteSQLDatabase(project, instanceName, name string) error {
	return r.deleteWithResult(`DELETE FROM sql_databases WHERE project = ? AND instance_name = ? AND name = ?`, project, instanceName, name)
}

func (r *Repository) CreateSQLUser(project, instanceName string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		name = getString(data, "user")
	}
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	data["name"] = name
	data["project"] = project
	data["instance_name"] = instanceName
	if getString(data, "id") == "" {
		data["id"] = numericID()
	}
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO sql_users (name, project, instance_name, data) VALUES (?, ?, ?, ?)`, name, project, instanceName, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.loadOne(`SELECT data FROM sql_users WHERE project = ? AND instance_name = ? AND name = ?`, project, instanceName, name)
}

func (r *Repository) ListSQLUsers(project, instanceName string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM sql_users WHERE project = ? AND instance_name = ? ORDER BY name`, project, instanceName)
}

func (r *Repository) UpdateSQLUser(project, instanceName, name string, patch map[string]any) (map[string]any, error) {
	current, err := r.loadOne(`SELECT data FROM sql_users WHERE project = ? AND instance_name = ? AND name = ?`, project, instanceName, name)
	if err != nil {
		return nil, err
	}
	merged := patchMerge(current, patch, "name", "project", "instance_name")
	merged["name"] = name
	merged["project"] = project
	merged["instance_name"] = instanceName
	body, err := marshalData(merged)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`UPDATE sql_users SET data = ? WHERE project = ? AND instance_name = ? AND name = ?`, string(body), project, instanceName, name)
	if err != nil {
		return nil, err
	}
	return merged, nil
}

func (r *Repository) DeleteSQLUser(project, instanceName, name string) error {
	return r.deleteWithResult(`DELETE FROM sql_users WHERE project = ? AND instance_name = ? AND name = ?`, project, instanceName, name)
}

func (r *Repository) CreateServiceAccount(project string, data map[string]any) (map[string]any, error) {
	accountID := getString(data, "accountId")
	if accountID == "" {
		accountID = getString(data, "name")
	}
	if accountID == "" {
		accountID = "sa-" + strings.ReplaceAll(newID()[:12], "-", "")
	}
	uniqueID := numericID()
	email := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", accountID, project)

	data["accountId"] = accountID
	data["project"] = project
	data["uniqueId"] = uniqueID
	data["email"] = email
	if getString(data, "name") == "" {
		data["name"] = fmt.Sprintf("projects/%s/serviceAccounts/%s", project, email)
	}

	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO iam_service_accounts (unique_id, project, email, data) VALUES (?, ?, ?, ?)`, uniqueID, project, email, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetServiceAccount(project, email)
}

func (r *Repository) GetServiceAccount(project, email string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM iam_service_accounts WHERE project = ? AND email = ?`, project, email)
}

func (r *Repository) ListServiceAccounts(project string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM iam_service_accounts WHERE project = ? ORDER BY email`, project)
}

func (r *Repository) DeleteServiceAccount(project, email string) error {
	return r.deleteWithResult(`DELETE FROM iam_service_accounts WHERE project = ? AND email = ?`, project, email)
}

func (r *Repository) CreateSAKey(project, serviceAccountEmail string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		name = fmt.Sprintf("projects/%s/serviceAccounts/%s/keys/%s", project, serviceAccountEmail, newID())
	}
	data["name"] = name
	data["project"] = project
	data["serviceAccountEmail"] = serviceAccountEmail
	if getString(data, "validAfterTime") == "" {
		data["validAfterTime"] = nowRFC3339()
	}
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO iam_sa_keys (name, project, service_account_email, data) VALUES (?, ?, ?, ?)`, name, project, serviceAccountEmail, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetSAKey(name)
}

func (r *Repository) GetSAKey(name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM iam_sa_keys WHERE name = ?`, name)
}

func (r *Repository) ListSAKeys(serviceAccountEmail string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM iam_sa_keys WHERE service_account_email = ? ORDER BY name`, serviceAccountEmail)
}

func (r *Repository) DeleteSAKey(name string) error {
	return r.deleteWithResult(`DELETE FROM iam_sa_keys WHERE name = ?`, name)
}

func (r *Repository) CreateBucket(project string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	location := getString(data, "location")
	if location == "" {
		location = "US"
	}
	data["name"] = name
	data["project"] = project
	data["location"] = location
	if getString(data, "id") == "" {
		data["id"] = name
	}
	if getString(data, "timeCreated") == "" {
		data["timeCreated"] = nowRFC3339()
	}
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO storage_buckets (name, project, location, data) VALUES (?, ?, ?, ?)`, name, project, location, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetBucket(name)
}

func (r *Repository) GetBucket(name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM storage_buckets WHERE name = ?`, name)
}

func (r *Repository) ListBuckets(project string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM storage_buckets WHERE project = ? ORDER BY name`, project)
}

func (r *Repository) UpdateBucket(name string, patch map[string]any) (map[string]any, error) {
	current, err := r.GetBucket(name)
	if err != nil {
		return nil, err
	}
	merged := patchMerge(current, patch, "name", "project")
	merged["name"] = name
	if p := getString(current, "project"); p != "" {
		merged["project"] = p
	}
	location := getString(merged, "location")
	if location == "" {
		location = "US"
		merged["location"] = location
	}
	raw, err := marshalData(merged)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`UPDATE storage_buckets SET location = ?, data = ? WHERE name = ?`, location, string(raw), name)
	if err != nil {
		return nil, err
	}
	return merged, nil
}

func (r *Repository) DeleteBucket(name string) error {
	return r.deleteWithResult(`DELETE FROM storage_buckets WHERE name = ?`, name)
}

func (r *Repository) Reset() error {
	tables := []string{
		"secretmanager_versions",
		"secretmanager_secrets",
		"pubsub_subscriptions",
		"pubsub_topics",
		"dns_record_sets",
		"dns_managed_zones",
		"compute_router_nats",
		"compute_routers",
		"cloudrun_services",
		"compute_global_forwarding_rules",
		"compute_target_https_proxies",
		"compute_url_maps",
		"compute_ssl_certificates",
		"compute_backend_services",
		"compute_health_checks",
		"compute_global_addresses",
		"iam_bindings",
		"iam_sa_keys",
		"iam_service_accounts",
		"container_node_pools",
		"container_clusters",
		"sql_databases",
		"sql_users",
		"sql_instances",
		"compute_subnetworks",
		"compute_firewalls",
		"compute_instances",
		"compute_disks",
		"compute_addresses",
		"compute_networks",
		"operations",
		"storage_buckets",
	}

	if _, err := r.db.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}
	defer r.db.Exec("PRAGMA foreign_keys = ON")

	// Resolve the snapshot path BEFORE truncating tables so a
	// dbFilePath() failure surfaces atomically — caller sees error
	// without state already wiped. Mirrors mockway's clearSnapshot
	// invariant.
	path, err := r.dbFilePath()
	if err != nil {
		return fmt.Errorf("resolve db path for snapshot clear: %w", err)
	}

	for _, table := range tables {
		if _, err := r.db.Exec("DELETE FROM " + table); err != nil {
			return err
		}
	}

	if _, err := r.db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return err
	}

	// Reset clears the snapshot baseline too so a subsequent Restore
	// cannot resurrect pre-reset state.
	if err := os.Remove(path + ".snapshot"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove snapshot baseline: %w", err)
	}
	return nil
}

func (r *Repository) Snapshot() error {
	path, err := r.dbFilePath()
	if err != nil {
		return err
	}
	snapshotPath := path + ".snapshot"

	if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
		return err
	}
	if _, err := r.db.Exec("PRAGMA wal_checkpoint(FULL)"); err != nil {
		return err
	}
	if _, err := r.db.Exec(fmt.Sprintf("VACUUM INTO '%s'", strings.ReplaceAll(snapshotPath, "'", "''"))); err != nil {
		return err
	}
	return nil
}

func (r *Repository) Restore() error {
	path, err := r.dbFilePath()
	if err != nil {
		return err
	}
	snapshotPath := path + ".snapshot"
	if _, err := os.Stat(snapshotPath); err != nil {
		if os.IsNotExist(err) {
			return models.ErrNotFound
		}
		return err
	}

	if err := r.db.Close(); err != nil {
		return err
	}
	if err := copyFile(snapshotPath, path); err != nil {
		// Reopen the original DB so the repository remains usable
		db, openErr := sql.Open("sqlite", path)
		if openErr == nil {
			db.SetMaxOpenConns(1)
			db.Exec("PRAGMA foreign_keys = ON")
			db.Exec("PRAGMA journal_mode = WAL")
			r.db = db
		}
		return err
	}
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return err
	}
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return err
	}

	r.db = db
	if err := r.migrate(); err != nil {
		db.Close()
		return err
	}

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmpDst := dst + ".tmp"
	out, err := os.OpenFile(tmpDst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := out.ReadFrom(in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}

	return os.Rename(tmpDst, dst)
}

func (r *Repository) FullState() (map[string]any, error) {
	compute, err := r.ServiceState("compute")
	if err != nil {
		return nil, err
	}
	container, err := r.ServiceState("container")
	if err != nil {
		return nil, err
	}
	sqlState, err := r.ServiceState("sql")
	if err != nil {
		return nil, err
	}
	iam, err := r.ServiceState("iam")
	if err != nil {
		return nil, err
	}
	storage, err := r.ServiceState("storage")
	if err != nil {
		return nil, err
	}
	operations, err := r.ServiceState("operations")
	if err != nil {
		return nil, err
	}
	dns, err := r.ServiceState("dns")
	if err != nil {
		return nil, err
	}
	lb, err := r.ServiceState("lb")
	if err != nil {
		return nil, err
	}
	secretManager, err := r.ServiceState("secretmanager")
	if err != nil {
		return nil, err
	}
	pubsub, err := r.ServiceState("pubsub")
	if err != nil {
		return nil, err
	}
	cloudRun, err := r.ServiceState("cloudrun")
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"compute":       compute,
		"container":     container,
		"sql":           sqlState,
		"iam":           iam,
		"storage":       storage,
		"operations":    operations,
		"dns":           dns,
		"lb":            lb,
		"secretmanager": secretManager,
		"pubsub":        pubsub,
		"cloudrun":      cloudRun,
	}, nil
}

func (r *Repository) ServiceState(service string) (map[string]any, error) {
	switch strings.ToLower(service) {
	case "compute":
		networks, err := r.loadMany(`SELECT data FROM compute_networks ORDER BY project, name`)
		if err != nil {
			return nil, err
		}
		subnetworks, err := r.loadMany(`SELECT data FROM compute_subnetworks ORDER BY project, region, name`)
		if err != nil {
			return nil, err
		}
		firewalls, err := r.loadMany(`SELECT data FROM compute_firewalls ORDER BY project, name`)
		if err != nil {
			return nil, err
		}
		disks, err := r.loadMany(`SELECT data FROM compute_disks ORDER BY project, zone, name`)
		if err != nil {
			return nil, err
		}
		instances, err := r.loadMany(`SELECT data FROM compute_instances ORDER BY project, zone, name`)
		if err != nil {
			return nil, err
		}
		addresses, err := r.loadMany(`SELECT data FROM compute_addresses ORDER BY project, region, name`)
		if err != nil {
			return nil, err
		}
		routers, err := r.loadMany(`SELECT data FROM compute_routers ORDER BY project, region, name`)
		if err != nil {
			return nil, err
		}
		routerNATs, err := r.loadMany(`SELECT data FROM compute_router_nats ORDER BY project, region, router_name, name`)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"networks":    networks,
			"subnetworks": subnetworks,
			"firewalls":   firewalls,
			"disks":       disks,
			"instances":   instances,
			"addresses":   addresses,
			"routers":     routers,
			"router_nats": routerNATs,
		}, nil
	case "container":
		clusters, err := r.loadMany(`SELECT data FROM container_clusters ORDER BY project, location, name`)
		if err != nil {
			return nil, err
		}
		nodePools, err := r.loadMany(`SELECT data FROM container_node_pools ORDER BY project, location, cluster_name, name`)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"clusters":  clusters,
			"nodePools": nodePools,
		}, nil
	case "sql":
		instances, err := r.loadMany(`SELECT data FROM sql_instances ORDER BY project, name`)
		if err != nil {
			return nil, err
		}
		databases, err := r.loadMany(`SELECT data FROM sql_databases ORDER BY project, instance_name, name`)
		if err != nil {
			return nil, err
		}
		users, err := r.loadMany(`SELECT data FROM sql_users ORDER BY project, instance_name, name`)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"instances": instances,
			"databases": databases,
			"users":     users,
		}, nil
	case "iam":
		serviceAccounts, err := r.loadMany(`SELECT data FROM iam_service_accounts ORDER BY project, email`)
		if err != nil {
			return nil, err
		}
		keys, err := r.loadMany(`SELECT data FROM iam_sa_keys ORDER BY service_account_email, name`)
		if err != nil {
			return nil, err
		}
		iamBindings, err := r.loadMany(`SELECT data FROM iam_bindings ORDER BY project, role, member`)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"serviceAccounts": serviceAccounts,
			"keys":            keys,
			"iam_bindings":    iamBindings,
		}, nil
	case "dns":
		zones, err := r.loadMany(`SELECT data FROM dns_managed_zones ORDER BY project, name`)
		if err != nil {
			return nil, err
		}
		recordSets, err := r.loadMany(`SELECT data FROM dns_record_sets ORDER BY project, managed_zone, name, type`)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"zones":       zones,
			"record_sets": recordSets,
		}, nil
	case "lb":
		globalAddresses, err := r.loadMany(`SELECT data FROM compute_global_addresses ORDER BY project, name`)
		if err != nil {
			return nil, err
		}
		healthChecks, err := r.loadMany(`SELECT data FROM compute_health_checks ORDER BY project, name`)
		if err != nil {
			return nil, err
		}
		backendServices, err := r.loadMany(`SELECT data FROM compute_backend_services ORDER BY project, name`)
		if err != nil {
			return nil, err
		}
		sslCertificates, err := r.loadMany(`SELECT data FROM compute_ssl_certificates ORDER BY project, name`)
		if err != nil {
			return nil, err
		}
		targetHTTPSProxies, err := r.loadMany(`SELECT data FROM compute_target_https_proxies ORDER BY project, name`)
		if err != nil {
			return nil, err
		}
		urlMaps, err := r.loadMany(`SELECT data FROM compute_url_maps ORDER BY project, name`)
		if err != nil {
			return nil, err
		}
		globalForwardingRules, err := r.loadMany(`SELECT data FROM compute_global_forwarding_rules ORDER BY project, name`)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"global_addresses":        globalAddresses,
			"health_checks":           healthChecks,
			"backend_services":        backendServices,
			"ssl_certificates":        sslCertificates,
			"target_https_proxies":    targetHTTPSProxies,
			"url_maps":                urlMaps,
			"global_forwarding_rules": globalForwardingRules,
		}, nil
	case "secretmanager":
		secrets, err := r.loadMany(`SELECT data FROM secretmanager_secrets ORDER BY project, name`)
		if err != nil {
			return nil, err
		}
		versions, err := r.loadMany(`SELECT data FROM secretmanager_versions ORDER BY project, secret_name, name`)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"secrets":  secrets,
			"versions": versions,
		}, nil
	case "pubsub":
		topics, err := r.loadMany(`SELECT data FROM pubsub_topics ORDER BY project, name`)
		if err != nil {
			return nil, err
		}
		subscriptions, err := r.loadMany(`SELECT data FROM pubsub_subscriptions ORDER BY project, name`)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"topics":        topics,
			"subscriptions": subscriptions,
		}, nil
	case "cloudrun":
		services, err := r.loadMany(`SELECT data FROM cloudrun_services ORDER BY project, location, name`)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"services": services,
		}, nil
	case "storage":
		buckets, err := r.loadMany(`SELECT data FROM storage_buckets ORDER BY project, name`)
		if err != nil {
			return nil, err
		}
		return map[string]any{"buckets": buckets}, nil
	case "operations":
		ops, err := r.loadMany(`SELECT data FROM operations ORDER BY project, name`)
		if err != nil {
			return nil, err
		}
		return map[string]any{"operations": ops}, nil
	default:
		return nil, models.ErrNotFound
	}
}

func (r *Repository) SetIAMBinding(project, role string, members []string) error {
	if role == "" {
		return fmt.Errorf("role is required")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM iam_bindings WHERE project = ? AND role = ?`, project, role); err != nil {
		return err
	}
	for _, member := range members {
		if strings.TrimSpace(member) == "" {
			continue
		}
		body, err := marshalData(map[string]any{"role": role, "member": member})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO iam_bindings (project, role, member, data) VALUES (?, ?, ?, ?)`, project, role, member, string(body)); err != nil {
			return mapInsertError(err)
		}
	}
	return tx.Commit()
}

func (r *Repository) GetIAMBinding(project, role string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM iam_bindings WHERE project = ? AND role = ? ORDER BY member`, project, role)
}

func (r *Repository) ListIAMBindings(project string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM iam_bindings WHERE project = ? ORDER BY role, member`, project)
}

func (r *Repository) AddIAMMember(project, role, member string) error {
	if role == "" {
		return fmt.Errorf("role is required")
	}
	if member == "" {
		return fmt.Errorf("member is required")
	}
	body, err := marshalData(map[string]any{"role": role, "member": member})
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`INSERT OR IGNORE INTO iam_bindings (project, role, member, data) VALUES (?, ?, ?, ?)`, project, role, member, string(body))
	return err
}

func (r *Repository) RemoveIAMMember(project, role, member string) error {
	return r.deleteWithResult(`DELETE FROM iam_bindings WHERE project = ? AND role = ? AND member = ?`, project, role, member)
}

func (r *Repository) GetIAMPolicy(project string) (map[string]any, error) {
	items, err := r.ListIAMBindings(project)
	if err != nil {
		return nil, err
	}
	byRole := map[string][]string{}
	roles := make([]string, 0)
	for _, item := range items {
		role := getString(item, "role")
		member := getString(item, "member")
		if role == "" || member == "" {
			continue
		}
		if _, ok := byRole[role]; !ok {
			roles = append(roles, role)
		}
		byRole[role] = append(byRole[role], member)
	}
	bindings := make([]map[string]any, 0, len(roles))
	for _, role := range roles {
		bindings = append(bindings, map[string]any{
			"role":    role,
			"members": byRole[role],
		})
	}
	return map[string]any{
		"bindings": bindings,
		"etag":     "fake-etag",
		"version":  1,
	}, nil
}

func (r *Repository) CreateRouter(project, region string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	networkName := getString(data, "network_name")
	if networkName == "" {
		networkName = extractNameFromSelfLink(getString(data, "network"))
	}
	if networkName == "" {
		return nil, fmt.Errorf("network reference is required")
	}
	data["name"] = name
	data["project"] = project
	data["region"] = region
	data["network_name"] = networkName
	if getString(data, "id") == "" {
		data["id"] = numericID()
	}
	if getString(data, "creationTimestamp") == "" {
		data["creationTimestamp"] = nowRFC3339()
	}
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO compute_routers (name, project, region, network_name, data) VALUES (?, ?, ?, ?, ?)`, name, project, region, networkName, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetRouter(project, region, name)
}

func (r *Repository) GetRouter(project, region, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM compute_routers WHERE project = ? AND region = ? AND name = ?`, project, region, name)
}

func (r *Repository) ListRouters(project, region string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM compute_routers WHERE project = ? AND region = ? ORDER BY name`, project, region)
}

func (r *Repository) UpdateRouter(project, region, name string, patch map[string]any) (map[string]any, error) {
	current, err := r.GetRouter(project, region, name)
	if err != nil {
		return nil, err
	}
	merged := patchMerge(current, patch, "name", "project", "region", "network_name")
	merged["name"] = name
	merged["project"] = project
	merged["region"] = region
	networkName := getString(merged, "network_name")
	if networkName == "" {
		networkName = extractNameFromSelfLink(getString(merged, "network"))
	}
	if networkName == "" {
		return nil, fmt.Errorf("network reference is required")
	}
	merged["network_name"] = networkName
	raw, err := marshalData(merged)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`UPDATE compute_routers SET network_name = ?, data = ? WHERE project = ? AND region = ? AND name = ?`, networkName, string(raw), project, region, name)
	if err != nil {
		return nil, mapInsertError(err)
	}
	return merged, nil
}

func (r *Repository) DeleteRouter(project, region, name string) error {
	return r.deleteWithResult(`DELETE FROM compute_routers WHERE project = ? AND region = ? AND name = ?`, project, region, name)
}

func (r *Repository) CreateRouterNAT(project, region, routerName string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	data["name"] = name
	data["project"] = project
	data["region"] = region
	data["router_name"] = routerName
	if getString(data, "id") == "" {
		data["id"] = numericID()
	}
	if getString(data, "creationTimestamp") == "" {
		data["creationTimestamp"] = nowRFC3339()
	}
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO compute_router_nats (name, project, region, router_name, data) VALUES (?, ?, ?, ?, ?)`, name, project, region, routerName, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetRouterNAT(project, region, routerName, name)
}

func (r *Repository) GetRouterNAT(project, region, routerName, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM compute_router_nats WHERE project = ? AND region = ? AND router_name = ? AND name = ?`, project, region, routerName, name)
}

func (r *Repository) ListRouterNATs(project, region, routerName string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM compute_router_nats WHERE project = ? AND region = ? AND router_name = ? ORDER BY name`, project, region, routerName)
}

func (r *Repository) UpdateRouterNAT(project, region, routerName, name string, patch map[string]any) (map[string]any, error) {
	current, err := r.GetRouterNAT(project, region, routerName, name)
	if err != nil {
		return nil, err
	}
	merged := patchMerge(current, patch, "name", "project", "region", "router_name")
	merged["name"] = name
	merged["project"] = project
	merged["region"] = region
	merged["router_name"] = routerName
	raw, err := marshalData(merged)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`UPDATE compute_router_nats SET data = ? WHERE project = ? AND region = ? AND router_name = ? AND name = ?`, string(raw), project, region, routerName, name)
	if err != nil {
		return nil, err
	}
	return merged, nil
}

func (r *Repository) DeleteRouterNAT(project, region, routerName, name string) error {
	return r.deleteWithResult(`DELETE FROM compute_router_nats WHERE project = ? AND region = ? AND router_name = ? AND name = ?`, project, region, routerName, name)
}

func (r *Repository) CreateDNSZone(project string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	data["name"] = name
	data["project"] = project
	if getString(data, "id") == "" {
		data["id"] = numericID()
	}
	if getString(data, "creationTime") == "" {
		data["creationTime"] = nowRFC3339()
	}
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO dns_managed_zones (name, project, data) VALUES (?, ?, ?)`, name, project, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetDNSZone(project, name)
}

func (r *Repository) GetDNSZone(project, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM dns_managed_zones WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) ListDNSZones(project string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM dns_managed_zones WHERE project = ? ORDER BY name`, project)
}

func (r *Repository) UpdateDNSZone(project, name string, patch map[string]any) (map[string]any, error) {
	current, err := r.GetDNSZone(project, name)
	if err != nil {
		return nil, err
	}
	merged := patchMerge(current, patch, "name", "project")
	merged["name"] = name
	merged["project"] = project
	raw, err := marshalData(merged)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`UPDATE dns_managed_zones SET data = ? WHERE project = ? AND name = ?`, string(raw), project, name)
	if err != nil {
		return nil, err
	}
	return merged, nil
}

func (r *Repository) DeleteDNSZone(project, name string) error {
	return r.deleteWithResult(`DELETE FROM dns_managed_zones WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) CreateDNSRecordSet(project, managedZone string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	rrtype := getString(data, "type")
	if rrtype == "" {
		return nil, fmt.Errorf("type is required")
	}
	data["name"] = name
	data["project"] = project
	data["managed_zone"] = managedZone
	data["type"] = rrtype
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO dns_record_sets (name, project, managed_zone, type, data) VALUES (?, ?, ?, ?, ?)`, name, project, managedZone, rrtype, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetDNSRecordSet(project, managedZone, name, rrtype)
}

func (r *Repository) GetDNSRecordSet(project, managedZone, name, rrtype string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM dns_record_sets WHERE project = ? AND managed_zone = ? AND name = ? AND type = ?`, project, managedZone, name, rrtype)
}

func (r *Repository) ListDNSRecordSets(project, managedZone string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM dns_record_sets WHERE project = ? AND managed_zone = ? ORDER BY name, type`, project, managedZone)
}

func (r *Repository) DeleteDNSRecordSet(project, managedZone, name, rrtype string) error {
	return r.deleteWithResult(`DELETE FROM dns_record_sets WHERE project = ? AND managed_zone = ? AND name = ? AND type = ?`, project, managedZone, name, rrtype)
}

func (r *Repository) CreateGlobalAddress(project string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	data["name"] = name
	data["project"] = project
	if getString(data, "id") == "" {
		data["id"] = numericID()
	}
	if getString(data, "creationTimestamp") == "" {
		data["creationTimestamp"] = nowRFC3339()
	}
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO compute_global_addresses (name, project, data) VALUES (?, ?, ?)`, name, project, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetGlobalAddress(project, name)
}

func (r *Repository) GetGlobalAddress(project, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM compute_global_addresses WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) ListGlobalAddresses(project string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM compute_global_addresses WHERE project = ? ORDER BY name`, project)
}

func (r *Repository) DeleteGlobalAddress(project, name string) error {
	return r.deleteWithResult(`DELETE FROM compute_global_addresses WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) CreateHealthCheck(project string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	data["name"] = name
	data["project"] = project
	if getString(data, "id") == "" {
		data["id"] = numericID()
	}
	if getString(data, "creationTimestamp") == "" {
		data["creationTimestamp"] = nowRFC3339()
	}
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO compute_health_checks (name, project, data) VALUES (?, ?, ?)`, name, project, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetHealthCheck(project, name)
}

func (r *Repository) GetHealthCheck(project, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM compute_health_checks WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) ListHealthChecks(project string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM compute_health_checks WHERE project = ? ORDER BY name`, project)
}

func (r *Repository) UpdateHealthCheck(project, name string, patch map[string]any) (map[string]any, error) {
	current, err := r.GetHealthCheck(project, name)
	if err != nil {
		return nil, err
	}
	merged := patchMerge(current, patch, "name", "project")
	merged["name"] = name
	merged["project"] = project
	raw, err := marshalData(merged)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`UPDATE compute_health_checks SET data = ? WHERE project = ? AND name = ?`, string(raw), project, name)
	if err != nil {
		return nil, err
	}
	return merged, nil
}

func (r *Repository) DeleteHealthCheck(project, name string) error {
	return r.deleteWithResult(`DELETE FROM compute_health_checks WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) CreateBackendService(project string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	data["name"] = name
	data["project"] = project
	if getString(data, "id") == "" {
		data["id"] = numericID()
	}
	if getString(data, "creationTimestamp") == "" {
		data["creationTimestamp"] = nowRFC3339()
	}
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO compute_backend_services (name, project, data) VALUES (?, ?, ?)`, name, project, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetBackendService(project, name)
}

func (r *Repository) GetBackendService(project, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM compute_backend_services WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) ListBackendServices(project string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM compute_backend_services WHERE project = ? ORDER BY name`, project)
}

func (r *Repository) UpdateBackendService(project, name string, patch map[string]any) (map[string]any, error) {
	current, err := r.GetBackendService(project, name)
	if err != nil {
		return nil, err
	}
	merged := patchMerge(current, patch, "name", "project")
	merged["name"] = name
	merged["project"] = project
	raw, err := marshalData(merged)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`UPDATE compute_backend_services SET data = ? WHERE project = ? AND name = ?`, string(raw), project, name)
	if err != nil {
		return nil, err
	}
	return merged, nil
}

func (r *Repository) DeleteBackendService(project, name string) error {
	return r.deleteWithResult(`DELETE FROM compute_backend_services WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) CreateSSLCertificate(project string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	data["name"] = name
	data["project"] = project
	if getString(data, "id") == "" {
		data["id"] = numericID()
	}
	if getString(data, "creationTimestamp") == "" {
		data["creationTimestamp"] = nowRFC3339()
	}
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO compute_ssl_certificates (name, project, data) VALUES (?, ?, ?)`, name, project, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetSSLCertificate(project, name)
}

func (r *Repository) GetSSLCertificate(project, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM compute_ssl_certificates WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) ListSSLCertificates(project string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM compute_ssl_certificates WHERE project = ? ORDER BY name`, project)
}

func (r *Repository) DeleteSSLCertificate(project, name string) error {
	return r.deleteWithResult(`DELETE FROM compute_ssl_certificates WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) CreateTargetHTTPSProxy(project string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	data["name"] = name
	data["project"] = project
	if getString(data, "id") == "" {
		data["id"] = numericID()
	}
	if getString(data, "creationTimestamp") == "" {
		data["creationTimestamp"] = nowRFC3339()
	}
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO compute_target_https_proxies (name, project, data) VALUES (?, ?, ?)`, name, project, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetTargetHTTPSProxy(project, name)
}

func (r *Repository) GetTargetHTTPSProxy(project, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM compute_target_https_proxies WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) ListTargetHTTPSProxies(project string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM compute_target_https_proxies WHERE project = ? ORDER BY name`, project)
}

func (r *Repository) UpdateTargetHTTPSProxy(project, name string, patch map[string]any) (map[string]any, error) {
	current, err := r.GetTargetHTTPSProxy(project, name)
	if err != nil {
		return nil, err
	}
	merged := patchMerge(current, patch, "name", "project")
	merged["name"] = name
	merged["project"] = project
	raw, err := marshalData(merged)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`UPDATE compute_target_https_proxies SET data = ? WHERE project = ? AND name = ?`, string(raw), project, name)
	if err != nil {
		return nil, err
	}
	return merged, nil
}

func (r *Repository) DeleteTargetHTTPSProxy(project, name string) error {
	return r.deleteWithResult(`DELETE FROM compute_target_https_proxies WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) CreateURLMap(project string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	data["name"] = name
	data["project"] = project
	if getString(data, "id") == "" {
		data["id"] = numericID()
	}
	if getString(data, "creationTimestamp") == "" {
		data["creationTimestamp"] = nowRFC3339()
	}
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO compute_url_maps (name, project, data) VALUES (?, ?, ?)`, name, project, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetURLMap(project, name)
}

func (r *Repository) GetURLMap(project, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM compute_url_maps WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) ListURLMaps(project string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM compute_url_maps WHERE project = ? ORDER BY name`, project)
}

func (r *Repository) UpdateURLMap(project, name string, patch map[string]any) (map[string]any, error) {
	current, err := r.GetURLMap(project, name)
	if err != nil {
		return nil, err
	}
	merged := patchMerge(current, patch, "name", "project")
	merged["name"] = name
	merged["project"] = project
	raw, err := marshalData(merged)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`UPDATE compute_url_maps SET data = ? WHERE project = ? AND name = ?`, string(raw), project, name)
	if err != nil {
		return nil, err
	}
	return merged, nil
}

func (r *Repository) DeleteURLMap(project, name string) error {
	return r.deleteWithResult(`DELETE FROM compute_url_maps WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) CreateGlobalForwardingRule(project string, data map[string]any) (map[string]any, error) {
	name := getString(data, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	data["name"] = name
	data["project"] = project
	if getString(data, "id") == "" {
		data["id"] = numericID()
	}
	if getString(data, "creationTimestamp") == "" {
		data["creationTimestamp"] = nowRFC3339()
	}
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO compute_global_forwarding_rules (name, project, data) VALUES (?, ?, ?)`, name, project, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetGlobalForwardingRule(project, name)
}

func (r *Repository) GetGlobalForwardingRule(project, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM compute_global_forwarding_rules WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) ListGlobalForwardingRules(project string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM compute_global_forwarding_rules WHERE project = ? ORDER BY name`, project)
}

func (r *Repository) DeleteGlobalForwardingRule(project, name string) error {
	return r.deleteWithResult(`DELETE FROM compute_global_forwarding_rules WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) CreateSecret(project string, data map[string]any) (map[string]any, error) {
	name := getString(data, "secretId")
	if name == "" {
		name = extractNameFromSelfLink(getString(data, "name"))
	}
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	data["project"] = project
	data["secretId"] = name
	if getString(data, "name") == "" {
		data["name"] = fmt.Sprintf("projects/%s/secrets/%s", project, name)
	}
	if getString(data, "createTime") == "" {
		data["createTime"] = nowRFC3339()
	}
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO secretmanager_secrets (name, project, data) VALUES (?, ?, ?)`, name, project, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetSecret(project, name)
}

func (r *Repository) GetSecret(project, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM secretmanager_secrets WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) ListSecrets(project string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM secretmanager_secrets WHERE project = ? ORDER BY name`, project)
}

func (r *Repository) DeleteSecret(project, name string) error {
	return r.deleteWithResult(`DELETE FROM secretmanager_secrets WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) CreateSecretVersion(project, secretName string, data map[string]any) (map[string]any, error) {
	// Use MAX(rowid) to get monotonically increasing version numbers even after deletions
	var maxRowid sql.NullInt64
	if err := r.db.QueryRow(`SELECT MAX(rowid) FROM secretmanager_versions WHERE project = ? AND secret_name = ?`, project, secretName).Scan(&maxRowid); err != nil {
		return nil, err
	}
	versionNum := int(maxRowid.Int64) + 1
	name := fmt.Sprintf("projects/%s/secrets/%s/versions/%d", project, secretName, versionNum)
	data["name"] = name
	data["project"] = project
	data["secret_name"] = secretName
	if getString(data, "createTime") == "" {
		data["createTime"] = nowRFC3339()
	}
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO secretmanager_versions (name, project, secret_name, data) VALUES (?, ?, ?, ?)`, name, project, secretName, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetSecretVersion(name)
}

func (r *Repository) GetSecretVersion(name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM secretmanager_versions WHERE name = ?`, name)
}

func (r *Repository) ListSecretVersions(project, secretName string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM secretmanager_versions WHERE project = ? AND secret_name = ? ORDER BY rowid`, project, secretName)
}

func (r *Repository) GetLatestSecretVersion(project, secretName string) (map[string]any, error) {
	// Order by rowid DESC to get the most recently inserted version (correct for numeric ordering)
	return r.loadOne(`SELECT data FROM secretmanager_versions WHERE project = ? AND secret_name = ? ORDER BY rowid DESC LIMIT 1`, project, secretName)
}

func (r *Repository) DeleteSecretVersion(name string) error {
	return r.deleteWithResult(`DELETE FROM secretmanager_versions WHERE name = ?`, name)
}

func (r *Repository) CreateTopic(project string, data map[string]any) (map[string]any, error) {
	name := extractNameFromSelfLink(getString(data, "name"))
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	data["project"] = project
	if getString(data, "name") == "" || getString(data, "name") == name {
		data["name"] = fmt.Sprintf("projects/%s/topics/%s", project, name)
	}
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO pubsub_topics (name, project, data) VALUES (?, ?, ?)`, name, project, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetTopic(project, name)
}

func (r *Repository) GetTopic(project, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM pubsub_topics WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) ListTopics(project string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM pubsub_topics WHERE project = ? ORDER BY name`, project)
}

func (r *Repository) DeleteTopic(project, name string) error {
	return r.deleteWithResult(`DELETE FROM pubsub_topics WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) CreateSubscription(project string, data map[string]any) (map[string]any, error) {
	name := extractNameFromSelfLink(getString(data, "name"))
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	topicName := extractNameFromSelfLink(getString(data, "topic"))
	if topicName == "" {
		return nil, fmt.Errorf("topic reference is required")
	}
	// Validate topic exists (FK was removed so subscriptions survive topic deletion)
	if _, err := r.GetTopic(project, topicName); err != nil {
		return nil, models.ErrNotFound
	}
	data["project"] = project
	if getString(data, "name") == "" || getString(data, "name") == name {
		data["name"] = fmt.Sprintf("projects/%s/subscriptions/%s", project, name)
	}
	data["topic_name"] = topicName
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO pubsub_subscriptions (name, project, topic_name, data) VALUES (?, ?, ?, ?)`, name, project, topicName, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetSubscription(project, name)
}

func (r *Repository) GetSubscription(project, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM pubsub_subscriptions WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) ListSubscriptions(project string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM pubsub_subscriptions WHERE project = ? ORDER BY name`, project)
}

func (r *Repository) UpdateSubscription(project, name string, patch map[string]any) (map[string]any, error) {
	current, err := r.GetSubscription(project, name)
	if err != nil {
		return nil, err
	}
	merged := patchMerge(current, patch, "name", "project", "topic_name")
	merged["project"] = project
	if currentName := extractNameFromSelfLink(getString(current, "name")); currentName != "" {
		merged["name"] = fmt.Sprintf("projects/%s/subscriptions/%s", project, currentName)
	}
	topicName := getString(current, "topic_name")
	if topicName == "" {
		topicName = extractNameFromSelfLink(getString(current, "topic"))
	}
	if topicName == "" {
		return nil, fmt.Errorf("topic reference is required")
	}
	merged["topic_name"] = topicName
	raw, err := marshalData(merged)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`UPDATE pubsub_subscriptions SET topic_name = ?, data = ? WHERE project = ? AND name = ?`, topicName, string(raw), project, name)
	if err != nil {
		return nil, mapInsertError(err)
	}
	return merged, nil
}

func (r *Repository) DeleteSubscription(project, name string) error {
	return r.deleteWithResult(`DELETE FROM pubsub_subscriptions WHERE project = ? AND name = ?`, project, name)
}

func (r *Repository) CreateCloudRunService(project, location string, data map[string]any) (map[string]any, error) {
	name := extractNameFromSelfLink(getString(data, "name"))
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	data["project"] = project
	data["location"] = location
	if getString(data, "name") == "" || getString(data, "name") == name {
		data["name"] = fmt.Sprintf("projects/%s/locations/%s/services/%s", project, location, name)
	}
	if getString(data, "createTime") == "" {
		data["createTime"] = nowRFC3339()
	}
	raw, err := marshalData(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO cloudrun_services (name, project, location, data) VALUES (?, ?, ?, ?)`, name, project, location, string(raw))
	if err != nil {
		return nil, mapInsertError(err)
	}
	return r.GetCloudRunService(project, location, name)
}

func (r *Repository) GetCloudRunService(project, location, name string) (map[string]any, error) {
	return r.loadOne(`SELECT data FROM cloudrun_services WHERE project = ? AND location = ? AND name = ?`, project, location, name)
}

func (r *Repository) ListCloudRunServices(project, location string) ([]map[string]any, error) {
	return r.loadMany(`SELECT data FROM cloudrun_services WHERE project = ? AND location = ? ORDER BY name`, project, location)
}

func (r *Repository) UpdateCloudRunService(project, location, name string, patch map[string]any) (map[string]any, error) {
	current, err := r.GetCloudRunService(project, location, name)
	if err != nil {
		return nil, err
	}
	merged := patchMerge(current, patch, "name", "project", "location")
	merged["project"] = project
	merged["location"] = location
	merged["name"] = fmt.Sprintf("projects/%s/locations/%s/services/%s", project, location, name)
	raw, err := marshalData(merged)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`UPDATE cloudrun_services SET data = ? WHERE project = ? AND location = ? AND name = ?`, string(raw), project, location, name)
	if err != nil {
		return nil, err
	}
	return merged, nil
}

func (r *Repository) DeleteCloudRunService(project, location, name string) error {
	return r.deleteWithResult(`DELETE FROM cloudrun_services WHERE project = ? AND location = ? AND name = ?`, project, location, name)
}
