resource "google_compute_network" "gke" {
	name                    = "gke"
	mtu                     = var.mtu
	auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "gke" {
	name          = "gke"
	ip_cidr_range = "10.15.0.0/22"
	network       = google_compute_network.gke.id
	secondary_ip_range {
		range_name    = "gke-nodes"
		ip_cidr_range = "10.16.0.0/22"
	}
	secondary_ip_range {
		range_name    = "gke-pods"
		ip_cidr_range = "10.24.0.0/14"
	}
	secondary_ip_range {
		range_name    = "gke-services"
		ip_cidr_range = "10.96.0.0/14"
	}
}

resource "google_compute_route" "gke-default" {
	name             = "gke-default-route"
	dest_range       = "0.0.0.0/0"
	network          = google_compute_network.gke.id
	next_hop_gateway = "default-internet-gateway"
	priority         = 1000
}

resource "google_service_account" "gke" {
	account_id   = "service-account-id"
	display_name = "GKE Service Account"
}

resource "google_container_cluster" "primary" {
	name       = "my-gke-cluster"
	location   = "us-east5"
	network    = google_compute_network.gke.id
	subnetwork = google_compute_subnetwork.gke.id

	ip_allocation_policy {
		# use_ip_aliases = true
		cluster_secondary_range_name = "gke-pods"
		services_secondary_range_name = "gke-services"
	}
	deletion_protection = false
	node_locations = [
		"us-east5-a"
	]

	workload_identity_config {
		workload_pool = "${data.google_project.self.project_id}.svc.id.goog"
	}

	# We can't create a cluster with no node pool defined, but we want to only use
	# separately managed node pools. So we create the smallest possible default
	# node pool and immediately delete it.
	remove_default_node_pool = true
	initial_node_count       = 1
}

resource "google_container_node_pool" "primary_preemptible_nodes" {
	name       = "my-node-pool"
	location   = "us-east5"
	cluster    = google_container_cluster.primary.name
	node_count = 3

	node_config {
		preemptible  = true
		machine_type = "e2-medium"

		# Google recommends custom service accounts that have cloud-platform scope and permissions granted via IAM Roles.
		service_account = google_service_account.gke.email
		oauth_scopes    = [
			"https://www.googleapis.com/auth/cloud-platform"
		]
	}
	network_config {
		#enable_private_nodes = true
		create_pod_range = false
		pod_range = "gke-pods"
		#additional_node_network_configs {
		#	network = google_compute_network.gke.id
		#	subnetwork = google_compute_subnetwork.gke.name
		#}
		#additional_pod_network_configs {
		#	subnetwork = google_compute_subnetwork.gke.name
		#	secondary_pod_range = "gke-pods"
		#}
	}
}

output "node_pool" {
	value = google_container_node_pool.primary_preemptible_nodes
}

data "google_client_config" "default" {}

data "template_file" "kubeconfig" {
	template = file("${path.module}/kubeconfig.tpl")

	vars = {
		name           = google_container_cluster.primary.name
		endpoint       = "https://${google_container_cluster.primary.endpoint}"
		cluster_ca_certificate = google_container_cluster.primary.master_auth[0].cluster_ca_certificate
		access_token           = data.google_client_config.default.access_token
	}
}

resource "local_file" "kubeconfig" {
	content  = data.template_file.kubeconfig.rendered
	filename = "kubeconfig"
	file_permission = "0600"
}


