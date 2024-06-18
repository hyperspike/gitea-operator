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
	"k8s.io/apimachinery/pkg/api/errors"
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

// UserReconciler reconciles a User object
type UserReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const userFinalizer = "user.hyperspike.io/finalizer"

// +kubebuilder:rbac:groups=hyperspike.io,resources=users,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hyperspike.io,resources=users/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=hyperspike.io,resources=users/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the User object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *UserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	user := &hyperv1.User{}
	if err := r.Get(ctx, req.NamespacedName, user); err != nil {
		logger.Error(err, "failed to get user")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !controllerutil.ContainsFinalizer(user, userFinalizer) {
		controllerutil.AddFinalizer(user, userFinalizer)
		if err := r.Update(ctx, user); err != nil {
			logger.Error(err, "failed to add finalizers")
			return ctrl.Result{}, err
		}
	}
	gClient, gitea, err := r.buildClient(ctx, user.Spec.Instance, user.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	if gClient == nil && gitea == nil {
		return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, nil
	}
	isRepoMarkedToBeDeleted := user.GetDeletionTimestamp() != nil
	if isRepoMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(user, userFinalizer) {
			if err := r.deleteUser(gClient, user); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(user, userFinalizer)
			err := r.Update(ctx, user)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if err := r.upsertUser(ctx, gClient, user); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *UserReconciler) buildClient(ctx context.Context, instance hyperv1.InstanceType, ns string) (*g.Client, *hyperv1.Gitea, error) {
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

func (r *UserReconciler) password(ctx context.Context, user *hyperv1.User) (string, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: user.Spec.Password.Name, Namespace: user.Namespace}, secret)
	if err != nil {
		if errors.IsNotFound(err) {
			secret.Name = user.Spec.Password.Name
			secret.Namespace = user.Namespace
			pw := randString(16)
			secret.Data[user.Spec.Password.Key] = []byte(pw)
			if err := r.Create(ctx, secret); err != nil {
				return "", err
			}
			return pw, nil
		}
		return "", err
	}
	return string(secret.Data[user.Spec.Password.Key]), nil
}

func compareUsers(u1, u2 *g.User) bool {
	if u1.Email != u2.Email {
		return false
	}
	if u1.FullName != u2.FullName {
		return false
	}
	return true
}

func (r *UserReconciler) userReady(ctx context.Context, user *hyperv1.User) error {
	if user.Status.Provisioned {
		return nil
	}
	user.Status.Provisioned = true
	if err := r.Client.Status().Update(ctx, user); err != nil {
		return err
	}
	return nil
}

func (r *UserReconciler) upsertUser(ctx context.Context, gClient *g.Client, user *hyperv1.User) error {
	fetched, res, err := gClient.GetUserInfo(user.Name)
	if err != nil && res.StatusCode != 404 {
		return err
	}
	want := &g.User{
		Email:    user.Spec.Email,
		FullName: user.Spec.FullName,
	}
	if res.StatusCode == 200 {
		if err := r.reconcileSSHKeys(gClient, user); err != nil {
			return err
		}
		if !compareUsers(want, fetched) {
			if _, err := gClient.AdminEditUser(user.Name, g.EditUserOption{
				Email:    &user.Spec.Email,
				FullName: &user.Spec.FullName,
			}); err != nil {
				return err
			}
		}
		return nil
	}
	pw, err := r.password(ctx, user)
	if err != nil {
		return err
	}
	_, _, err = gClient.AdminCreateUser(g.CreateUserOption{
		FullName:           user.Spec.FullName,
		Email:              user.Spec.Email,
		LoginName:          user.Spec.LoginName,
		Username:           user.Name,
		SendNotify:         user.Spec.SendNotify,
		SourceID:           user.Spec.SourceId,
		MustChangePassword: ptrBool(false),
		Password:           pw,
	})
	if err := r.reconcileSSHKeys(gClient, user); err != nil {
		return err
	}
	if err := r.userReady(ctx, user); err != nil {
		return err
	}
	return err
}

func (r *UserReconciler) reconcileSSHKeys(gClient *g.Client, user *hyperv1.User) error {
	keys, _, err := gClient.ListPublicKeys(user.Name, g.ListPublicKeysOptions{})
	if err != nil {
		return err
	}

	var deleteKeys []int64
	for _, key := range keys {
		matched := false
		for _, uKey := range user.Spec.SSHkeys {
			if key.Key == uKey {
				matched = true
			}
		}
		if !matched {
			deleteKeys = append(deleteKeys, key.ID)
		}
	}
	var addKeys []string
	for _, uKey := range user.Spec.SSHkeys {
		missing := true
		for _, key := range keys {
			if uKey == key.Key {
				missing = false
			}
		}
		if missing {
			addKeys = append(addKeys, uKey)
		}
	}
	for _, k := range deleteKeys {
		_, _ = gClient.AdminDeleteUserPublicKey(user.Name, int(k))
	}
	for _, k := range addKeys {
		// @TODO handle errors
		_, _, _ = gClient.AdminCreateUserPublicKey(user.Name, g.CreateKeyOption{ReadOnly: false, Key: k, Title: user.Name + "-key"})
	}
	return nil
}

func (r *UserReconciler) deleteUser(gClient *g.Client, user *hyperv1.User) error {
	_, err := gClient.AdminDeleteUser(user.Name)
	return err
}

// SetupWithManager sets up the controller with the Manager.
func (r *UserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hyperv1.User{}).
		Complete(r)
}
