data "google_project" "self" {}

locals {
	member = "principal://iam.googleapis.com/projects/${data.google_project.self.number}/locations/global/workloadIdentityPools/${data.google_project.self.name}.svc.id.goog/subject/ns/kube-system/sa/external-dns"
}

resource "google_project_iam_member" "external_dns" {
	member  = local.member
	project = "DNS-PROJECT"
	role    = "roles/dns.reader"
}

resource "google_dns_managed_zone_iam_member" "member" {
	project      = data.google_project.self.project_id
	managed_zone = google_dns_managed_zone.hyperspike-io.name
	role         = "roles/dns.admin"
	member       = local.member
}

data "template_file" "external_dns" {
	template = file("${path.module}/external-dns.yaml.tpl")

	vars = {
		role_arn = local.member
		domain   = google_dns_managed_zone.hyperspike-io.dns_name
	}
}

resource "local_file" "external_dns" {
	content  = data.template_file.external_dns.rendered
	filename = "${path.module}/external-dns.yaml"
}

resource "google_dns_managed_zone" "hyperspike-io" {
	name        = "gcp-sandbox-hyperspike-io"
	dns_name    = "gcp-sandbox.hyperspike.io."
	description = "Subdomain for hyperspike DNS"

	#dnssec_config {
	#}
	#labels      = {
	#}
}

resource "null_resource" "external_dns" {
	provisioner "local-exec" {
		command = "KUBECONFIG=${path.module}/kubeconfig kubectl apply -f ${local_file.external_dns.filename}"
	}

	depends_on = [
		local_file.kubeconfig,
		local_file.external_dns
	]
}
