# Gemini Code Assistant Context

This document provides context for the Gemini Code Assistant to understand the `gitea-operator` project.

## Project Overview

This is a Kubernetes operator for deploying and managing [Gitea](https://gitea.io/), a self-hosted Git service. The operator is built with the [Kubebuilder](https://book.kubebuilder.io/) framework and is written in Go.

Its primary purpose is to automate the lifecycle of Gitea instances on Kubernetes. This includes:

*   Deploying a Gitea `StatefulSet`.
*   Managing dependencies like PostgreSQL (via the [Zalando Postgres Operator](https://github.com/zalando/postgres-operator)) and Valkey (via the `valkey-operator`).
*   Configuring Gitea through a `Gitea` Custom Resource, including settings for database, caching, object storage, ingress, and TLS.
*   Creating and managing other Gitea-related resources like users, organizations, and repositories through their own Custom Resources (`User`, `Org`, `Repo`, etc.).
*   Setting up monitoring with Prometheus via `ServiceMonitor` resources.
*   Managing TLS certificates with `cert-manager`.

### Architecture

*   **Language:** Go
*   **Framework:** Kubebuilder / controller-runtime
*   **Custom Resources (CRDs):**
    *   `Gitea`: Manages the main Gitea instance.
    *   `User`: Manages Gitea users.
    *   `Org`: Manages Gitea organizations.
    *   `Repo`: Manages Gitea repositories.
    *   `Runner`: Manages Gitea Actions runners.
    *   `Auth`: Manages authentication sources.
    *   `MigrateRepo`: Manages repository migrations.
*   **Key Dependencies:**
    *   `k8s.io/client-go`: For interacting with the Kubernetes API.
    *   `sigs.k8s.io/controller-runtime`: The core operator framework.
    *   `code.gitea.io/sdk/gitea`: Gitea API client.
    *   `github.com/zalando/postgres-operator`: For database management.
    *   `hyperspike.io/valkey-operator`: For cache management.
    *   `github.com/cert-manager/cert-manager`: For TLS certificate management.
    *   `github.com/prometheus-operator/prometheus-operator`: For monitoring.

## Building and Running

The project uses a `Makefile` to automate common tasks.

### Key `make` targets:

*   `make all` or `make build`: Builds the operator manager binary.
*   `make run`: Runs the controller on the local machine against the cluster configured in `~/.kube/config`.
*   `make test`: Runs unit tests.
*   `make test-e2e`: Runs end-to-end tests.
*   `make docker-build`: Builds the operator container image.
*   `make docker-push`: Pushes the container image to a registry.
*   `make install`: Installs the CRDs into a Kubernetes cluster.
*   `make deploy`: Deploys the operator to a Kubernetes cluster.

### Local Development Quickstart

1.  **Start a local Kubernetes cluster:**
    ```bash
    make minikube
    ```
2.  **Build and deploy the operator:**
    ```bash
    TAG=latest; make docker-build IMG=localhost:5000/controller:$TAG; docker push localhost:5000/controller:$TAG ; make IMG=localhost:5000/controller:$TAG build-installer  ; kubectl apply -f dist/install.yaml
    ```
3.  **Apply a sample Gitea resource:**
    ```bash
    kubectl apply -f config/samples/v1_gitea.yaml
    ```

## Development Conventions

*   **Code Generation:** The operator uses `controller-gen` to generate boilerplate code, CRDs, and RBAC manifests. Run `make manifests` and `make generate` after modifying API types or controller RBAC comments.
*   **Linting:** `make lint` runs `golangci-lint` to check for code style and quality issues.
*   **Testing:** Unit tests are located alongside the code they test (e.g., `gitea_controller_test.go`). End-to-end tests are in the `/test/e2e` directory.
*   **Dependencies:** Go modules are used for dependency management. Tool dependencies like `kustomize` and `controller-gen` are managed via the `Makefile` and installed in the `/bin` directory.
