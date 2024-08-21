locals {
	controller_member = "principal://iam.googleapis.com/projects/${data.google_project.self.number}/locations/global/workloadIdentityPools/${data.google_project.self.project_id}.svc.id.goog/subject/ns/gitea-operator-system/sa/gitea-operator-controller-manager"
}

resource "google_project_iam_member" "gitea-operator" {
	member = local.controller_member
	project = data.google_project.self.project_id
	role    = "roles/storage.admin"
}

resource "google_project_iam_member" "gitea-operator-iam" {
	member = local.controller_member
	project = data.google_project.self.project_id
	role    = "roles/iam.serviceAccountCreator"
}

resource "google_project_iam_member" "gitea-operator-iam-keys" {
	member = local.controller_member
	project = data.google_project.self.project_id
	role    = "roles/iam.serviceAccountKeyAdmin"
}
resource "google_project_iam_member" "gitea-operator-editor" {
	member = local.controller_member
	project = data.google_project.self.project_id
	role    = "roles/editor"
}

#
#resource "google_dns_managed_zone_iam_member" "external-dns" {
#	member       = local.member
#	project      = data.google_project.self.project_id
#	managed_zone = google_dns_managed_zone.hyperspike-io.name
#	role         = "roles/dns.admin"
#}
