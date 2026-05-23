package repository_test

import (
	"path/filepath"
	"testing"

	"github.com/redscaresu/fakegcp/models"
	"github.com/redscaresu/fakegcp/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	project  = "test-project"
	region   = "us-central1"
	zone     = "us-central1-a"
	location = "us-central1"
)

// newRepo returns a hermetic, in-memory repository for one test.
func newRepo(t *testing.T) *repository.Repository {
	t.Helper()
	repo, err := repository.New(":memory:")
	require.NoError(t, err)
	return repo
}

// TestSchemaMigration verifies that New(":memory:") opens cleanly and the
// foundational tables enumerated in migrate() are created. We probe a
// representative subset via the Create/List public surface so the test
// doesn't depend on SQL internals.
func TestSchemaMigration(t *testing.T) {
	repo := newRepo(t)

	// Listing on every targeted table should succeed (empty slice, no
	// error) once migrate() has run — that's the most direct way to
	// assert the table exists without touching the *sql.DB directly.
	lists := []func() error{
		func() error { _, err := repo.ListNetworks(project); return err },
		func() error { _, err := repo.ListSubnetworks(project, region); return err },
		func() error { _, err := repo.ListFirewalls(project); return err },
		func() error { _, err := repo.ListDisks(project, zone); return err },
		func() error { _, err := repo.ListInstances(project, zone); return err },
		func() error { _, err := repo.ListAddresses(project, region); return err },
		func() error { _, err := repo.ListClusters(project, location); return err },
		func() error { _, err := repo.ListNodePools(project, location, "any-cluster"); return err },
		func() error { _, err := repo.ListSQLInstances(project); return err },
		func() error { _, err := repo.ListSQLDatabases(project, "any-instance"); return err },
		func() error { _, err := repo.ListSQLUsers(project, "any-instance"); return err },
		func() error { _, err := repo.ListServiceAccounts(project); return err },
		func() error { _, err := repo.ListSAKeys("missing@example.iam.gserviceaccount.com"); return err },
		func() error { _, err := repo.ListBuckets(project); return err },
	}
	for i, fn := range lists {
		require.NoError(t, fn(), "list probe %d failed; schema migration is broken", i)
	}

	// FullState walks every supported table — a successful call means
	// every table referenced by ServiceState exists.
	state, err := repo.FullState()
	require.NoError(t, err)
	require.NotNil(t, state)
	// Sanity-check the top-level service buckets are present.
	for _, service := range []string{"compute", "container", "sql", "iam", "storage", "operations"} {
		_, ok := state[service]
		require.True(t, ok, "FullState missing %q bucket", service)
	}
}

// ---------------------------------------------------------------------
// compute_networks
// ---------------------------------------------------------------------

