/*
Copyright 2024.

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
	hclient "hyperspike.io/gitea-operator/internal/client"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	hyperv1 "hyperspike.io/gitea-operator/api/v1"
)

// RepoReconciler reconciles a Repo object
type RepoReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	h      *hclient.Client
}

const repoFinalizer = "repo.hyperspike.io/finalizer"

// +kubebuilder:rbac:groups=hyperspike.io,resources=repoes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hyperspike.io,resources=repoes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=hyperspike.io,resources=repoes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Repo object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
//
//nolint:gocyclo
func (r *RepoReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	repo := &hyperv1.Repo{}
	repo.Spec.Org = &hyperv1.OrgRef{}
	repo.Spec.User = &hyperv1.UserRef{}
	logger.Info("repo: fetching self")
	if err := r.Get(ctx, req.NamespacedName, repo); err != nil {
		logger.Error(err, "failed to get repo")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	h, gitea, err := hclient.BuildFromOrg(ctx, r.Client, repo.Spec.Org, repo.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	if h == nil && gitea == nil {
		return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, fmt.Errorf("failed to get gitea client")
	}
	r.h = h

	isRepoMarkedToBeDeleted := repo.GetDeletionTimestamp() != nil
	if isRepoMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(repo, repoFinalizer) {
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

	if repo.Spec.User == nil && repo.Spec.Org == nil {
		return ctrl.Result{}, fmt.Errorf("repo must be created for either user or org")
	}
	if repo.Spec.User != nil && repo.Spec.Org != nil {
		return ctrl.Result{}, fmt.Errorf("repo cannot be created for both user and org")
	}
	want := &g.Repository{
		Description: repo.Spec.Description,
		Private:     repo.Spec.Private,
	}
	if repo.Spec.Org != nil {
		gRepo, resp, err := r.h.GetRepo(repo.Spec.Org.Name, repo.Name)
		if err != nil && resp.StatusCode != 404 {
			return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, err
		}
		if resp.StatusCode == 200 {
			if !compareRepo(want, gRepo) {
				logger.Info("updating repo")
				if _, _, err := r.h.EditRepo(repo.Spec.Org.Name, repo.Name, g.EditRepoOption{}); err != nil {
					return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, err
				}
			}
		}
		logger.Info("creating repo")
		_, _, err = r.h.CreateOrgRepo(repo.Spec.Org.Name, g.CreateRepoOption{
			Name:          repo.Name,
			Description:   repo.Spec.Description,
			Private:       repo.Spec.Private,
			Readme:        repo.Spec.Readme,
			License:       repo.Spec.License,
			DefaultBranch: repo.Spec.DefaultBranch,
			Gitignores:    repo.Spec.Gitignores,
			Template:      repo.Spec.Template,
			AutoInit:      repo.Spec.AutoInit,
			IssueLabels:   repo.Spec.IssueLabels,
		})
		if err != nil {
			return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, err
		}
		if repo.Spec.Mirror != nil && repo.Spec.Mirror.RemoteURL != "" {
			_, _, err = r.h.PushMirrors(repo.Spec.Org.Name, repo.Name, g.CreatePushMirrorOption{
				Interval:       repo.Spec.Mirror.Interval,
				RemoteAddress:  repo.Spec.Mirror.RemoteURL,
				SyncONCommit:   repo.Spec.Mirror.SyncOnCommit,
				RemoteUsername: repo.Spec.Mirror.UserName,
				RemotePassword: repo.Spec.Mirror.Password,
			})
			if err != nil {
				logger.Error(err, "failed to create push mirror")
			}
		}
		if !repo.Status.Provisioned {
			repo.Status.Provisioned = true
			if err := r.Client.Status().Update(ctx, repo); err != nil {
				logger.Error(err, "Failed to update Repo status")
				return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, err
			}
		}
		if !controllerutil.ContainsFinalizer(repo, repoFinalizer) {
			controllerutil.AddFinalizer(repo, repoFinalizer)
			if err := r.Update(ctx, repo); err != nil {
				return ctrl.Result{}, err
			}
		}
	}
	if repo.Spec.User != nil {
		_, _, err := r.h.CreateRepo(g.CreateRepoOption{})
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	if !repo.Status.Provisioned {
		repo.Status.Provisioned = true
		if err := r.Client.Status().Update(ctx, repo); err != nil {
			logger.Error(err, "Failed to update Repo status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *RepoReconciler) deleteRepo(repo *hyperv1.Repo) error {
	_, err := r.h.DeleteRepo(repo.Spec.Org.Name, repo.Name)
	return err
}

func compareRepo(fetch, req *g.Repository) bool {
	if fetch.Description != req.Description {
		return false
	}
	if fetch.Private != req.Private {
		return false
	}

	return true
}

// SetupWithManager sets up the controller with the Manager.
func (r *RepoReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hyperv1.Repo{}).
		Complete(r)
}
