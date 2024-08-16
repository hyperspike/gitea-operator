resource "aws_route53_zone" "sandbox" {
	name = "aws-sandbox.hyperspike.io"
}

data "aws_iam_policy_document" "eks_external_dns_assume_role_policy" {
	statement {
		actions = ["sts:AssumeRoleWithWebIdentity"]

		principals {
			type = "Federated"
			identifiers = ["arn:aws:iam::${data.aws_caller_identity.current.account_id}:oidc-provider/${replace(aws_eks_cluster.eks.identity[0].oidc[0].issuer, "https://", "")}"]
		}
		condition {
			test = "StringEquals"
			variable = "${replace(aws_eks_cluster.eks.identity[0].oidc[0].issuer, "https://", "")}:sub"
			values = ["system:serviceaccount:kube-system:external-dns"]
		}
		condition {
			test = "StringEquals"
			variable = "${replace(aws_eks_cluster.eks.identity[0].oidc[0].issuer, "https://", "")}:aud"
			values = ["sts.amazonaws.com"]
		}
	}
}

resource "aws_iam_role" "eks_external_dns_role" {
	name = "eks-external-dns-role"
	assume_role_policy = data.aws_iam_policy_document.eks_external_dns_assume_role_policy.json
}

data "aws_iam_policy_document" "eks_external_dns_policy" {
	statement {
		actions = ["route53:ChangeResourceRecordSets", "route53:ListResourceRecordSets"]
		resources = ["arn:aws:route53:::hostedzone/${aws_route53_zone.sandbox.zone_id}"]
	}

	statement {
		actions = ["route53:ListHostedZones", "route53:ListResourceRecordSets", "route53:ListTagsForResource"]
		resources = ["*"]
	}
}

resource "aws_iam_policy" "eks_external_dns_policy" {
	name = "eks-external-dns-policy"
	policy = data.aws_iam_policy_document.eks_external_dns_policy.json
}

resource "aws_iam_role_policy_attachment" "eks_external_dns-ExternalDNSPolicy" {
	role = aws_iam_role.eks_external_dns_role.name
	policy_arn = aws_iam_policy.eks_external_dns_policy.arn
}

data "template_file" "external_dns" {
	template = file("${path.module}/external-dns.yaml.tpl")

	vars = {
		role_arn = aws_iam_role.eks_external_dns_role.arn
		domain   = aws_route53_zone.sandbox.name
	}
}

resource "local_file" "external_dns" {
	content = data.template_file.external_dns.rendered
	filename = "${path.module}/external-dns.yaml"
}

resource "null_resource" "kubectl-external-dns" {
	provisioner "local-exec" {
		command = "KUBECONFIG=${path.module}/kubeconfig kubectl apply -f ${path.module}/external-dns.yaml"
	}

	depends_on = [
		local_file.kubeconfig,
		local_file.external_dns
	]
}
