package client

import (
	"context"
	"fmt"

	"cloud.google.com/go/compute/metadata"
	gcpiam "cloud.google.com/go/iam"
	"cloud.google.com/go/storage"
	giam "google.golang.org/api/iam/v1"
	"google.golang.org/api/iterator"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type ObjectClient interface {
	// Authenticate with the Cloud Provider.
	Auth(opts *ObjectOpts) error
	// ListBuckets lists all buckets.
	ListBuckets() ([]string, error)
	// CreateBucket creates a new bucket.
	CreateBucket(bucketName string) error
	// DeleteBucket deletes a bucket.
	DeleteBucket(bucketName string) error
	// Create Cloud Provider specific credentials.
	Credentials(userName, bucketName string) (string, string, error)
	// Delete a user
	DeleteUser(userName string) error
	// Get the Cloud Provider
	Provider() string
}

type ObjectOpts struct {
	CloudProvider string
	Endpoint      string
	Region        string
	AccessKey     string
	SecretKey     string
	TLS           bool
}

func NewObjectClient(ctx context.Context, opts *ObjectOpts) (ObjectClient, error) {
	switch opts.CloudProvider {
	case "gcs":
		g := &GCPObjectClient{ctx: ctx}
		if err := g.Auth(opts); err != nil {
			return nil, err
		}
		return g, nil
	case "s3":
		a := &AWSObjectClient{ctx: ctx}
		if err := a.Auth(opts); err != nil {
			return nil, err
		}
		return a, nil
	case "minio":
		m := &MinioObjectClient{ctx: ctx}
		if err := m.Auth(opts); err != nil {
			return nil, err
		}
		return m, nil
	default:
		return nil, fmt.Errorf("Cloud Provider %s, not supported", opts.CloudProvider)
	}
}

type GCPObjectClient struct {
	ObjectClient
	// Add any GCP specific fields here
	ctx        context.Context
	client     *storage.Client
	iam        *giam.Service
	projectID  string
	projectNum string
}

func (gcp *GCPObjectClient) Auth(opts *ObjectOpts) error {
	// Authenticate with GCP
	client, err := storage.NewClient(gcp.ctx)
	if err != nil {
		return err
	}
	gcp.client = client
	gcp.projectID, err = metadata.ProjectID()
	if err != nil {
		return err
	}
	gcp.projectNum, err = metadata.NumericProjectID()
	if err != nil {
		return err
	}
	gcp.iam, err = giam.NewService(gcp.ctx)
	if err != nil {
		return err
	}
	return nil
}

func (gcp *GCPObjectClient) Provider() string {
	return "gcp"
}

func (gcp *GCPObjectClient) Credentials(userName, bucketName string) (string, string, error) {
	found := false
	resp, err := gcp.iam.Projects.ServiceAccounts.List("projects/" + gcp.projectNum).Do()
	if err != nil {
		return "", "", err
	}
	var serviceAccount *giam.ServiceAccount
	for _, sa := range resp.Accounts {
		if sa.DisplayName == userName {
			found = true
			serviceAccount = sa
			break
		}
	}
	if !found {
		// Create a service account
		serviceAccount, err = gcp.iam.Projects.ServiceAccounts.Create("projects/"+gcp.projectID, &giam.CreateServiceAccountRequest{
			AccountId: userName,
			ServiceAccount: &giam.ServiceAccount{
				DisplayName: userName,
			},
		}).Do()
		if err != nil {
			return "", "", err
		}
	}

	// Attach the service account to the bucket
	bucket := gcp.client.Bucket(bucketName)
	policy, err := bucket.IAM().Policy(gcp.ctx)
	if err != nil {
		return "", "", err
	}
	id := "serviceAccount:" + userName + "@" + gcp.projectID + ".iam.gserviceaccount.com"
	var role gcpiam.RoleName = "roles/storage.objectAdmin"
	policy.Add(id, role)
	if err := bucket.IAM().SetPolicy(gcp.ctx, policy); err != nil {
		return "", "", err
	}
	key, err := gcp.client.CreateHMACKey(gcp.ctx, gcp.projectID, serviceAccount.Email)
	if err != nil {
		return "", "", err
	}

	return key.AccessID, key.Secret, nil
}

func (gcp *GCPObjectClient) DeleteUser(userName string) error {
	// Delete a user in GCP
	resp, err := gcp.iam.Projects.ServiceAccounts.List("projects/" + gcp.projectNum).Do()
	if err != nil {
		return err
	}
	for _, sa := range resp.Accounts {
		if sa.DisplayName == userName {
			_, err := gcp.iam.Projects.ServiceAccounts.Delete("projects/" + gcp.projectID + "/serviceAccounts/" + sa.Email).Do()
			if err != nil {
				return err
			}
			break
		}
	}
	return nil
}

func (gcp *GCPObjectClient) ListBuckets() ([]string, error) {
	// List all buckets in GCP
	it := gcp.client.Buckets(gcp.ctx, gcp.projectID)
	var buckets []string
	for {
		bucketAttrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		buckets = append(buckets, bucketAttrs.Name)
	}
	return buckets, nil
}

func (gcp *GCPObjectClient) CreateBucket(bucketName string) error {
	// Create a bucket in GCP
	return gcp.client.Bucket(bucketName).Create(gcp.ctx, gcp.projectID, nil)
}

func (gcp *GCPObjectClient) DeleteBucket(bucketName string) error {
	// Delete a bucket in GCP
	return gcp.client.Bucket(bucketName).Delete(gcp.ctx)
}

type AWSObjectClient struct {
	ObjectClient
	ctx    context.Context
	svc    *s3.Client
	iam    *iam.Client
	region string
}

func (aws *AWSObjectClient) Auth(opts *ObjectOpts) error {
	// Authenticate with AWS
	cfg, err := config.LoadDefaultConfig(aws.ctx, config.WithRegion(opts.Region))
	if err != nil {
		return err
	}
	aws.region = opts.Region

	// Using the Config value, create the DynamoDB client
	aws.svc = s3.NewFromConfig(cfg)
	aws.iam = iam.NewFromConfig(cfg)

	return nil
}

func (aws *AWSObjectClient) Provider() string {
	return "aws"
}

func (aws *AWSObjectClient) Credentials(userName, bucketName string) (string, string, error) {
	// Create User, and Policy in AWS
	// Create a user
	users, err := aws.iam.ListUsers(aws.ctx, &iam.ListUsersInput{})
	if err != nil {
		return "", "", err
	}
	found := false
	for _, user := range users.Users {
		if *user.UserName == userName {
			found = true
			break
		}
	}
	if !found {
		_, err := aws.iam.CreateUser(aws.ctx, &iam.CreateUserInput{
			UserName: &userName,
		})
		if err != nil {
			return "", "", err
		}
	}
	// Create a policy
	policy := fmt.Sprintf(`{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "ListObjectsInBucket",
            "Effect": "Allow",
            "Action": ["s3:ListBucket"],
            "Resource": ["arn:aws:s3:::%s"]
        },
        {
            "Sid": "AllObjectActions",
            "Effect": "Allow",
            "Action": "s3:*Object",
            "Resource": ["arn:aws:s3:::%s/*"]
        }
    ]
}`, bucketName, bucketName)
	policies, err := aws.iam.ListPolicies(aws.ctx, &iam.ListPoliciesInput{})
	if err != nil {
		return "", "", err
	}
	found = false
	for _, p := range policies.Policies {
		if *p.PolicyName == userName {
			found = true
			break
		}
	}
	if !found {
		p, err := aws.iam.CreatePolicy(aws.ctx, &iam.CreatePolicyInput{
			PolicyName:     &userName,
			PolicyDocument: &policy,
		})
		if err != nil {
			return "", "", err
		}
		// Attach the role to the user
		_, err = aws.iam.AttachUserPolicy(aws.ctx, &iam.AttachUserPolicyInput{
			PolicyArn: p.Policy.Arn,
			UserName:  &userName,
		})
		if err != nil {
			return "", "", err
		}
	}
	// Create access key for the user
	key, err := aws.iam.CreateAccessKey(aws.ctx, &iam.CreateAccessKeyInput{
		UserName: &userName,
	})
	if err != nil {
		return "", "", err
	}
	return *key.AccessKey.AccessKeyId, *key.AccessKey.SecretAccessKey, nil
}

func (aws *AWSObjectClient) DeleteUser(userName string) error {
	// Delete a user in AWS
	_, err := aws.iam.DeleteUser(aws.ctx, &iam.DeleteUserInput{
		UserName: &userName,
	})
	if err != nil {
		return err
	}

	// Delete the policy
	_, err = aws.iam.DeletePolicy(aws.ctx, &iam.DeletePolicyInput{
		PolicyArn: &userName,
	})
	if err != nil {
		return err
	}
	return nil
}

func (aws *AWSObjectClient) ListBuckets() ([]string, error) {
	buckets, err := aws.svc.ListBuckets(aws.ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, err
	}
	bucketNames := []string{}
	for _, bucket := range buckets.Buckets {
		bucketNames = append(bucketNames, *bucket.Name)
	}
	return bucketNames, nil
}

func (aws *AWSObjectClient) CreateBucket(bucketName string) error {
	// Create a bucket in AWS
	_, err := aws.svc.CreateBucket(aws.ctx, &s3.CreateBucketInput{
		Bucket: &bucketName,
		CreateBucketConfiguration: &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(aws.region),
		},
	})
	if err != nil {
		return err
	}
	return nil
}

