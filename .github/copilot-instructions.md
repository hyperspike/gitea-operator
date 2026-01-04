# Gitea Operator - AI Coding Assistant Guide

## Project Architecture

This is a Kubernetes operator built with Kubebuilder that manages Gitea instances and related resources. The operator orchestrates multiple components:

- **Main Gitea CR** ([api/v1/gitea_types.go](api/v1/gitea_types.go)): Provisions StatefulSet, PostgreSQL (via Zalando or CloudNative-PG), Valkey cache, object storage (S3/GCS), Ingress, and TLS certificates
- **Dependent CRs**: User, Org, Repo, Runner, Auth, MigrateRepo - all reference a parent Gitea instance via `InstanceType` struct ([api/v1/refs.go](api/v1/refs.go))
- **Cross-namespace references**: Resources can reference Gitea instances in different namespaces using `namespace` field in refs

### Key Dependencies & Integration Points

- **Database**: Supports two providers via `spec.postgres.provider`: `"zalando"` (default) or `"cnpg"` (CloudNative-PG)
- **Cache**: Optional Valkey cluster (`spec.valkey: true`) managed by [hyperspike.io/valkey-operator](https://github.com/hyperspike/valkey-operator)
- **TLS**: Cert-manager integration for internal TLS (service-to-service) when `spec.tls: true`
- **Object Storage**: AWS S3 or GCP GCS via [internal/client/object.go](internal/client/object.go) with automatic bucket/IAM user provisioning
- **Monitoring**: ServiceMonitor resources for both Gitea and PostgreSQL when `spec.prometheus: true`

### Reconciliation Pattern

Controllers follow this pattern (see [internal/controller/gitea_controller.go](internal/controller/gitea_controller.go:2400-2562)):
1. Upsert dependencies (PostgreSQL, Valkey)
2. Wait for dependencies to become ready (poll with `RequeueAfter: 5s`)
3. Generate secrets and configuration
4. Upsert StatefulSet and supporting resources
5. Wait for pod readiness and API availability
6. Provision admin token via Gitea API

**Finalizers**: Used for cleanup of external resources (object storage buckets, cloud IAM users) - see `objectFinalizer` pattern in gitea_controller.go

## Development Workflows

### Local Development Setup

```bash
# Start local Minikube cluster with Cilium CNI
make minikube  # Uses hack/minikube.sh and hack/quickstart.sh

# Build and deploy to local registry
TAG=latest
make docker-build IMG=localhost:5000/controller:$TAG
docker push localhost:5000/controller:$TAG
make IMG=localhost:5000/controller:$TAG build-installer
kubectl apply -f dist/install.yaml

# Apply sample Gitea instance
kubectl apply -f config/samples/v1_gitea.yaml
```

### Code Generation Requirements

**Always run after modifying API types or RBAC annotations:**
```bash
make manifests  # Regenerates CRDs, RBAC, webhooks from controller comments
make generate   # Regenerates DeepCopy methods
```

### Testing

```bash
make test        # Unit tests (excludes e2e)
make test-e2e    # End-to-end tests in test/e2e/
make lint        # golangci-lint
```

### Cloud Provider Environments

The operator supports AWS, GCP, and Azure via OpenTofu configs in `hack/{aws,gcp,azure}/`:
```bash
make aws          # Provision EKS cluster
make quickstart-aws  # Deploy operator with dependencies
make aws-destroy  # Teardown
```

## Project-Specific Conventions

### API Design Patterns

**Reference Types**: All dependent resources use typed refs from [api/v1/refs.go](api/v1/refs.go):
```go
type UserRef struct {
    Name      string            `json:"name"`
    Namespace string            `json:"namespace,omitempty"`  // Defaults to same namespace
    Labels    map[string]string `json:"labels,omitempty"`
}
```

**Status Conditions**: Use `meta.SetStatusCondition` for standardized condition tracking:
```go
condition := metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "ApiUp", Message: "Api Up"}
meta.SetStatusCondition(&gitea.Status.Conditions, condition)
```

### Controller Patterns

**Client Construction**: Use [internal/client/gitea.go](internal/client/gitea.go) to build Gitea SDK clients with TLS support:
```go
client, gitea, err := hclient.Build(ctx, r.Client, &instance, namespace)
// Returns nil client if gitea.Status.Ready == false - caller should requeue
```

**Resource Upsert Pattern**: All controllers follow "create-if-not-found" pattern:
```go
err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, resource)
if err != nil && errors.IsNotFound(err) {
    r.Recorder.Event(parent, "Normal", "Creating", fmt.Sprintf("Creating %s", name))
    if err := r.Create(ctx, resource); err != nil {
        return err
    }
}
```

**Database Provider Detection**: Always check `gitea.Spec.Postgres.Provider`:
- `"cnpg"`: Use `Cluster` CR, secret named `{gitea.Name}-db-app`
- `"zalando"` (or empty): Use `Postgresql` CR, secret named `{gitea.Name}.{gitea.Name}-{gitea.Name}.credentials.postgresql.acid.zalan.do`

### Configuration & Secrets

**Gitea Configuration**: Split across three secrets in [internal/controller/gitea_controller.go](internal/controller/gitea_controller.go):
- `{name}-init`: Init container scripts
- `{name}-config`: env2ini configuration script
- `{name}-inline-config`: INI section overrides (cache, database, server, etc.)

**Admin Credentials**: Stored in `{name}-admin` secret with `username`, `password`, and `token` (token generated post-deployment)

### Tool Dependencies

Tools are version-pinned and installed to `bin/` directory:
- `controller-gen`: v0.19.0
- `kustomize`: v5.8.0
- `golangci-lint`: v2.6.2

Always use `$(CONTROLLER_GEN)` makefile variables, not global tools.

## Common Tasks

**Add a new CRD**: 
1. Create `api/v1/{name}_types.go` with kubebuilder markers
2. Create `internal/controller/{name}_controller.go`
3. Register in [cmd/main.go](cmd/main.go)
4. Run `make manifests generate`

**Add RBAC permissions**: Add `+kubebuilder:rbac` comments above `Reconcile()` method, then `make manifests`

**Update dependencies**: Modify `go.mod`, run `go mod tidy`, and verify with `make test`

**Debug reconciliation**: Controllers use `ctrl.Log.FromContext(ctx)` - increase verbosity with `--zap-log-level=debug` in manager args

## Important Files

- [internal/controller/gitea_controller.go](internal/controller/gitea_controller.go): Main reconciliation logic (~2500 lines)
- [internal/client/gitea.go](internal/client/gitea.go): Gitea API client with TLS
- [internal/client/object.go](internal/client/object.go): S3/GCS bucket management
- [hack/quickstart.sh](hack/quickstart.sh): Installs operator dependencies (Postgres, Valkey, cert-manager, Prometheus)
- [config/crd/bases/](config/crd/bases/): Generated CRD manifests (never edit directly)
