resource "aws_kms_key" "eks_encryption" {
  description         = "KMS key for EKS cluster encryption"
  policy              = data.aws_iam_policy_document.kms_key_policy.json
  enable_key_rotation = true
}

# alias
resource "aws_kms_alias" "eks_encryption" {
  name          = "alias/eks/${var.cluster_name}"
  target_key_id = aws_kms_key.eks_encryption.id
}

data "aws_iam_policy_document" "kms_key_policy" {
  statement {
    sid = "Key Administrators"
    actions = [
      "kms:Create*",
      "kms:Describe*",
      "kms:Enable*",
      "kms:List*",
      "kms:Put*",
      "kms:Update*",
      "kms:Revoke*",
      "kms:Disable*",
      "kms:Get*",
      "kms:Delete*",
      "kms:ScheduleKeyDeletion",
      "kms:CancelKeyDeletion",
      "kms:Decrypt",
      "kms:DescribeKey",
      "kms:Encrypt",
      "kms:ReEncrypt*",
      "kms:GenerateDataKey*",
      "kms:TagResource"
    ]
    principals {
      type = "AWS"
      identifiers = [
        "arn:aws:iam::${data.aws_caller_identity.current.account_id}:root",
        data.aws_caller_identity.current.arn
      ]
    }
    resources = ["*"]
  }

  statement {
    actions = [
      "kms:Encrypt",
      "kms:Decrypt",
      "kms:ReEncrypt*",
      "kms:GenerateDataKey*",
      "kms:DescribeKey"
    ]
    principals {
      type        = "Service"
      identifiers = ["eks.amazonaws.com"]
    }
    resources = ["*"]
  }
}


resource "aws_iam_policy" "cluster_encryption" {
  name        = "${var.cluster_name}-encryption-policy"
  description = "IAM policy for EKS cluster encryption"
  policy      = data.aws_iam_policy_document.cluster_encryption.json
}

data "aws_iam_policy_document" "cluster_encryption" {
  statement {
    actions = [
      "kms:Encrypt",
      "kms:Decrypt",
      "kms:ListGrants",
      "kms:DescribeKey"
    ]
    resources = [aws_kms_key.eks_encryption.arn]
  }
}

# Granting the EKS Cluster role the ability to use the KMS key
resource "aws_iam_role_policy_attachment" "cluster_encryption" {
  policy_arn = aws_iam_policy.cluster_encryption.arn
  role       = aws_iam_role.eks_cluster_role.name
}

