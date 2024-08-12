data "google_project" "project" {
	project_id = var.gke-project
}

locals {
	member = "principal://iam.googleapis.com/projects/${data.google_project.project.number}/locations/global/workloadIdentityPools/${var.gke-project}.svc.id.goog/subject/ns/external-dns/sa/external-dns"
}

resource "google_project_iam_member" "external_dns" {
	member  = local.member
	project = "DNS-PROJECT"
	role    = "roles/dns.reader"
}

resource "google_dns_managed_zone_iam_member" "member" {
	project      = "DNS-PROJECT"
	managed_zone = "ZONE-NAME"
	role         = "roles/dns.admin"
	member       = local.member
}
