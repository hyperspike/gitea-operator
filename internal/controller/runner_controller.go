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
	"encoding/json"
	"fmt"
	"io"
	"time"

	hyperspikeClient "hyperspike.io/gitea-operator/internal/client"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	hyperv1 "hyperspike.io/gitea-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RunnerReconciler reconciles a Runner object
type RunnerReconciler struct {
	client.Client
	Recorder record.EventRecorder
	Scheme   *runtime.Scheme
}

// +kubebuilder:rbac:groups=hyperspike.io,resources=runners,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hyperspike.io,resources=runners/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=hyperspike.io,resources=runners/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Runner object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *RunnerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	runner := &hyperv1.Runner{}
	if err := r.Get(ctx, req.NamespacedName, runner); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to get Runner")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	token, url, tls, instanceName, err := r.registrationToken(ctx, runner.Spec.Org, runner.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	runner.Status.TLS = tls
	if err := r.Client.Status().Update(ctx, runner); err != nil {
		logger.Error(err, "Failed to update Runner TLS status")
		return ctrl.Result{}, nil
	}
	if token == "" {
		logger.Info("No token found, requeueing")
		return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, nil
	}
	if err := r.upsertRunnerSA(ctx, runner); err != nil {
		logger.Error(err, "failed to upsert Runner Service Account")
		return ctrl.Result{}, err
	}
	if err := r.upsertRunnerSecret(ctx, token, runner); err != nil {
		logger.Error(err, "failed to upsert Runner Secret")
		return ctrl.Result{}, err
	}
	if err := r.upsertRunnerSts(ctx, runner, url, instanceName); err != nil {
		logger.Error(err, "failed to upsert Runner")
		return ctrl.Result{}, err
	}
	if !runner.Status.Provisioned {
		runner.Status.Provisioned = true
		if err := r.Client.Status().Update(ctx, runner); err != nil {
			logger.Error(err, "Failed to update Runner status")
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, nil
}

func (r *RunnerReconciler) registrationToken(ctx context.Context, instance *hyperv1.OrgRef, ns string) (string, string, bool, string, error) {
	logger := log.FromContext(ctx)

	hclient, git, err := hyperspikeClient.BuildFromOrg(ctx, r.Client, instance, ns)
	if err != nil {
		logger.Error(err, "failed to build client")
		return "", "", false, "", err
	}
	instanceUrl := "http://" + git.Name + "." + git.Namespace + ".svc"
	if git.Spec.TLS {
		instanceUrl = "https://" + git.Name + "." + git.Namespace + ".svc"
	}
	url := instanceUrl + "/api/v1/orgs/" + instance.Name + "/actions/runners/registration-token"
	resp, err := hclient.GetJSON(url)
	if err != nil {
		logger.Error(err, "failed posting to url "+url)
		return "", "", false, "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Info("failed to close token body")
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error(err, "failed reading body from url "+url)
		return "", "", false, "", err
	}
	var tresp tokenRunner
	err = json.Unmarshal(body, &tresp)
	if err != nil {
		logger.Error(err, "failed json unmarshal "+url+" "+string(body))
		return "", "", false, "", err
	}
	if tresp.Token == "" {
		logger.Info("no token found in response from " + url + " " + string(body))
		return "", "", false, "", nil
	}
	return tresp.Token, instanceUrl, git.Spec.TLS, git.Name, nil
}

type tokenRunner struct {
	Token string `json:"token"`
}

func (r *RunnerReconciler) upsertRunnerSA(ctx context.Context, runner *hyperv1.Runner) error {
	logger := log.FromContext(ctx)
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      runner.Name,
			Namespace: runner.Namespace,
			Labels:    labels(runner.Name),
		},
	}
	if err := controllerutil.SetControllerReference(runner, sa, r.Scheme); err != nil {
		return err
	}
	err := r.Get(ctx, types.NamespacedName{Name: runner.Name, Namespace: runner.Namespace}, sa)
	if err != nil && errors.IsNotFound(err) {
		r.Recorder.Event(runner, "Normal", "Created",
			fmt.Sprintf("ServiceAccount %s is created", runner.Name))
		if err := r.Create(ctx, sa); err != nil {
			logger.Error(err, "failed to create serviceaccount")
			return err
		}
	} else {
		return err
	}
	return nil
}
func (r *RunnerReconciler) upsertRunnerSecret(ctx context.Context, token string, runner *hyperv1.Runner) error {
	logger := log.FromContext(ctx)
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      runner.Name,
			Namespace: runner.Namespace,
			Labels:    labels(runner.Name),
		},
		Data: map[string][]byte{
			"token": []byte(token),
		},
		Type: "Opaque",
	}
	fetched := secret
	if err := controllerutil.SetControllerReference(runner, &secret, r.Scheme); err != nil {
		return err
	}
	if err := controllerutil.SetControllerReference(runner, &fetched, r.Scheme); err != nil {
		return err
	}
	err := r.Get(ctx, types.NamespacedName{Name: runner.Name, Namespace: runner.Namespace}, &fetched)
	if err != nil && errors.IsNotFound(err) {
		r.Recorder.Event(runner, "Normal", "Created",
			fmt.Sprintf("Secret %s is created", runner.Name))
		if err := r.Create(ctx, &secret); err != nil {
			logger.Error(err, "failed to create secret")
			return err
		}
	}
	return nil
}

