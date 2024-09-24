---
apiVersion: v1
kind: ServiceAccount
metadata:
  annotations:
    eks.amazonaws.com/role-arn: ${role_arn}
  name: external-dns
  namespace: kube-system
  labels:
    app.kubernetes.io/name: external-dns
    app.kubernetes.io/instance: external-dns
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: external-dns
  labels:
    app.kubernetes.io/name: external-dns
    app.kubernetes.io/instance: external-dns
rules:
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["list","watch"]
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get","watch","list"]
  - apiGroups: [""]
    resources: ["services","endpoints"]
    verbs: ["get","watch","list"]
  - apiGroups: ["extensions","networking.k8s.io"]
    resources: ["ingresses"]
    verbs: ["get","watch","list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: external-dns-viewer
  labels:
    app.kubernetes.io/name: external-dns
    app.kubernetes.io/instance: external-dns
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: external-dns
subjects:
  - kind: ServiceAccount
    name: external-dns
    namespace: kube-system
---
# Source: external-dns/templates/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: external-dns
  namespace: kube-system
  labels:
    app.kubernetes.io/name: external-dns
    app.kubernetes.io/instance: external-dns
spec:
  type: ClusterIP
  selector:
    app.kubernetes.io/name: external-dns
    app.kubernetes.io/instance: external-dns
  ports:
    - name: http
      port: 7979
      targetPort: http
      protocol: TCP
---
# Source: external-dns/templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: external-dns
  namespace: kube-system
  labels:
    app.kubernetes.io/name: external-dns
    app.kubernetes.io/instance: external-dns
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: external-dns
      app.kubernetes.io/instance: external-dns
  strategy:
    type: Recreate
  template:
    metadata:
      labels:
        app.kubernetes.io/name: external-dns
        app.kubernetes.io/instance: external-dns
    spec:
      serviceAccountName: external-dns
      securityContext:
        fsGroup: 65534
        runAsNonRoot: true
        seccompProfile:
          type: RuntimeDefault
      containers:
        - name: external-dns
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              drop:
              - ALL
            privileged: false
            readOnlyRootFilesystem: true
            runAsGroup: 65532
            runAsNonRoot: true
            runAsUser: 65532
          image: registry.k8s.io/external-dns/external-dns:v0.15.0
          imagePullPolicy: IfNotPresent
          args:
            - --log-level=info
            - --log-format=text
            - --interval=1m
            - --source=service
            - --source=ingress
            - --policy=upsert-only
            - --registry=txt
            - --provider=aws
            - --domain-filter=${domain}
            - --aws-zone-type=public
          ports:
            - name: http
              protocol: TCP
              containerPort: 7979
          livenessProbe:
            failureThreshold: 2
            httpGet:
              path: /healthz
              port: http
            initialDelaySeconds: 10
            periodSeconds: 10
            successThreshold: 1
            timeoutSeconds: 5
          readinessProbe:
            failureThreshold: 6
            httpGet:
              path: /healthz
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
            successThreshold: 1
            timeoutSeconds: 5

# vim:syntax=yaml
