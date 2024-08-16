apiVersion: v1
clusters:
- cluster:
    server: ${endpoint}
    certificate-authority-data: ${ca}
  name: ${name}
contexts:
- context:
    cluster: ${name}
    user: ${name}
  name: ${name}
current-context: ${name}
kind: Config
preferences: {}
users:
- name: ${name}
  user:
    token: ${token}
