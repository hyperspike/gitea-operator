data "aws_iam_policy_document" "gitea_operator_assume_role_policy" {
	statement {
		actions = ["sts:AssumeRoleWithWebIdentity"]

		principals {
			type = "Federated"
			identifiers = ["arn:aws:iam::${data.aws_caller_identity.current.account_id}:oidc-provider/${replace(aws_eks_cluster.eks.identity[0].oidc[0].issuer, "https://", "")}"]
		}
		condition {
			test = "StringEquals"
			variable = "${replace(aws_eks_cluster.eks.identity[0].oidc[0].issuer, "https://", "")}:sub"
			values = ["system:serviceaccount:gitea-operator-system:gitea-operator-controller-manager"]
		}
		condition {
			test = "StringEquals"
			variable = "${replace(aws_eks_cluster.eks.identity[0].oidc[0].issuer, "https://", "")}:aud"
			values = ["sts.amazonaws.com"]
		}
	}
}

resource "aws_iam_role" "gitea_operator_role" {
	name = "gitea-operator-role"
	assume_role_policy = data.aws_iam_policy_document.gitea_operator_assume_role_policy.json
}

data "aws_iam_policy_document" "gitea_operator_policy_document" {
	statement {
		actions = [
			"iam:CreateUser",
			"iam:DeleteUser",
			"iam:CreatePolicy",
			"iam:CreateAccessKey",
			"iam:DeleteAccessKey",
			"iam:AttachUserPolicy",
			"iam:DetachUserPolicy",
			"iam:PutUserPolicy",
			"iam:DeleteUserPolicy",
			"iam:DeletePolicy",
			"iam:ListAttachedUserPolicies",
			"iam:ListUsers",
			"iam:ListAccessKeys",
			"iam:ListPolicies",

			"s3:CreateBucket",
			"s3:DeleteBucket",
			"s3:ListBucket",
			"s3:ListAllMyBuckets",
			"s3:DeleteObject",
		]

		resources = ["*"]
	}
}

resource "aws_iam_policy" "gitea_operator_policy" {
	name = "gitea-operator-policy"
	description = "Policy for Gitea Operator"
	policy = data.aws_iam_policy_document.gitea_operator_policy_document.json
}

resource "aws_iam_role_policy_attachment" "gitea_operator" {
	role = aws_iam_role.gitea_operator_role.name
	policy_arn = aws_iam_policy.gitea_operator_policy.arn
}

#data "template_file" "gitea_operator" {
#	template = file("${path.module}/gitea-operator.yaml.tpl")

#	vars = {
#		role_arn = aws_iam_role.gitea_operator_role.arn
#		operator_image = var.operator_image
#	}
#}

#resource "local_file" "gitea_operator" {
#	content = data.template_file.gitea_operator.rendered
#	filename = "${path.module}/gitea-operator.yaml"
#}
