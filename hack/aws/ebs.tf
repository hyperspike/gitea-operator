data "aws_iam_policy_document" "eks_ebs_assume_role_policy" {
	statement {
		actions = ["sts:AssumeRoleWithWebIdentity"]

		principals {
			type = "Federated"
			identifiers = ["arn:aws:iam::${data.aws_caller_identity.current.account_id}:oidc-provider/${replace(aws_eks_cluster.eks.identity[0].oidc[0].issuer, "https://", "")}"]
		}
		condition {
			test = "StringEquals"
			variable = "${replace(aws_eks_cluster.eks.identity[0].oidc[0].issuer, "https://", "")}:sub"
			values = ["system:serviceaccount:kube-system:ebs-csi-controller-sa"]
		}
		condition {
			test = "StringEquals"
			variable = "${replace(aws_eks_cluster.eks.identity[0].oidc[0].issuer, "https://", "")}:aud"
			values = ["sts.amazonaws.com"]
		}
	}
}

resource "aws_iam_role" "eks_ebs_role" {
	name = "eks-ebs-role"
	assume_role_policy = data.aws_iam_policy_document.eks_ebs_assume_role_policy.json
}

resource "aws_iam_role_policy_attachment" "eks_ebs-EBSControllerPolicy" {
	role = aws_iam_role.eks_ebs_role.name
	policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy"
}

data "template_file" "ebs" {
	template = file("${path.module}/ebs.yaml.tpl")

	vars = {
		role_arn = aws_iam_role.eks_ebs_role.arn
	}
}

resource "local_file" "ebs" {
	content = data.template_file.ebs.rendered
	filename = "${path.module}/ebs.yaml"
}

resource "null_resource" "kubectl-ebs" {
	provisioner "local-exec" {
		command = "KUBECONFIG=${path.module}/kubeconfig kubectl apply -f ${path.module}/ebs.yaml"
	}

	depends_on = [
		local_file.kubeconfig,
		local_file.ebs
	]
}
