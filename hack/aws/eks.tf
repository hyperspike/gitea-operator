resource "aws_eks_cluster" "eks" {
	name = "eks"
	role_arn = aws_iam_role.eks_role.arn

	vpc_config {
		subnet_ids = aws_subnet.eks.*.id
		# security_group_ids = [aws_security_group.eks.id]
	}

	access_config {
		authentication_mode = "CONFIG_MAP"
		bootstrap_cluster_creator_admin_permissions = true
	}

	depends_on = [
		aws_iam_role_policy_attachment.eks-EKSClusterPolicy,
		aws_iam_role_policy_attachment.eks-EKSVPCPolicy
	]
}

data "aws_iam_policy_document" "eks_assume_role_policy" {
	statement {
		actions = ["sts:AssumeRole"]

		principals {
			type = "Service"
			identifiers = ["eks.amazonaws.com"]
		}
	}
}

resource "aws_iam_role" "eks_role" {
	name = "eks-role"
	assume_role_policy = data.aws_iam_policy_document.eks_assume_role_policy.json
}

resource "aws_iam_role_policy_attachment" "eks-EKSClusterPolicy" {
	role = aws_iam_role.eks_role.name
	policy_arn = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
}
resource "aws_iam_role_policy_attachment" "eks-EKSVPCPolicy" {
	role = aws_iam_role.eks_role.name
	policy_arn = "arn:aws:iam::aws:policy/AmazonEKSVPCResourceController"
}

resource "aws_eks_node_group" "eks_node_group" {
	cluster_name = aws_eks_cluster.eks.name
	node_group_name = "eks-node-group"
	node_role_arn = aws_iam_role.eks_node_role.arn
	subnet_ids = [aws_subnet.eks[0].id]
	instance_types = ["t3a.medium"]

	scaling_config {
		desired_size = 3

		min_size = 2
		max_size = 4
	}

	depends_on = [
		aws_iam_role_policy_attachment.eks_node_EKSWorkerNodePolicy,
		aws_iam_role_policy_attachment.eks_node_EKSCNIPolicy,
		aws_iam_role_policy_attachment.eks_node_ContainerRegistry
	]
}

data "aws_iam_policy_document" "eks_node_assume_role_policy" {
	statement {
		actions = ["sts:AssumeRole"]

		principals {
			type = "Service"
			identifiers = ["ec2.amazonaws.com"]
		}
	}
}

resource "aws_iam_role" "eks_node_role" {
	name = "eks-node-role"
	assume_role_policy = data.aws_iam_policy_document.eks_node_assume_role_policy.json
}

resource "aws_iam_role_policy_attachment" "eks_node_EKSWorkerNodePolicy" {
	role = aws_iam_role.eks_node_role.name
	policy_arn = "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"
}

resource "aws_iam_role_policy_attachment" "eks_node_EKSCNIPolicy" {
	role = aws_iam_role.eks_node_role.name
	policy_arn = "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"
}

resource "aws_iam_role_policy_attachment" "eks_node_ContainerRegistry" {
	role = aws_iam_role.eks_node_role.name
	policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
}

data "tls_certificate" "eks" {
	url = aws_eks_cluster.eks.identity[0].oidc[0].issuer
}

resource "aws_iam_openid_connect_provider" "eks" {
	client_id_list  = ["sts.amazonaws.com"]
	thumbprint_list = [data.tls_certificate.eks.certificates[0].sha1_fingerprint]
	url             = data.tls_certificate.eks.url
}

data "aws_eks_cluster_auth" "current" {
	name = aws_eks_cluster.eks.name
}

data "template_file" "kubeconfig" {
	template = file("${path.module}/kubeconfig.tpl")

	vars = {
		name     = aws_eks_cluster.eks.name
		endpoint = aws_eks_cluster.eks.endpoint
		ca       = aws_eks_cluster.eks.certificate_authority.0.data
		token    =  data.aws_eks_cluster_auth.current.token
	}
}

resource "local_file" "kubeconfig" {
	filename = "${path.module}/kubeconfig"
	content  = data.template_file.kubeconfig.rendered
	file_permission = "0600"
}