func TestComputeNetworksCRUD(t *testing.T) {
	repo := newRepo(t)

	t.Run("create returns persisted row", func(t *testing.T) {
		net, err := repo.CreateNetwork(project, map[string]any{
			"name":                  "vpc-a",
			"autoCreateSubnetworks": true,
		})
		require.NoError(t, err)
		require.Equal(t, "vpc-a", net["name"])
		require.NotEmpty(t, net["id"])
	})

	t.Run("get returns the same row", func(t *testing.T) {
		got, err := repo.GetNetwork(project, "vpc-a")
		require.NoError(t, err)
		require.Equal(t, "vpc-a", got["name"])
	})

	t.Run("list returns the row", func(t *testing.T) {
		items, err := repo.ListNetworks(project)
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("update merges fields", func(t *testing.T) {
		merged, err := repo.UpdateNetwork(project, "vpc-a", map[string]any{
			"description": "patched",
		})
		require.NoError(t, err)
		require.Equal(t, "patched", merged["description"])
		require.Equal(t, "vpc-a", merged["name"])
	})

	t.Run("duplicate create rejected", func(t *testing.T) {
		_, err := repo.CreateNetwork(project, map[string]any{"name": "vpc-a"})
		require.ErrorIs(t, err, models.ErrAlreadyExists)
	})

	t.Run("missing name rejected", func(t *testing.T) {
		_, err := repo.CreateNetwork(project, map[string]any{})
		require.Error(t, err)
	})

	t.Run("delete removes the row", func(t *testing.T) {
		require.NoError(t, repo.DeleteNetwork(project, "vpc-a"))
		_, err := repo.GetNetwork(project, "vpc-a")
		require.ErrorIs(t, err, models.ErrNotFound)
	})

	t.Run("delete on missing returns ErrNotFound", func(t *testing.T) {
		err := repo.DeleteNetwork(project, "vpc-a")
		require.ErrorIs(t, err, models.ErrNotFound)
	})
}

// ---------------------------------------------------------------------
// compute_subnetworks  (FK -> compute_networks)
// ---------------------------------------------------------------------

func TestComputeSubnetworksCRUD(t *testing.T) {
	repo := newRepo(t)
	_, err := repo.CreateNetwork(project, map[string]any{"name": "vpc-a"})
	require.NoError(t, err)

	t.Run("create with bare network name", func(t *testing.T) {
		sub, err := repo.CreateSubnetwork(project, region, map[string]any{
			"name":        "sub-a",
			"network":     "vpc-a",
			"ipCidrRange": "10.0.0.0/24",
		})
		require.NoError(t, err)
		require.Equal(t, "sub-a", sub["name"])
		require.Equal(t, "vpc-a", sub["network_name"])
	})

	t.Run("get returns the row", func(t *testing.T) {
		got, err := repo.GetSubnetwork(project, region, "sub-a")
		require.NoError(t, err)
		require.Equal(t, "sub-a", got["name"])
	})

	t.Run("list returns the row", func(t *testing.T) {
		items, err := repo.ListSubnetworks(project, region)
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("update merges fields", func(t *testing.T) {
		merged, err := repo.UpdateSubnetwork(project, region, "sub-a", map[string]any{
			"description": "patched",
		})
		require.NoError(t, err)
		require.Equal(t, "patched", merged["description"])
	})

	t.Run("delete removes the row", func(t *testing.T) {
		require.NoError(t, repo.DeleteSubnetwork(project, region, "sub-a"))
		_, err := repo.GetSubnetwork(project, region, "sub-a")
		require.ErrorIs(t, err, models.ErrNotFound)
	})
}

func TestComputeSubnetworksFKEnforcement(t *testing.T) {
	repo := newRepo(t)

	// No vpc-a row exists — create must reject.
	_, err := repo.CreateSubnetwork(project, region, map[string]any{
		"name":        "sub-bad",
		"network":     "vpc-missing",
		"ipCidrRange": "10.0.0.0/24",
	})
	require.ErrorIs(t, err, models.ErrNotFound)
}

// ---------------------------------------------------------------------
// compute_firewalls  (FK -> compute_networks)
// ---------------------------------------------------------------------

func TestComputeFirewallsCRUD(t *testing.T) {
	repo := newRepo(t)
	_, err := repo.CreateNetwork(project, map[string]any{"name": "vpc-a"})
	require.NoError(t, err)

	t.Run("create", func(t *testing.T) {
		fw, err := repo.CreateFirewall(project, map[string]any{
			"name":    "fw-allow-ssh",
			"network": "vpc-a",
		})
		require.NoError(t, err)
		require.Equal(t, "fw-allow-ssh", fw["name"])
		require.Equal(t, "vpc-a", fw["network_name"])
	})

	t.Run("get", func(t *testing.T) {
		got, err := repo.GetFirewall(project, "fw-allow-ssh")
		require.NoError(t, err)
		require.Equal(t, "fw-allow-ssh", got["name"])
	})

	t.Run("list", func(t *testing.T) {
		items, err := repo.ListFirewalls(project)
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("update", func(t *testing.T) {
		merged, err := repo.UpdateFirewall(project, "fw-allow-ssh", map[string]any{
			"description": "patched",
		})
		require.NoError(t, err)
		require.Equal(t, "patched", merged["description"])
	})

	t.Run("delete", func(t *testing.T) {
		require.NoError(t, repo.DeleteFirewall(project, "fw-allow-ssh"))
		_, err := repo.GetFirewall(project, "fw-allow-ssh")
		require.ErrorIs(t, err, models.ErrNotFound)
	})
}

func TestComputeFirewallsFKEnforcement(t *testing.T) {
	repo := newRepo(t)

	_, err := repo.CreateFirewall(project, map[string]any{
		"name":    "fw-bad",
		"network": "vpc-missing",
	})
	require.ErrorIs(t, err, models.ErrNotFound)
}

// ---------------------------------------------------------------------
// compute_disks  (no FK; no Update)
// ---------------------------------------------------------------------

func TestComputeDisksCRUD(t *testing.T) {
	repo := newRepo(t)

	t.Run("create", func(t *testing.T) {
		d, err := repo.CreateDisk(project, zone, map[string]any{
			"name":   "disk-a",
			"sizeGb": "10",
		})
		require.NoError(t, err)
		require.Equal(t, "disk-a", d["name"])
		require.Equal(t, zone, d["zone"])
	})

	t.Run("get", func(t *testing.T) {
		got, err := repo.GetDisk(project, zone, "disk-a")
		require.NoError(t, err)
		require.Equal(t, "disk-a", got["name"])
	})

	t.Run("list", func(t *testing.T) {
		items, err := repo.ListDisks(project, zone)
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("duplicate rejected", func(t *testing.T) {
		_, err := repo.CreateDisk(project, zone, map[string]any{"name": "disk-a"})
		require.ErrorIs(t, err, models.ErrAlreadyExists)
	})

	t.Run("delete", func(t *testing.T) {
		require.NoError(t, repo.DeleteDisk(project, zone, "disk-a"))
		_, err := repo.GetDisk(project, zone, "disk-a")
		require.ErrorIs(t, err, models.ErrNotFound)
	})

	t.Run("delete missing returns ErrNotFound", func(t *testing.T) {
		err := repo.DeleteDisk(project, zone, "disk-a")
		require.ErrorIs(t, err, models.ErrNotFound)
	})
}

// ---------------------------------------------------------------------
// compute_instances  (no FK column, but validateInstanceNetworkInterfaces
// rejects dangling network/subnetwork refs)
// ---------------------------------------------------------------------

func TestComputeInstancesCRUD(t *testing.T) {
	repo := newRepo(t)

	t.Run("create without NICs", func(t *testing.T) {
		inst, err := repo.CreateInstance(project, zone, map[string]any{
			"name":        "vm-a",
			"machineType": "n1-standard-1",
		})
		require.NoError(t, err)
		require.Equal(t, "vm-a", inst["name"])
		require.Equal(t, zone, inst["zone"])
	})

	t.Run("get", func(t *testing.T) {
		got, err := repo.GetInstance(project, zone, "vm-a")
		require.NoError(t, err)
		require.Equal(t, "vm-a", got["name"])
	})

	t.Run("list", func(t *testing.T) {
		items, err := repo.ListInstances(project, zone)
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("delete", func(t *testing.T) {
		require.NoError(t, repo.DeleteInstance(project, zone, "vm-a"))
		_, err := repo.GetInstance(project, zone, "vm-a")
		require.ErrorIs(t, err, models.ErrNotFound)
	})
}

func TestComputeInstancesValidatesNetworkInterfaces(t *testing.T) {
	repo := newRepo(t)

	// NIC references a network that doesn't exist — must reject.
	_, err := repo.CreateInstance(project, zone, map[string]any{
		"name": "vm-bad",
		"networkInterfaces": []any{
			map[string]any{
				"network": "projects/" + project + "/global/networks/missing-vpc",
			},
		},
	})
	require.ErrorIs(t, err, models.ErrNotFound)
}

// ---------------------------------------------------------------------
// compute_addresses  (no FK; no Update)
// ---------------------------------------------------------------------

func TestComputeAddressesCRUD(t *testing.T) {
	repo := newRepo(t)

	t.Run("create", func(t *testing.T) {
		a, err := repo.CreateAddress(project, region, map[string]any{
			"name":        "addr-a",
			"addressType": "EXTERNAL",
		})
		require.NoError(t, err)
		require.Equal(t, "addr-a", a["name"])
	})

	t.Run("get", func(t *testing.T) {
		got, err := repo.GetAddress(project, region, "addr-a")
		require.NoError(t, err)
		require.Equal(t, "addr-a", got["name"])
	})

	t.Run("list", func(t *testing.T) {
		items, err := repo.ListAddresses(project, region)
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("delete", func(t *testing.T) {
		require.NoError(t, repo.DeleteAddress(project, region, "addr-a"))
		_, err := repo.GetAddress(project, region, "addr-a")
		require.ErrorIs(t, err, models.ErrNotFound)
	})
}

// ---------------------------------------------------------------------
// container_clusters
// ---------------------------------------------------------------------

func TestContainerClustersCRUD(t *testing.T) {
	repo := newRepo(t)

	t.Run("create without network refs", func(t *testing.T) {
		c, err := repo.CreateCluster(project, location, map[string]any{
			"name": "gke-a",
		})
		require.NoError(t, err)
		require.Equal(t, "gke-a", c["name"])
		require.Equal(t, location, c["location"])
	})

	t.Run("get", func(t *testing.T) {
		got, err := repo.GetCluster(project, location, "gke-a")
		require.NoError(t, err)
		require.Equal(t, "gke-a", got["name"])
	})

	t.Run("list", func(t *testing.T) {
		items, err := repo.ListClusters(project, location)
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("update merges fields", func(t *testing.T) {
		merged, err := repo.UpdateCluster(project, location, "gke-a", map[string]any{
			"description": "patched",
		})
		require.NoError(t, err)
		require.Equal(t, "patched", merged["description"])
	})

	t.Run("delete", func(t *testing.T) {
		require.NoError(t, repo.DeleteCluster(project, location, "gke-a"))
		_, err := repo.GetCluster(project, location, "gke-a")
		require.ErrorIs(t, err, models.ErrNotFound)
	})
}

func TestContainerClustersValidatesNetwork(t *testing.T) {
	repo := newRepo(t)

	// Cluster referring to a missing VPC must be rejected.
	_, err := repo.CreateCluster(project, location, map[string]any{
		"name":    "gke-bad",
		"network": "projects/" + project + "/global/networks/missing-vpc",
	})
	require.ErrorIs(t, err, models.ErrNotFound)
}

// ---------------------------------------------------------------------
// container_node_pools  (FK -> container_clusters, CASCADE)
// ---------------------------------------------------------------------

func TestContainerNodePoolsCRUD(t *testing.T) {
	repo := newRepo(t)
	_, err := repo.CreateCluster(project, location, map[string]any{"name": "gke-a"})
	require.NoError(t, err)

	t.Run("create", func(t *testing.T) {
		np, err := repo.CreateNodePool(project, location, "gke-a", map[string]any{
			"name":             "pool-a",
			"initialNodeCount": 1,
		})
		require.NoError(t, err)
		require.Equal(t, "pool-a", np["name"])
	})

	t.Run("get", func(t *testing.T) {
		got, err := repo.GetNodePool(project, location, "gke-a", "pool-a")
		require.NoError(t, err)
		require.Equal(t, "pool-a", got["name"])
	})

	t.Run("list", func(t *testing.T) {
		items, err := repo.ListNodePools(project, location, "gke-a")
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("delete", func(t *testing.T) {
		require.NoError(t, repo.DeleteNodePool(project, location, "gke-a", "pool-a"))
		_, err := repo.GetNodePool(project, location, "gke-a", "pool-a")
		require.ErrorIs(t, err, models.ErrNotFound)
	})
}

func TestContainerNodePoolsFKEnforcement(t *testing.T) {
	repo := newRepo(t)

	// Parent cluster does not exist — insert must be rejected by FK.
	_, err := repo.CreateNodePool(project, location, "missing-cluster", map[string]any{
		"name": "pool-bad",
	})
	require.ErrorIs(t, err, models.ErrNotFound)
}

// ---------------------------------------------------------------------
// sql_instances
// ---------------------------------------------------------------------

func TestSQLInstancesCRUD(t *testing.T) {
	repo := newRepo(t)

	t.Run("create", func(t *testing.T) {
		inst, err := repo.CreateSQLInstance(project, map[string]any{
			"name":            "pg-a",
			"databaseVersion": "POSTGRES_15",
		})
		require.NoError(t, err)
		require.Equal(t, "pg-a", inst["name"])
	})

	t.Run("get", func(t *testing.T) {
		got, err := repo.GetSQLInstance(project, "pg-a")
		require.NoError(t, err)
		require.Equal(t, "pg-a", got["name"])
	})

	t.Run("list", func(t *testing.T) {
		items, err := repo.ListSQLInstances(project)
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("update", func(t *testing.T) {
		merged, err := repo.UpdateSQLInstance(project, "pg-a", map[string]any{
			"settings": map[string]any{"tier": "db-f1-micro"},
		})
		require.NoError(t, err)
		settings, _ := merged["settings"].(map[string]any)
		require.NotNil(t, settings)
		assert.Equal(t, "db-f1-micro", settings["tier"])
	})

	t.Run("delete", func(t *testing.T) {
		require.NoError(t, repo.DeleteSQLInstance(project, "pg-a"))
		_, err := repo.GetSQLInstance(project, "pg-a")
		require.ErrorIs(t, err, models.ErrNotFound)
	})
}

func TestSQLInstancesValidatesPrivateNetwork(t *testing.T) {
	repo := newRepo(t)

	_, err := repo.CreateSQLInstance(project, map[string]any{
		"name": "pg-bad",
		"settings": map[string]any{
			"ipConfiguration": map[string]any{
				"privateNetwork": "projects/" + project + "/global/networks/missing-vpc",
			},
		},
	})
	require.ErrorIs(t, err, models.ErrNotFound)
}

// ---------------------------------------------------------------------
// sql_databases  (FK -> sql_instances, CASCADE)
// ---------------------------------------------------------------------

func TestSQLDatabasesCRUD(t *testing.T) {
	repo := newRepo(t)
	_, err := repo.CreateSQLInstance(project, map[string]any{"name": "pg-a"})
	require.NoError(t, err)

	t.Run("create", func(t *testing.T) {
		db, err := repo.CreateSQLDatabase(project, "pg-a", map[string]any{
			"name":    "appdb",
			"charset": "UTF8",
		})
		require.NoError(t, err)
		require.Equal(t, "appdb", db["name"])
	})

	t.Run("get", func(t *testing.T) {
		got, err := repo.GetSQLDatabase(project, "pg-a", "appdb")
		require.NoError(t, err)
		require.Equal(t, "appdb", got["name"])
	})

	t.Run("list", func(t *testing.T) {
		items, err := repo.ListSQLDatabases(project, "pg-a")
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("delete", func(t *testing.T) {
		require.NoError(t, repo.DeleteSQLDatabase(project, "pg-a", "appdb"))
		_, err := repo.GetSQLDatabase(project, "pg-a", "appdb")
		require.ErrorIs(t, err, models.ErrNotFound)
	})
}

func TestSQLDatabasesFKEnforcement(t *testing.T) {
	repo := newRepo(t)

	_, err := repo.CreateSQLDatabase(project, "missing-instance", map[string]any{
		"name": "db-bad",
	})
	require.ErrorIs(t, err, models.ErrNotFound)
}

// ---------------------------------------------------------------------
// sql_users  (FK -> sql_instances, CASCADE)
// ---------------------------------------------------------------------

func TestSQLUsersCRUD(t *testing.T) {
	repo := newRepo(t)
	_, err := repo.CreateSQLInstance(project, map[string]any{"name": "pg-a"})
	require.NoError(t, err)

	t.Run("create", func(t *testing.T) {
		u, err := repo.CreateSQLUser(project, "pg-a", map[string]any{
			"name":     "appuser",
			"password": "hunter2",
		})
		require.NoError(t, err)
		require.Equal(t, "appuser", u["name"])
	})

	t.Run("list", func(t *testing.T) {
		items, err := repo.ListSQLUsers(project, "pg-a")
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("update merges fields", func(t *testing.T) {
		merged, err := repo.UpdateSQLUser(project, "pg-a", "appuser", map[string]any{
			"password": "new-secret",
		})
		require.NoError(t, err)
		require.Equal(t, "new-secret", merged["password"])
	})

	t.Run("delete", func(t *testing.T) {
		require.NoError(t, repo.DeleteSQLUser(project, "pg-a", "appuser"))
		// No GetSQLUser, but list should now be empty.
		items, err := repo.ListSQLUsers(project, "pg-a")
		require.NoError(t, err)
		require.Len(t, items, 0)
	})
}

func TestSQLUsersFKEnforcement(t *testing.T) {
	repo := newRepo(t)

	_, err := repo.CreateSQLUser(project, "missing-instance", map[string]any{
		"name": "user-bad",
	})
	require.ErrorIs(t, err, models.ErrNotFound)
}

// ---------------------------------------------------------------------
// iam_service_accounts
// ---------------------------------------------------------------------

func TestIAMServiceAccountsCRUD(t *testing.T) {
	repo := newRepo(t)
	var email string

	t.Run("create", func(t *testing.T) {
		sa, err := repo.CreateServiceAccount(project, map[string]any{
			"accountId":   "deployer",
			"displayName": "Deployer",
		})
		require.NoError(t, err)
		require.NotEmpty(t, sa["email"])
		email = sa["email"].(string)
	})

	t.Run("get", func(t *testing.T) {
		got, err := repo.GetServiceAccount(project, email)
		require.NoError(t, err)
		require.Equal(t, email, got["email"])
	})

	t.Run("list", func(t *testing.T) {
		items, err := repo.ListServiceAccounts(project)
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("update merges fields", func(t *testing.T) {
		merged, err := repo.UpdateServiceAccount(project, email, map[string]any{
			"description": "patched",
		})
		require.NoError(t, err)
		require.Equal(t, "patched", merged["description"])
		require.Equal(t, email, merged["email"])
	})

	t.Run("delete", func(t *testing.T) {
		require.NoError(t, repo.DeleteServiceAccount(project, email))
		_, err := repo.GetServiceAccount(project, email)
		require.ErrorIs(t, err, models.ErrNotFound)
	})
}

// ---------------------------------------------------------------------
// iam_sa_keys  (FK -> iam_service_accounts.email, CASCADE)
// ---------------------------------------------------------------------

func TestIAMSAKeysCRUD(t *testing.T) {
	repo := newRepo(t)
	sa, err := repo.CreateServiceAccount(project, map[string]any{"accountId": "deployer"})
	require.NoError(t, err)
	email := sa["email"].(string)

	var keyName string

	t.Run("create", func(t *testing.T) {
		k, err := repo.CreateSAKey(project, email, map[string]any{
			"keyAlgorithm": "KEY_ALG_RSA_2048",
		})
		require.NoError(t, err)
		require.NotEmpty(t, k["name"])
		keyName = k["name"].(string)
	})

	t.Run("get", func(t *testing.T) {
		got, err := repo.GetSAKey(keyName)
		require.NoError(t, err)
		require.Equal(t, keyName, got["name"])
	})

	t.Run("list", func(t *testing.T) {
		items, err := repo.ListSAKeys(email)
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("delete", func(t *testing.T) {
		require.NoError(t, repo.DeleteSAKey(keyName))
		_, err := repo.GetSAKey(keyName)
		require.ErrorIs(t, err, models.ErrNotFound)
	})
}

func TestIAMSAKeysFKEnforcement(t *testing.T) {
	repo := newRepo(t)

	_, err := repo.CreateSAKey(project, "missing@example.iam.gserviceaccount.com", map[string]any{
		"keyAlgorithm": "KEY_ALG_RSA_2048",
	})
	require.ErrorIs(t, err, models.ErrNotFound)
}

func TestIAMSAKeysCascadeOnServiceAccountDelete(t *testing.T) {
	repo := newRepo(t)
	sa, err := repo.CreateServiceAccount(project, map[string]any{"accountId": "deployer"})
	require.NoError(t, err)
	email := sa["email"].(string)

	key, err := repo.CreateSAKey(project, email, map[string]any{"keyAlgorithm": "KEY_ALG_RSA_2048"})
	require.NoError(t, err)
	keyName := key["name"].(string)

	require.NoError(t, repo.DeleteServiceAccount(project, email))

	// Cascade should have wiped the dependent key row.
	_, err = repo.GetSAKey(keyName)
	require.ErrorIs(t, err, models.ErrNotFound)
}

// ---------------------------------------------------------------------
// storage_buckets
// ---------------------------------------------------------------------

func TestStorageBucketsCRUD(t *testing.T) {
	repo := newRepo(t)

	t.Run("create", func(t *testing.T) {
		b, err := repo.CreateBucket(project, map[string]any{
			"name":     "my-bucket",
			"location": "US",
		})
		require.NoError(t, err)
		require.Equal(t, "my-bucket", b["name"])
	})

	t.Run("get", func(t *testing.T) {
		got, err := repo.GetBucket("my-bucket")
		require.NoError(t, err)
		require.Equal(t, "my-bucket", got["name"])
	})

	t.Run("list", func(t *testing.T) {
		items, err := repo.ListBuckets(project)
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("update", func(t *testing.T) {
		merged, err := repo.UpdateBucket("my-bucket", map[string]any{
			"storageClass": "NEARLINE",
		})
		require.NoError(t, err)
		require.Equal(t, "NEARLINE", merged["storageClass"])
	})

	t.Run("duplicate create rejected", func(t *testing.T) {
		_, err := repo.CreateBucket(project, map[string]any{"name": "my-bucket"})
		require.ErrorIs(t, err, models.ErrAlreadyExists)
	})

	t.Run("delete", func(t *testing.T) {
		require.NoError(t, repo.DeleteBucket("my-bucket"))
		_, err := repo.GetBucket("my-bucket")
		require.ErrorIs(t, err, models.ErrNotFound)
	})

	t.Run("delete missing returns ErrNotFound", func(t *testing.T) {
		err := repo.DeleteBucket("my-bucket")
		require.ErrorIs(t, err, models.ErrNotFound)
	})
}

// ---------------------------------------------------------------------
// operations  (Store / Get only — no full CRUD surface)
// ---------------------------------------------------------------------

func TestOperationsStoreAndGet(t *testing.T) {
	repo := newRepo(t)

	t.Run("store with explicit name", func(t *testing.T) {
		err := repo.StoreOperation(project, zone, "", "op-1", map[string]any{
			"status":        "DONE",
			"operationType": "insert",
		})
		require.NoError(t, err)
	})

	t.Run("get returns the row", func(t *testing.T) {
		got, err := repo.GetOperation(project, "op-1")
		require.NoError(t, err)
		require.Equal(t, "op-1", got["name"])
		require.Equal(t, "DONE", got["status"])
	})

	t.Run("store with autogenerated name", func(t *testing.T) {
		// Empty name => StoreOperation allocates one. We can't predict
		// it, but the call must succeed.
		err := repo.StoreOperation(project, "", region, "", map[string]any{
			"status": "PENDING",
		})
		require.NoError(t, err)
	})

	t.Run("store with ON CONFLICT updates existing row", func(t *testing.T) {
		// Re-storing op-1 with new status should overwrite, not error.
		err := repo.StoreOperation(project, zone, "", "op-1", map[string]any{
			"status": "RUNNING",
		})
		require.NoError(t, err)

		got, err := repo.GetOperation(project, "op-1")
		require.NoError(t, err)
		require.Equal(t, "RUNNING", got["status"])
	})

	t.Run("get missing returns ErrNotFound", func(t *testing.T) {
		_, err := repo.GetOperation(project, "missing-op")
		require.ErrorIs(t, err, models.ErrNotFound)
	})
}

// ---------------------------------------------------------------------
// Cross-cutting: Reset wipes every table
// ---------------------------------------------------------------------

func TestResetWipesEveryTable(t *testing.T) {
	// Reset resolves the on-disk DB path to clear a snapshot baseline,
	// so this test needs a file-backed DB rather than `:memory:`.
	dbPath := filepath.Join(t.TempDir(), "repo.db")
	repo, err := repository.New(dbPath)
	require.NoError(t, err)

	_, err = repo.CreateNetwork(project, map[string]any{"name": "vpc-r"})
	require.NoError(t, err)
	_, err = repo.CreateBucket(project, map[string]any{"name": "b-r"})
	require.NoError(t, err)
	sa, err := repo.CreateServiceAccount(project, map[string]any{"accountId": "deployer-r"})
	require.NoError(t, err)
	_, err = repo.CreateSAKey(project, sa["email"].(string), map[string]any{})
	require.NoError(t, err)

	require.NoError(t, repo.Reset())

	nets, err := repo.ListNetworks(project)
	require.NoError(t, err)
	require.Len(t, nets, 0)

	buckets, err := repo.ListBuckets(project)
	require.NoError(t, err)
	require.Len(t, buckets, 0)

	sas, err := repo.ListServiceAccounts(project)
	require.NoError(t, err)
	require.Len(t, sas, 0)
}
