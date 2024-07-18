package client

import (
	"context"

	"code.gitea.io/sdk/gitea"
	"k8s.io/apimachinery/pkg/types"
	rclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	hyperv1 "hyperspike.io/gitea-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Client struct {
	*gitea.Client
	Instance *hyperv1.Gitea
}

func BuildFromOrg(ctx context.Context, r rclient.Client, instance *hyperv1.OrgRef, ns string) (*Client, *hyperv1.Gitea, error) {
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
	return Build(ctx, r, &org.Spec.Instance, "")
}

func Build(ctx context.Context, r rclient.Client, instance *hyperv1.InstanceType, ns string) (*Client, *hyperv1.Gitea, error) {
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
	g, err := gitea.NewClient(url, gitea.SetContext(ctx), gitea.SetToken(string(secret.Data["token"])))
	if err != nil {
		logger.Error(err, "failed to create client for "+url)
		return nil, nil, err
	}
	_, _, err = g.ServerVersion()
	if err != nil {
		logger.Error(err, "failed to get server version "+url)
		return nil, nil, err
	}

	c := Client{}
	c.Client = g
	c.Instance = git
	return &c, git, nil
}
