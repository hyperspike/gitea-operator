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
	hclient "hyperspike.io/gitea-operator/internal/client"
	"k8s.io/apimachinery/pkg/api/errors"
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
	h      *hclient.Client
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
	h, gitea, err := hclient.Build(ctx, r.Client, &user.Spec.Instance, user.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	if h == nil && gitea == nil {
		return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, nil
	}
	r.h = h
	isUserMarkedToBeDeleted := user.GetDeletionTimestamp() != nil
	if isUserMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(user, userFinalizer) {
			if err := r.deleteUser(user); err != nil {
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

	if err := r.upsertUser(ctx, user); err != nil {
		return ctrl.Result{}, err
	}
	if !controllerutil.ContainsFinalizer(user, userFinalizer) {
		controllerutil.AddFinalizer(user, userFinalizer)
		if err := r.Update(ctx, user); err != nil {
			logger.Error(err, "failed to add finalizers")
			return ctrl.Result{}, err
		}
	}
	users, err := r.listUsers(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	gitUsers, err := r.listGitUsers(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	for _, u := range gitUsers {
		found := false
		for _, usr := range users {
			if usr.Name == u.UserName {
				found = true
				break
			}
		}
		if !found {
			if err := r.createUser(ctx, user, u); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *UserReconciler) password(ctx context.Context, user *hyperv1.User) (string, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: user.Spec.Password.Name, Namespace: user.Namespace}, secret)
	if err != nil {
		if errors.IsNotFound(err) {
			secret.Name = user.Spec.Password.Name
			secret.Namespace = user.Namespace
			pw, err := randString(16)
			if err != nil {
				return "", err
			}
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
	if u1.IsAdmin != u2.IsAdmin {
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

func (r *UserReconciler) upsertUser(ctx context.Context, user *hyperv1.User) error {
	logger := log.FromContext(ctx)
	fetched, res, err := r.h.GetUserInfo(user.Name)
	if err != nil && res.StatusCode != 404 {
		return err
	}
	loginName := user.Spec.LoginName
	if loginName == "" {
		loginName = user.Name
	}
	want := &g.User{
		Email:    user.Spec.Email,
		FullName: user.Spec.FullName,
		IsAdmin:  user.Spec.Admin,
	}
	if fetched != nil && res.StatusCode == 200 {
		if err := r.reconcileSSHKeys(user); err != nil {
			return err
		}
		if !compareUsers(want, fetched) {
			logger.Info("updating user", "user", user.Name, "instance", user.Spec.Instance.Name)
			if _, err := r.h.AdminEditUser(user.Name, g.EditUserOption{
				Email:     &user.Spec.Email,
				FullName:  &user.Spec.FullName,
				LoginName: loginName,
				Admin:     ptrBool(user.Spec.Admin),
			}); err != nil {
				logger.Error(err, "failed to update user", "user", user.Name)
				return err
			}
		}
		return nil
	}
	pw, err := r.password(ctx, user)
	if err != nil {
		return err
	}
	_, _, err = r.h.AdminCreateUser(g.CreateUserOption{
		FullName:           user.Spec.FullName,
		Email:              user.Spec.Email,
		LoginName:          loginName,
		Username:           user.Name,
		SendNotify:         user.Spec.SendNotify,
		SourceID:           user.Spec.SourceId,
		MustChangePassword: ptrBool(false),
		Password:           pw,
	})
	if err != nil {
		logger.Error(err, "failed to create user", "user", user.Name)
		return err
	}

	if err := r.reconcileSSHKeys(user); err != nil {
		logger.Error(err, "failed to add ssh keys", "user", user.Name)
		return err
	}
	if err := r.userReady(ctx, user); err != nil {
		return err
	}
	return err
}

func (r *UserReconciler) createUser(ctx context.Context, usr *hyperv1.User, gitUser *g.User) error {
	logger := log.FromContext(ctx)
	user := &hyperv1.User{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      gitUser.UserName,
			Namespace: usr.Namespace,
		},
		Spec: hyperv1.UserSpec{
			Email:    gitUser.Email,
			FullName: gitUser.FullName,
			SourceId: gitUser.SourceID,
			Admin:    gitUser.IsAdmin,
			Instance: usr.Spec.Instance,
		},
	}
	if err := r.Create(ctx, user); err != nil {
		logger.Error(err, "failed to create user", "user", gitUser.UserName)
		return err
	}
	return nil
}

func (r *UserReconciler) listGitUsers(ctx context.Context) ([]*g.User, error) {
	users, _, err := r.h.AdminListUsers(g.AdminListUsersOptions{})
	if err != nil {
		return nil, err
	}
	return users, nil
}

func (r *UserReconciler) listUsers(ctx context.Context) ([]hyperv1.User, error) {
	users := &hyperv1.UserList{}
	if err := r.List(ctx, users); err != nil {
		return nil, err
	}
	return users.Items, nil
}

func (r *UserReconciler) reconcileSSHKeys(user *hyperv1.User) error {
	keys, _, err := r.h.ListPublicKeys(user.Name, g.ListPublicKeysOptions{})
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
		_, _ = r.h.AdminDeleteUserPublicKey(user.Name, int(k))
	}
	for _, k := range addKeys {
		// @TODO handle errors
		_, _, _ = r.h.AdminCreateUserPublicKey(user.Name, g.CreateKeyOption{ReadOnly: false, Key: k, Title: user.Name + "-key"})
	}
	return nil
}

func (r *UserReconciler) deleteUser(user *hyperv1.User) error {
	_, err := r.h.AdminDeleteUser(user.Name)
	return err
}

// SetupWithManager sets up the controller with the Manager.
func (r *UserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hyperv1.User{}).
		Complete(r)
}
