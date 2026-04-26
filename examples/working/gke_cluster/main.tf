# GKE cluster with node pool.
# Tests: cluster → node pool dependency chain.

resource "google_compute_network" "gke" {
  name                    = "gke-network"
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "gke" {
  name          = "gke-subnet"
  ip_cidr_range = "10.1.0.0/24"
  region        = "us-central1"
  network       = google_compute_network.gke.id
}

resource "google_container_cluster" "main" {
  name     = "test-cluster"
  location = "us-central1"

  network    = google_compute_network.gke.id
  subnetwork = google_compute_subnetwork.gke.id

  # We can't create a cluster with no node pool,
  # but we want to manage them separately.
  remove_default_node_pool = true
  initial_node_count       = 1
}

resource "google_container_node_pool" "primary" {
  name       = "primary-pool"
  location   = "us-central1"
  cluster    = google_container_cluster.main.name
  node_count = 2

  node_config {
    machine_type = "e2-medium"
  }
}
