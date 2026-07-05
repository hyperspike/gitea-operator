# Gemini Code Assistant Context

This document provides extensive context for the Gemini Code Assistant to understand, build, and debug the `gitea-operator` project.

## Project Overview

This is a Kubernetes operator for deploying and managing [Gitea](https://gitea.io/), a self-hosted Git service. The operator is built with the Go [Kubebuilder](https://book.kubebuilder.io/) framework and `controller-runtime`.

Its primary purpose is to automate the lifecycle of Gitea instances on Kubernetes. This includes:
*   Deploying a Gitea `StatefulSet`.
*   Managing database backends via either the [Zalando Postgres Operator](https://github.com/zalando/postgres-operator) or [CloudNativePG](https://github.com/cloudnative-pg/cloudnative-pg).
*   Managing cache backends using the `valkey-operator`.
*   Configuring Gitea configurations, object storage, ingress, and TLS.
*   Automating Gitea-related resources like users, organizations, repositories, auth providers, and repository migrations through custom resources.
*   Setting up monitoring with Prometheus via `ServiceMonitor` resources.
*   Managing TLS certificates with `cert-manager`.

## Architecture & Code Map

*   **Language:** Go (v1.26.3)
*   **Framework:** Kubebuilder / controller-runtime (v0.24.1)
*   **Controllers:** Located under [internal/controller](file:///home/dan/code/hyperspike/gitea-operator/internal/controller)
    *   [GiteaReconciler](file:///home/dan/code/hyperspike/gitea-operator/internal/controller/gitea_controller.go#L104): Main coordinator that orchestrates Gitea, Valkey, Postgres, Ingress, TLS, and Prometheus.
    *   `UserReconciler` ([user_controller.go](file:///home/dan/code/hyperspike/gitea-operator/internal/controller/user_controller.go)): Provisions/manages Gitea users via Gitea API.
    *   `OrgReconciler` ([org_controller.go](file:///home/dan/code/hyperspike/gitea-operator/internal/controller/org_controller.go)): Provisions/manages Gitea organizations.
    *   `RepoReconciler` ([repo_controller.go](file:///home/dan/code/hyperspike/gitea-operator/internal/controller/repo_controller.go)): Provisions/manages Gitea repositories.
    *   `RunnerReconciler` ([runner_controller.go](file:///home/dan/code/hyperspike/gitea-operator/internal/controller/runner_controller.go)): Configures and scales Actions Runners.
    *   `AuthReconciler` ([auth_controller.go](file:///home/dan/code/hyperspike/gitea-operator/internal/controller/auth_controller.go)): Installs OIDC authentication sources.
    *   `MigrateRepoReconciler` ([migraterepo_controller.go](file:///home/dan/code/hyperspike/gitea-operator/internal/controller/migraterepo_controller.go)): Handles migrating repositories from external sources.

---

## Custom Resource Definitions (CRDs)

### 1. `Gitea` Resource
Manages the core Gitea installation.
*   **Spec definition:** [GiteaSpec](file:///home/dan/code/hyperspike/gitea-operator/api/v1/gitea_types.go#L27)
*   **Key Fields:**
    *   `ingress`: Configures the Hostname and Ingress annotations.
    *   `postgres`: Selects postgres provider (`zalando` or `cnpg`).
    *   `valkey`: Enables Valkey caching backend (boolean).
    *   `objectStorage`: Integrates MinIO, GCS, or S3.
    *   `tls`: Enables TLS (boolean).
    *   `certIssuer` / `certIssuerType`: Integrates Cert-Manager issuers.
    *   `rootless`: Runs Gitea in rootless mode (boolean).
    *   `prometheus`: Enables Prometheus metrics (boolean).

#### Sample YAML:
```yaml
apiVersion: hyperspike.io/v1
kind: Gitea
metadata:
  name: gitea
spec:
  image: "gitea/gitea:1.26.2"
  rootless: false
  postgres:
    provider: cnpg  # Options: zalando, cnpg
  valkey: true
  tls: true
  certIssuer: letsencrypt-prod
  certIssuerType: ClusterIssuer
  ingress:
    host: git.local
    annotations:
      kubernetes.io/ingress.class: nginx
```

### 2. `User` Resource
Manages Gitea users automatically via the Gitea API.
*   **Spec definition:** [UserSpec](file:///home/dan/code/hyperspike/gitea-operator/api/v1/user_types.go#L28)
*   **Key Fields:**
    *   `email`: Email address (required).
    *   `password`: Reference to a secret key selector.
    *   `admin`: Make the user an administrator (boolean).
    *   `sshkeys`: List of public SSH keys.
    *   `instance`: Reference to the `Gitea` instance target.

#### Sample YAML:
```yaml
apiVersion: hyperspike.io/v1
kind: User
metadata:
  name: admin-user
spec:
  email: admin@git.local
  admin: true
  password:
    name: user-passwords
    key: admin-pwd
  instance:
    name: gitea
```

### 3. `Org` Resource
Manages Gitea organizations and team member permissions.
*   **Spec definition:** [OrgSpec](file:///home/dan/code/hyperspike/gitea-operator/api/v1/org_types.go#L27)
*   **Key Fields:**
    *   `fullname`: Human-readable name.
    *   `teams`: List of teams, permission level (`none`, `read`, `write`, `admin`, `owner`), and members.
    *   `instance`: Reference to the `Gitea` instance.

#### Sample YAML:
```yaml
apiVersion: hyperspike.io/v1
kind: Org
metadata:
  name: dev-org
spec:
  fullname: "Development Organization"
  description: "Dev group teams and repos"
  visibility: public
  instance:
    name: gitea
  teams:
    - name: Developers
      permission: write
      members:
        - admin-user
```

### 4. `Repo` Resource
Creates and configures Gitea repositories.
*   **Spec definition:** [RepoSpec](file:///home/dan/code/hyperspike/gitea-operator/api/v1/repo_types.go#L27)
*   **Key Fields:**
    *   `private`: Set repository visibility (boolean).
    *   `auto_init`: Initialize with README (boolean).
    *   `push_mirror`: Configures remote push mirror endpoint, username, password, and sync intervals.
    *   `user` / `org`: Ownership assignment.

#### Sample YAML:
```yaml
apiVersion: hyperspike.io/v1
kind: Repo
metadata:
  name: main-application
spec:
  private: true
  auto_init: true
  org:
    name: dev-org
```

### 5. `Runner` Resource
Deploys and registers Gitea Actions runners to the cluster.
*   **Spec definition:** [RunnerSpec](file:///home/dan/code/hyperspike/gitea-operator/api/v1/runner_types.go#L27)
*   **Key Fields:**
    *   `replicas`: Number of running instances.
    *   `rootless`: Runs runners securely in rootless mode.
    *   `org` or `instance`: Scope of runner registration (org-level or global).

#### Sample YAML:
```yaml
apiVersion: hyperspike.io/v1
kind: Runner
metadata:
  name: org-runner
spec:
  replicas: 2
  rootless: true
  org:
    name: dev-org
```

### 6. `Auth` Resource
Configures external authentication providers (e.g. OIDC) on Gitea.
*   **Spec definition:** [AuthSpec](file:///home/dan/code/hyperspike/gitea-operator/api/v1/auth_types.go#L27)
*   **Key Fields:**
    *   `provider`: Auth provider type (e.g. OpenID Connect).
    *   `clientID` / `clientSecret`: SecretKeySelector containing OIDC client credentials.
    *   `autoDiscoveryURL`: Discovery endpoint.
    *   `instance`: Target `Gitea` instance reference.

#### Sample YAML:
```yaml
apiVersion: hyperspike.io/v1
kind: Auth
metadata:
  name: keycloak-auth
spec:
  provider: "openidConnect"
  autoDiscoveryURL: "https://keycloak.example.com/realms/git"
  clientID:
    name: auth-secrets
    key: client-id
  clientSecret:
    name: auth-secrets
    key: client-secret
  instance:
    name: gitea
```

### 7. `MigrateRepo` Resource
Migrates existing repositories from external hosts (GitHub, GitLab, Gitea, etc.).
*   **Spec definition:** [MigrateRepoSpec](file:///home/dan/code/hyperspike/gitea-operator/api/v1/migraterepo_types.go#L26)
*   **Key Fields:**
    *   `url`: Source URL of external repository.
    *   `service`: Provider name (`git`, `github`, `gitlab`, `gogs`, `gitea`).
    *   `mirror`: Keep repo synced as a mirror (boolean).
    *   `user` / `org`: Ownership assignment.

#### Sample YAML:
```yaml
apiVersion: hyperspike.io/v1
kind: MigrateRepo
metadata:
  name: migrate-sample
spec:
  url: "https://github.com/someuser/somerepo.git"
  service: github
  mirror: false
  org:
    name: dev-org
```

---

## Building and Running

The project leverages a `Makefile` for developer tasks:

*   `make all` or `make build`: Compiles the operator controller binary.
*   `make run`: Runs the controller locally using the active `KUBECONFIG`.
*   `make test`: Runs unit tests locally.
*   `make test-e2e`: Runs E2E integration tests.
*   `make manifests`: Runs `controller-gen` to generate CRD manifests, cluster roles, and webhook configurations.
*   `make generate`: Runs `controller-gen` to generate code like `zz_generated.deepcopy.go`.
*   `make lint`: Runs `golangci-lint` to check code style and configuration.
*   `make install`: Registers the CRDs on the cluster.
*   `make deploy`: Installs the controller manager to the cluster.

### Local Development Quickstart

1.  **Boot local Kubernetes cluster:**
    ```bash
    make minikube
    ```
2.  **Build, publish, and deploy operator:**
    ```bash
    TAG=latest; make docker-build IMG=localhost:5000/controller:$TAG; docker push localhost:5000/controller:$TAG ; make IMG=localhost:5000/controller:$TAG build-installer  ; kubectl apply -f dist/install.yaml
    ```
3.  **Apply a sample configuration:**
    ```bash
    kubectl apply -f config/samples/v1_gitea.yaml
    ```