func (aws *AWSObjectClient) DeleteBucket(bucketName string) error {
	// Delete a bucket in AWS
	_, err := aws.svc.DeleteBucket(aws.ctx, &s3.DeleteBucketInput{
		Bucket: &bucketName,
	})
	if err != nil {
		return err
	}
	return nil
}

type MinioObjectClient struct {
	ObjectClient
	ctx    context.Context
	client *minio.Client
}

func (mc *MinioObjectClient) Auth(opts *ObjectOpts) error {
	// Authenticate with Minio
	var err error
	mc.client, err = minio.New(opts.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(opts.AccessKey, opts.SecretKey, ""),
		Secure: opts.TLS,
	})
	if err != nil {
		return err
	}
	return nil
}

func (mc *MinioObjectClient) Provider() string {
	return "minio"
}

func (mc *MinioObjectClient) Credentials(userName, bucketName string) (string, string, error) {
	// Get Minio credentials
	return "", "", nil
}

func (mc *MinioObjectClient) DeleteUser(userName string) error {
	// Delete a user in Minio
	return nil
}

func (mc *MinioObjectClient) ListBuckets() ([]string, error) {
	// List all buckets in Minio
	buckets, err := mc.client.ListBuckets(mc.ctx)
	if err != nil {
		return nil, err
	}
	bucketNames := []string{}
	for _, bucket := range buckets {
		bucketNames = append(bucketNames, bucket.Name)
	}
	return bucketNames, nil
}

func (mc *MinioObjectClient) CreateBucket(bucketName string) error {
	// Create a bucket in Minio
	return mc.client.MakeBucket(mc.ctx, bucketName, minio.MakeBucketOptions{})
}

func (mc *MinioObjectClient) DeleteBucket(bucketName string) error {
	// Delete a bucket in Minio
	return mc.client.RemoveBucket(mc.ctx, bucketName)
}
