/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	g "code.gitea.io/sdk/gitea"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	log "sigs.k8s.io/controller-runtime/pkg/log"

	hyperv1 "hyperspike.io/gitea-operator/api/v1"
	hclient "hyperspike.io/gitea-operator/internal/client"
)

// MigrateRepoReconciler reconciles a MigrateRepo object
type MigrateRepoReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	h      *hclient.Client
}

// +kubebuilder:rbac:groups=hyperspike.io,resources=migraterepoes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hyperspike.io,resources=migraterepoes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=hyperspike.io,resources=migraterepoes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MigrateRepo object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile
func (r *MigrateRepoReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	repo := &hyperv1.MigrateRepo{}
	repo.Spec.Org = &hyperv1.OrgRef{}
	repo.Spec.User = &hyperv1.UserRef{}
	logger.Info("Fetching MigrateRepo", "namespacedName", req.NamespacedName)
	if err := r.Get(ctx, req.NamespacedName, repo); err != nil {
		logger.Error(err, "unable to fetch MigrateRepo")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	h, gitea, err := hclient.BuildFromOrg(ctx, r.Client, repo.Spec.Org, repo.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	if h == nil || gitea == nil {
		return ctrl.Result{Requeue: true, RequeueAfter: 10 * time.Second}, fmt.Errorf("missing Gitea client")
	}
	r.h = h
	isRepoMarkedToBeDeleted := repo.GetDeletionTimestamp() != nil
	if isRepoMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(repo, repoFinalizer) {
			logger.Info("Deleting migrated repo", "repo", repo.Name)
			if err := r.deleteRepo(repo); err != nil {
				return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, err
			}
			controllerutil.RemoveFinalizer(repo, repoFinalizer)
			if err := r.Update(ctx, repo); err != nil {
				return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, err
			}
		}
		return ctrl.Result{}, nil
	}
	if repo.Spec.Org != nil {
		gRepo, resp, err := r.h.GetRepo(repo.Spec.Org.Name, repo.Name)
		if err != nil && resp.StatusCode != 404 {
			return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, err
		}
		if resp.StatusCode == 200 {
			logger.Info("Repo already exists, skipping migration", "repo", gRepo.FullName)
			return ctrl.Result{}, nil
		}
		logger.Info("creating migrate repos")
		serviceType, err := lookupGitService(repo.Spec.Service)
		if err != nil {
			return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, err
		}
		_, _, err = r.h.MigrateRepo(g.MigrateRepoOption{
			RepoName:    repo.Name,
			RepoOwner:   repo.Spec.Org.Name,
			CloneAddr:   repo.Spec.URL,
			Description: repo.Spec.Description,
			Private:     repo.Spec.Private,
			Service:     serviceType,
			Mirror:      repo.Spec.Mirror,
		})
		if err != nil {
			return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, err
		}
		if !controllerutil.ContainsFinalizer(repo, repoFinalizer) {
			controllerutil.AddFinalizer(repo, repoFinalizer)
			if err := r.Update(ctx, repo); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func lookupGitService(service string) (g.GitServiceType, error) {
	switch service {
	case "git":
		return g.GitServicePlain, nil
	case "github":
		return g.GitServiceGithub, nil
	case "gitlab":
		return g.GitServiceGitlab, nil
	case "gitea":
		return g.GitServiceGitea, nil
	case "gogs":
		return g.GitServiceGogs, nil
	default:
		return "", fmt.Errorf("unsupported git service type: %s", service)
	}
}

func (r *MigrateRepoReconciler) deleteRepo(repo *hyperv1.MigrateRepo) error {
	_, err := r.h.DeleteRepo(repo.Spec.Org.Name, repo.Name)
	return err
}

// SetupWithManager sets up the controller with the Manager.
func (r *MigrateRepoReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hyperv1.MigrateRepo{}).
		Named("migraterepo").
		Complete(r)
}
