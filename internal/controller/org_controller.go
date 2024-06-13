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

// OrgReconciler reconciles a Org object
type OrgReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const orgFinalizer = "org.hyperspike.io/finalizer"

// +kubebuilder:rbac:groups=hyperspike.io,resources=orgs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hyperspike.io,resources=orgs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=hyperspike.io,resources=orgs/finalizers,verbs=update
// +kubebuilder:rbac:groups=hyperspike.io,resources=gitea,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Org object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *OrgReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	org := &hyperv1.Org{}
	if err := r.Get(ctx, req.NamespacedName, org); err != nil {
		logger.Error(err, "failed to get org")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !controllerutil.ContainsFinalizer(org, orgFinalizer) {
		controllerutil.AddFinalizer(org, orgFinalizer)
		if err := r.Update(ctx, org); err != nil {
			logger.Error(err, "failed to add finalizers")
			return ctrl.Result{}, err
		}
	}
	gClient, gitea, err := r.buildClient(ctx, org.Spec.Instance, org.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	if gClient == nil && gitea == nil {
		return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, nil
	}
	isRepoMarkedToBeDeleted := org.GetDeletionTimestamp() != nil
	if isRepoMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(org, orgFinalizer) {
			if err := r.deleteOrg(gClient, org); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(org, orgFinalizer)
			err := r.Update(ctx, org)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}
	var vis g.VisibleType
	switch org.Spec.Visibility {
	case "public":
		vis = g.VisibleTypePublic
	case "limited":
		vis = g.VisibleTypeLimited
	case "private":
		vis = g.VisibleTypePrivate
	default:
		vis = g.VisibleTypePrivate
	}
	website := org.Spec.Website
	if website == "" && gitea.Spec.Hostname != "" {
		website = "https://" + gitea.Spec.Hostname + "/" + org.Name
	}
	want := &g.Organization{
		UserName:    org.Name,
		FullName:    org.Spec.FullName,
		Description: org.Spec.Description,
		Website:     website,
		Location:    org.Spec.Location,
		Visibility:  org.Spec.Visibility,
	}
	o, resp, err := gClient.GetOrg(org.Name)
	if err != nil && resp.StatusCode != 404 {
		return ctrl.Result{}, err
	}
	if resp.StatusCode == 200 {
		if !compare(want, o) {
			logger.Info("updating org")
			if _, err := gClient.EditOrg(org.Name, g.EditOrgOption{
				FullName:    org.Spec.FullName,
				Description: org.Spec.Description,
				Website:     website,
				Location:    org.Spec.Location,
				Visibility:  vis,
			}); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}
	_, _, err = gClient.CreateOrg(g.CreateOrgOption{
		Name:        org.Name,
		FullName:    org.Spec.FullName,
		Description: org.Spec.Description,
		Website:     website,
		Location:    org.Spec.Location,
		Visibility:  vis,
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	if !org.Status.Provisioned {
		org.Status.Provisioned = true
		if err := r.Client.Status().Update(ctx, org); err != nil {
			logger.Error(err, "Failed to update Org status")
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, nil
}

func (r *OrgReconciler) upsertTeam(ctx context.Context, gClient *g.Client, team *hyperv1.Team) error {

	return nil
}

func (r *OrgReconciler) buildClient(ctx context.Context, instance hyperv1.InstanceType, ns string) (*g.Client, *hyperv1.Gitea, error) {
	logger := log.FromContext(ctx)

	name := instance.Name
	namespace := instance.Namespace
	if namespace == "" {
		namespace = ns
	}
	git := &hyperv1.Gitea{}
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, git); err != nil {
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

func (r *OrgReconciler) deleteOrg(gClient *g.Client, org *hyperv1.Org) error {
	_, err := gClient.DeleteOrg(org.Name)
	return err
}

func compare(fetch, req *g.Organization) bool {
	if fetch.FullName != req.FullName {
		return false
	}
	if fetch.Description != req.Description {
		return false
	}
	if fetch.Website != req.Website {
		return false
	}
	if fetch.Location != req.Location {
		return false
	}
	if fetch.Visibility != req.Visibility {
		return false
	}

	return true
}

// SetupWithManager sets up the controller with the Manager.
func (r *OrgReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hyperv1.Org{}).
		Complete(r)
}
