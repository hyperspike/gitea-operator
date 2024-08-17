package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"time"

	"code.gitea.io/sdk/gitea"
	"k8s.io/apimachinery/pkg/types"
	rclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmetav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	hyperv1 "hyperspike.io/gitea-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Client struct {
	*gitea.Client
	Instance   *hyperv1.Gitea
	httpClient *http.Client
	CA         []byte
	token      string
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
		logger.Error(err, "failed to get gitea org "+orgName)
		return nil, nil, err
	}
	if org.Spec.Instance.Namespace == "" {
		org.Spec.Instance.Namespace = org.Namespace
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
		logger.Error(err, "failed to get gitea instance "+name+" in namespace "+namespace)
		return nil, nil, err
	}
	if !git.Status.Ready {
		return nil, nil, nil
	}
	var err error
	c := Client{}
	url := "http://" + git.Name + "." + git.Namespace + ".svc"
	if git.Spec.TLS {
		url = "https://" + git.Name + "." + git.Namespace + ".svc"
		c.CA, err = getCACertificate(ctx, r, git)
		if err != nil {
			logger.Error(err, "failed to get ca certificate")
			return nil, nil, err
		}
		c.httpClient = httpClient(ctx, c.CA)
	} else {
		c.httpClient = httpClient(ctx, nil)
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
	c.token = string(secret.Data["token"])
	g, err := gitea.NewClient(url, gitea.SetContext(ctx), gitea.SetToken(c.token), gitea.SetHTTPClient(c.httpClient))
	if err != nil {
		logger.Error(err, "failed to create client for "+url)
		return nil, nil, err
	}
	_, _, err = g.ServerVersion()
	if err != nil {
		logger.Error(err, "failed to get server version "+url)
		return nil, nil, err
	}

	c.Client = g
	c.Instance = git
	return &c, git, nil
}

func httpClient(ctx context.Context, CA []byte) *http.Client {
	logger := log.FromContext(ctx)
	httpClient := http.Client{
		Timeout: time.Second * 10,
	}
	if CA != nil {
		certpool, _ := x509.SystemCertPool()
		if certpool == nil {
			logger.Info("system cert pool is nil, creating new")
			certpool = x509.NewCertPool()
		}
		certpool.AppendCertsFromPEM(CA)
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    certpool,
				MinVersion: tls.VersionTLS12,
			},
		}
	}
	return &httpClient
}

func getCACertificate(ctx context.Context, r rclient.Client, gitea *hyperv1.Gitea) ([]byte, error) {
	logger := log.FromContext(ctx)

	cert := &certv1.Certificate{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: gitea.Namespace, Name: gitea.Name}, cert); err != nil {
		logger.Error(err, "failed to get ca certificate")
		return []byte{}, err
	}
	if cert.Status.Conditions == nil {
		return []byte{}, nil
	}
	good := false
	for _, cond := range cert.Status.Conditions {
		if cond.Type == certv1.CertificateConditionReady {
			if cond.Status == cmetav1.ConditionTrue {
				good = true
				break
			}
		}
	}
	if !good {
		return []byte{}, nil
	}
	tls := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Namespace: gitea.Namespace, Name: cert.Spec.SecretName}, tls)
	if err != nil {
		logger.Error(err, "failed to get tls secret")
		return []byte{}, err
	}
	return tls.Data["ca.crt"], nil
}

func (c *Client) Get(url string) (resp *http.Response, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "gitea-operator/0.1.0")
	req.Header.Set("Authorization", c.token)

	return c.httpClient.Do(req)

}
func (c *Client) GetJSON(url string) (resp *http.Response, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "gitea-operator/0.1.0")
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/json")

	return c.httpClient.Do(req)

}

func (c *Client) Post(url, contentType string, body io.Reader) (resp *http.Response, err error) {
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err

	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "gitea-operator/0.1.0")
	req.Header.Set("Authorization", "token "+c.token)

	return c.httpClient.Do(req)

}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", "gitea-operator/0.1.0")
	req.Header.Set("Authorization", "token "+c.token)
	return c.httpClient.Do(req)

}
