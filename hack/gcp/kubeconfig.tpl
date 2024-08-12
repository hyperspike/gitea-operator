apiVersion: v1
kind: Config
current-context: ${name}
preferences: {}
clusters:
- cluster:
    certificate-authority-data: ${cluster_ca_certificate}
    server: ${endpoint}
  name: ${name}
contexts:
- context:
    cluster: ${name}
    user: ${name}
  name: ${name}
users:
- name: ${name}
  user:
    token: ${access_token}
