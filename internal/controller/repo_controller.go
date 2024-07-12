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
	"time"

	g "code.gitea.io/sdk/gitea"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	hyperv1 "hyperspike.io/gitea-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
)

// RepoReconciler reconciles a Repo object
type RepoReconciler struct {
	client.Client
	Scheme *runtime.Scheme
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

	gClient, gitea, err := r.buildClient(ctx, repo.Spec.Org, repo.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	if gClient == nil && gitea == nil {
		return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, nil
	}

	isRepoMarkedToBeDeleted := repo.GetDeletionTimestamp() != nil
	if isRepoMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(repo, repoFinalizer) {
			if err := r.deleteRepo(gClient, repo); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(repo, repoFinalizer)
			if err := r.Update(ctx, repo); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if repo.Spec.User == nil && repo.Spec.Org == nil {
		return ctrl.Result{}, nil
	}
	if repo.Spec.User != nil && repo.Spec.Org != nil {
		return ctrl.Result{}, nil
	}
	want := &g.Repository{
		Description: repo.Spec.Description,
		Private:     repo.Spec.Private,
	}
	if repo.Spec.Org != nil {
		gRepo, resp, err := gClient.GetRepo(repo.Spec.Org.Name, repo.Name)
		if err != nil && resp.StatusCode != 404 {
			return ctrl.Result{}, err
		}
		if resp.StatusCode == 200 {
			if !compareRepo(want, gRepo) {
				logger.Info("updating repo")
				if _, _, err := gClient.EditRepo(repo.Spec.Org.Name, repo.Name, g.EditRepoOption{}); err != nil {
					return ctrl.Result{}, err
				}
			}
		}
		logger.Info("creating repo")
		_, _, err = gClient.CreateOrgRepo(repo.Spec.Org.Name, g.CreateRepoOption{
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
			return ctrl.Result{}, nil
		}
		if !repo.Status.Provisioned {
			repo.Status.Provisioned = true
			if err := r.Client.Status().Update(ctx, repo); err != nil {
				logger.Error(err, "Failed to update Repo status")
				return ctrl.Result{}, nil
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
		_, _, err := gClient.CreateRepo(g.CreateRepoOption{})
		if err != nil {
			return ctrl.Result{}, nil
		}
	}
	if !repo.Status.Provisioned {
		repo.Status.Provisioned = true
		if err := r.Client.Status().Update(ctx, repo); err != nil {
			logger.Error(err, "Failed to update Repo status")
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, nil
}

func (r *RepoReconciler) deleteRepo(gClient *g.Client, repo *hyperv1.Repo) error {
	_, err := gClient.DeleteRepo(repo.Spec.Org.Name, repo.Name)
	return err
}

func (r *RepoReconciler) buildClient(ctx context.Context, instance *hyperv1.OrgRef, ns string) (*g.Client, *hyperv1.Gitea, error) {
	logger := log.FromContext(ctx)

	orgName := instance.Name
	orgNamespace := instance.Namespace
	if orgNamespace == "" {
		orgNamespace = ns
	}
	org := &hyperv1.Org{}
	if err := r.Get(ctx, types.NamespacedName{Name: orgName, Namespace: orgNamespace}, org); err != nil {
		logger.Error(err, "failed to get gitea")
		return nil, nil, err
	}

	git := &hyperv1.Gitea{}
	gitName := org.Spec.Instance.Name
	gitNamespace := org.Spec.Instance.Namespace
	if gitNamespace == "" {
		gitNamespace = ns
	}
	if err := r.Get(ctx, types.NamespacedName{Name: gitName, Namespace: gitNamespace}, git); err != nil {
		logger.Error(err, "failed to get gitea")
		return nil, nil, err
	}
	if !git.Status.Ready {
		return nil, nil, nil
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      git.Name + "-admin",
			Namespace: git.Namespace,
		},
	}
	if err := r.Get(ctx, types.NamespacedName{Name: git.Name + "-admin", Namespace: git.Namespace}, secret); err != nil {
		logger.Error(err, "failed getting admin secret "+git.Name+"-admin ")
		return nil, nil, err
	}
	url := "http://" + git.Name + "." + git.Namespace + ".svc"
	gClient, err := g.NewClient(url, g.SetContext(ctx), g.SetToken(string(secret.Data["token"])))
	if err != nil {
		logger.Error(err, "failed to create client for "+url)
		return nil, nil, err
	}
	_, _, err = gClient.ServerVersion()
	if err != nil {
		logger.Error(err, "failed to get server version "+url)
		return nil, nil, err
	}
	return gClient, git, nil
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