func ptr32(i int32) *int32 {
	return &i
}

func (r *RunnerReconciler) upsertRunnerSts(ctx context.Context, runner *hyperv1.Runner, instanceUrl, instanceName string) error {
	logger := log.FromContext(ctx)

	disk := resource.NewQuantity(10*1024*1024*1024, resource.BinarySI)

	replicas := ptr32(int32(runner.Spec.Replicas))
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      runner.Name,
			Namespace: runner.Namespace,
			Labels:    labels(runner.Name),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels(runner.Name),
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "data",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								"storage": *disk,
							},
						},
					},
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels(runner.Name),
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: runner.Name,
					Volumes: []corev1.Volume{
						{
							Name: "docker-certs",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{
									Medium: "",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "runner",
							Image: "gitea/act_runner:nightly",
							Command: []string{
								"sh",
								"-c",
								"while ! nc -z localhost 2376 </dev/null; do echo 'waiting for docker daemon...'; sleep 5; done; /sbin/tini -- /opt/act/run.sh",
							},
							Env: []corev1.EnvVar{
								{
									Name:  "DOCKER_HOST",
									Value: "tcp://localhost:2376",
								},
								{
									Name:  "DOCKER_CERT_PATH",
									Value: "/certs/client",
								},
								{
									Name:  "DOCKER_TLS_VERIFY",
									Value: "1",
								},
								{
									Name:  "GITEA_INSTANCE_URL",
									Value: instanceUrl,
								},
								{
									Name: "GITEA_RUNNER_REGISTRATION_TOKEN",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: runner.Name,
											},
											Key: "token",
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "docker-certs",
									MountPath: "/certs",
								},
								{
									Name:      "data",
									MountPath: "/data",
								},
							},
						},
						{
							Name:  "daemon",
							Image: "docker:23.0.6-dind",
							SecurityContext: &corev1.SecurityContext{
								Privileged: ptrBool(true),
							},
							Env: []corev1.EnvVar{
								{
									Name:  "DOCKER_TLS-CERTDIR",
									Value: "/certs",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "docker-certs",
									MountPath: "/certs",
								},
							},
						},
					},
				},
			},
		},
	}
	if runner.Status.TLS {
		sts.Spec.Template.Spec.Volumes = append(sts.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: "ca-certs",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: instanceName + "-tls",
					Items: []corev1.KeyToPath{
						{
							Key:  "ca.crt",
							Path: "ca-certificates.crt",
						},
					},
				},
			},
		})
		sts.Spec.Template.Spec.Containers[0].VolumeMounts = append(sts.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "ca-certs",
			MountPath: "/etc/ssl/certs/ca-certificates.crt",
			SubPath:   "ca-certificates.crt",
		})
	}
	if err := controllerutil.SetControllerReference(runner, sts, r.Scheme); err != nil {
		return err
	}
	err := r.Get(ctx, types.NamespacedName{Name: runner.Name, Namespace: runner.Namespace}, sts)
	if err != nil && errors.IsNotFound(err) {
		r.Recorder.Event(runner, "Normal", "Created",
			fmt.Sprintf("StatefulSet %s is updated", runner.Name))
		if err := r.Create(ctx, sts); err != nil {
			logger.Error(err, "failed to create statefulset")
			return err
		}
	} else if runner.Spec.Replicas != int(*sts.Spec.Replicas) {
		r.Recorder.Event(runner, "Normal", "Updated", fmt.Sprintf("StatefulSet %s is updated replicas %d", runner.Name, runner.Spec.Replicas))
		sts.Spec.Replicas = replicas
		if err := r.Update(ctx, sts); err != nil {
			logger.Error(err, "failed to update statefulset")
			return err
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RunnerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hyperv1.Runner{}).
		Complete(r)
}
