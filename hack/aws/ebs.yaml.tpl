apiVersion: v1
automountServiceAccountToken: true
kind: ServiceAccount
metadata:
  annotations:
    eks.amazonaws.com/role-arn: ${role_arn}
  labels:
    app.kubernetes.io/name: aws-ebs-csi-driver
  name: ebs-csi-controller-sa
  namespace: kube-system
---
apiVersion: v1
automountServiceAccountToken: true
kind: ServiceAccount
metadata:
  labels:
    app.kubernetes.io/name: aws-ebs-csi-driver
  name: ebs-csi-node-sa
  namespace: kube-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  labels:
    app.kubernetes.io/name: aws-ebs-csi-driver
  name: ebs-csi-leases-role
  namespace: kube-system
rules:
- apiGroups:
  - coordination.k8s.io
  resources:
  - leases
  verbs:
  - get
  - watch
  - list
  - delete
  - update
  - create
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  labels:
    app.kubernetes.io/name: aws-ebs-csi-driver
  name: ebs-csi-node-role
rules:
- apiGroups:
  - ""
  resources:
  - nodes
  verbs:
  - get
  - patch
- apiGroups:
  - storage.k8s.io
  resources:
  - volumeattachments
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - storage.k8s.io
  resources:
  - csinodes
  verbs:
  - get
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  labels:
    app.kubernetes.io/name: aws-ebs-csi-driver
  name: ebs-external-attacher-role
rules:
- apiGroups:
  - ""
  resources:
  - persistentvolumes
  verbs:
  - get
  - list
  - watch
  - update
  - patch
- apiGroups:
  - ""
  resources:
  - nodes
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - csi.storage.k8s.io
  resources:
  - csinodeinfos
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - storage.k8s.io
  resources:
  - volumeattachments
  verbs:
  - get
  - list
  - watch
  - update
  - patch
- apiGroups:
  - storage.k8s.io
  resources:
  - volumeattachments/status
  verbs:
  - patch
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  labels:
    app.kubernetes.io/name: aws-ebs-csi-driver
  name: ebs-external-provisioner-role
rules:
- apiGroups:
  - ""
  resources:
  - persistentvolumes
  verbs:
  - get
  - list
  - watch
  - create
  - patch
  - delete
- apiGroups:
  - ""
  resources:
  - persistentvolumeclaims
  verbs:
  - get
  - list
  - watch
  - update
- apiGroups:
  - storage.k8s.io
  resources:
  - storageclasses
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - events
  verbs:
  - list
  - watch
  - create
  - update
  - patch
- apiGroups:
  - snapshot.storage.k8s.io
  resources:
  - volumesnapshots
  verbs:
  - get
  - list
- apiGroups:
  - snapshot.storage.k8s.io
  resources:
  - volumesnapshotcontents
  verbs:
  - get
  - list
- apiGroups:
  - storage.k8s.io
  resources:
  - csinodes
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - nodes
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - storage.k8s.io
  resources:
  - volumeattachments
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - storage.k8s.io
  resources:
  - volumeattributesclasses
  verbs:
  - get
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  labels:
    app.kubernetes.io/name: aws-ebs-csi-driver
  name: ebs-external-resizer-role
rules:
- apiGroups:
  - ""
  resources:
  - persistentvolumes
  verbs:
  - get
  - list
  - watch
  - update
  - patch
- apiGroups:
  - ""
  resources:
  - persistentvolumeclaims
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - persistentvolumeclaims/status
  verbs:
  - update
  - patch
- apiGroups:
  - storage.k8s.io
  resources:
  - storageclasses
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - ""
  resources:
  - events
  verbs:
  - list
  - watch
  - create
  - update
  - patch
- apiGroups:
  - ""
  resources:
  - pods
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - storage.k8s.io
  resources:
  - volumeattributesclasses
  verbs:
  - get
  - list
  - watch
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  labels:
    app.kubernetes.io/name: aws-ebs-csi-driver
  name: ebs-external-snapshotter-role
rules:
- apiGroups:
  - ""
  resources:
  - events
  verbs:
  - list
  - watch
  - create
  - update
  - patch
- apiGroups:
  - snapshot.storage.k8s.io
  resources:
  - volumesnapshotclasses
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - snapshot.storage.k8s.io
  resources:
  - volumesnapshotcontents
  verbs:
  - create
  - get
  - list
  - watch
  - update
  - delete
  - patch
- apiGroups:
  - snapshot.storage.k8s.io
  resources:
  - volumesnapshotcontents/status
  verbs:
  - update
  - patch
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  labels:
    app.kubernetes.io/name: aws-ebs-csi-driver
  name: ebs-csi-leases-rolebinding
  namespace: kube-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: ebs-csi-leases-role
subjects:
- kind: ServiceAccount
  name: ebs-csi-controller-sa
  namespace: kube-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  labels:
    app.kubernetes.io/name: aws-ebs-csi-driver
  name: ebs-csi-attacher-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: ebs-external-attacher-role
subjects:
- kind: ServiceAccount
  name: ebs-csi-controller-sa
  namespace: kube-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  labels:
    app.kubernetes.io/name: aws-ebs-csi-driver
  name: ebs-csi-node-getter-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: ebs-csi-node-role
subjects:
- kind: ServiceAccount
  name: ebs-csi-node-sa
  namespace: kube-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  labels:
    app.kubernetes.io/name: aws-ebs-csi-driver
  name: ebs-csi-provisioner-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: ebs-external-provisioner-role
subjects:
- kind: ServiceAccount
  name: ebs-csi-controller-sa
  namespace: kube-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  labels:
    app.kubernetes.io/name: aws-ebs-csi-driver
  name: ebs-csi-resizer-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: ebs-external-resizer-role
subjects:
- kind: ServiceAccount
  name: ebs-csi-controller-sa
  namespace: kube-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  labels:
    app.kubernetes.io/name: aws-ebs-csi-driver
  name: ebs-csi-snapshotter-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: ebs-external-snapshotter-role
subjects:
- kind: ServiceAccount
  name: ebs-csi-controller-sa
  namespace: kube-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app.kubernetes.io/name: aws-ebs-csi-driver
  name: ebs-csi-controller
  namespace: kube-system
spec:
  replicas: 2
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app: ebs-csi-controller
      app.kubernetes.io/name: aws-ebs-csi-driver
  strategy:
    rollingUpdate:
      maxUnavailable: 1
    type: RollingUpdate
  template:
    metadata:
      labels:
        app: ebs-csi-controller
        app.kubernetes.io/name: aws-ebs-csi-driver
    spec:
      affinity:
        nodeAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - preference:
              matchExpressions:
              - key: eks.amazonaws.com/compute-type
                operator: NotIn
                values:
                - fargate
            weight: 1
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: app
                  operator: In
                  values:
                  - ebs-csi-controller
              topologyKey: kubernetes.io/hostname
            weight: 100
      containers:
      - args:
        - --endpoint=$(CSI_ENDPOINT)
        - --batching=true
        - --logging-format=text
        - --user-agent-extra=kustomize
        - --v=2
        env:
        - name: CSI_ENDPOINT
          value: unix:///var/lib/csi/sockets/pluginproxy/csi.sock
        - name: CSI_NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        - name: AWS_ACCESS_KEY_ID
          valueFrom:
            secretKeyRef:
              key: key_id
              name: aws-secret
              optional: true
        - name: AWS_SECRET_ACCESS_KEY
          valueFrom:
            secretKeyRef:
              key: access_key
              name: aws-secret
              optional: true
        - name: AWS_EC2_ENDPOINT
          valueFrom:
            configMapKeyRef:
              key: endpoint
              name: aws-meta
              optional: true
        image: public.ecr.aws/ebs-csi-driver/aws-ebs-csi-driver:v1.34.0
        imagePullPolicy: IfNotPresent
        livenessProbe:
          failureThreshold: 5
          httpGet:
            path: /healthz
            port: healthz
          initialDelaySeconds: 10
          periodSeconds: 10
          timeoutSeconds: 3
        name: ebs-plugin
        ports:
        - containerPort: 9808
          name: healthz
          protocol: TCP
        readinessProbe:
          failureThreshold: 5
          httpGet:
            path: /healthz
            port: healthz
          initialDelaySeconds: 10
          periodSeconds: 10
          timeoutSeconds: 3
        resources:
          limits:
            memory: 256Mi
          requests:
            cpu: 10m
            memory: 40Mi
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          seccompProfile:
            type: RuntimeDefault
        volumeMounts:
        - mountPath: /var/lib/csi/sockets/pluginproxy/
          name: socket-dir
      - args:
        - --timeout=60s
        - --csi-address=$(ADDRESS)
        - --v=2
        - --feature-gates=Topology=true
        - --extra-create-metadata
        - --leader-election=true
        - --default-fstype=ext4
        - --kube-api-qps=20
        - --kube-api-burst=100
        - --worker-threads=100
        - --retry-interval-max=30m
        env:
        - name: ADDRESS
          value: /var/lib/csi/sockets/pluginproxy/csi.sock
        image: public.ecr.aws/eks-distro/kubernetes-csi/external-provisioner:v5.0.1-eks-1-30-10
        imagePullPolicy: IfNotPresent
        name: csi-provisioner
        resources:
          limits:
            memory: 256Mi
          requests:
            cpu: 10m
            memory: 40Mi
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          seccompProfile:
            type: RuntimeDefault
        volumeMounts:
        - mountPath: /var/lib/csi/sockets/pluginproxy/
          name: socket-dir
      - args:
        - --timeout=60s
        - --csi-address=$(ADDRESS)
        - --v=2
        - --leader-election=true
        - --kube-api-qps=20
        - --kube-api-burst=100
        - --worker-threads=100
        - --retry-interval-max=5m
        env:
        - name: ADDRESS
          value: /var/lib/csi/sockets/pluginproxy/csi.sock
        image: public.ecr.aws/eks-distro/kubernetes-csi/external-attacher:v4.6.1-eks-1-30-10
        imagePullPolicy: IfNotPresent
        name: csi-attacher
        resources:
          limits:
            memory: 256Mi
          requests:
            cpu: 10m
            memory: 40Mi
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          seccompProfile:
            type: RuntimeDefault
        volumeMounts:
        - mountPath: /var/lib/csi/sockets/pluginproxy/
          name: socket-dir
      - args:
        - --csi-address=$(ADDRESS)
        - --leader-election=true
        - --extra-create-metadata
        - --kube-api-qps=20
        - --kube-api-burst=100
        - --worker-threads=100
        - --retry-interval-max=30m
        env:
        - name: ADDRESS
          value: /var/lib/csi/sockets/pluginproxy/csi.sock
        image: public.ecr.aws/eks-distro/kubernetes-csi/external-snapshotter/csi-snapshotter:v8.0.1-eks-1-30-10
        imagePullPolicy: IfNotPresent
        name: csi-snapshotter
        resources:
          limits:
            memory: 256Mi
          requests:
            cpu: 10m
            memory: 40Mi
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          seccompProfile:
            type: RuntimeDefault
        volumeMounts:
        - mountPath: /var/lib/csi/sockets/pluginproxy/
          name: socket-dir
      - args:
        - --timeout=60s
        - --csi-address=$(ADDRESS)
        - --v=2
        - --handle-volume-inuse-error=false
        - --leader-election=true
        - --kube-api-qps=20
        - --kube-api-burst=100
        - --workers=100
        - --retry-interval-max=30m
        env:
        - name: ADDRESS
          value: /var/lib/csi/sockets/pluginproxy/csi.sock
        image: public.ecr.aws/eks-distro/kubernetes-csi/external-resizer:v1.11.1-eks-1-30-10
        imagePullPolicy: IfNotPresent
        name: csi-resizer
        resources:
          limits:
            memory: 256Mi
          requests:
            cpu: 10m
            memory: 40Mi
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          seccompProfile:
            type: RuntimeDefault
        volumeMounts:
        - mountPath: /var/lib/csi/sockets/pluginproxy/
          name: socket-dir
      - args:
        - --csi-address=/csi/csi.sock
        image: public.ecr.aws/eks-distro/kubernetes-csi/livenessprobe:v2.13.0-eks-1-30-10
        imagePullPolicy: IfNotPresent
        name: liveness-probe
        resources:
          limits:
            memory: 256Mi
          requests:
            cpu: 10m
            memory: 40Mi
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
        volumeMounts:
        - mountPath: /csi
          name: socket-dir
      nodeSelector:
        kubernetes.io/os: linux
      priorityClassName: system-cluster-critical
      securityContext:
        fsGroup: 1000
        runAsGroup: 1000
        runAsNonRoot: true
        runAsUser: 1000
      serviceAccountName: ebs-csi-controller-sa
      tolerations:
      - key: CriticalAddonsOnly
        operator: Exists
      - effect: NoExecute
        operator: Exists
        tolerationSeconds: 300
      volumes:
      - emptyDir: {}
        name: socket-dir
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  labels:
    app.kubernetes.io/name: aws-ebs-csi-driver
  name: ebs-csi-controller
  namespace: kube-system
spec:
  maxUnavailable: 1
  selector:
    matchLabels:
      app: ebs-csi-controller
      app.kubernetes.io/name: aws-ebs-csi-driver
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  labels:
    app.kubernetes.io/name: aws-ebs-csi-driver
  name: ebs-csi-node
  namespace: kube-system
spec:
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app: ebs-csi-node
      app.kubernetes.io/name: aws-ebs-csi-driver
  template:
    metadata:
      labels:
        app: ebs-csi-node
        app.kubernetes.io/name: aws-ebs-csi-driver
    spec:
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: eks.amazonaws.com/compute-type
                operator: NotIn
                values:
                - fargate
              - key: node.kubernetes.io/instance-type
                operator: NotIn
                values:
                - a1.medium
                - a1.large
                - a1.xlarge
                - a1.2xlarge
                - a1.4xlarge
      containers:
      - args:
        - node
        - --endpoint=$(CSI_ENDPOINT)
        - --logging-format=text
        - --v=2
        env:
        - name: CSI_ENDPOINT
          value: unix:/csi/csi.sock
        - name: CSI_NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        image: public.ecr.aws/ebs-csi-driver/aws-ebs-csi-driver:v1.34.0
        imagePullPolicy: IfNotPresent
        lifecycle:
          preStop:
            exec:
              command:
              - /bin/aws-ebs-csi-driver
              - pre-stop-hook
        livenessProbe:
          failureThreshold: 5
          httpGet:
            path: /healthz
            port: healthz
          initialDelaySeconds: 10
          periodSeconds: 10
          timeoutSeconds: 3
        name: ebs-plugin
        ports:
        - containerPort: 9808
          name: healthz
          protocol: TCP
        resources:
          limits:
            memory: 256Mi
          requests:
            cpu: 10m
            memory: 40Mi
        securityContext:
          privileged: true
          readOnlyRootFilesystem: true
        volumeMounts:
        - mountPath: /var/lib/kubelet
          mountPropagation: Bidirectional
          name: kubelet-dir
        - mountPath: /csi
          name: plugin-dir
        - mountPath: /dev
          name: device-dir
      - args:
        - --csi-address=$(ADDRESS)
        - --kubelet-registration-path=$(DRIVER_REG_SOCK_PATH)
        - --v=2
        env:
        - name: ADDRESS
          value: /csi/csi.sock
        - name: DRIVER_REG_SOCK_PATH
          value: /var/lib/kubelet/plugins/ebs.csi.aws.com/csi.sock
        image: public.ecr.aws/eks-distro/kubernetes-csi/node-driver-registrar:v2.11.0-eks-1-30-10
        imagePullPolicy: IfNotPresent
        livenessProbe:
          exec:
            command:
            - /csi-node-driver-registrar
            - --kubelet-registration-path=$(DRIVER_REG_SOCK_PATH)
            - --mode=kubelet-registration-probe
          initialDelaySeconds: 30
          periodSeconds: 90
          timeoutSeconds: 15
        name: node-driver-registrar
        resources:
          limits:
            memory: 256Mi
          requests:
            cpu: 10m
            memory: 40Mi
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
        volumeMounts:
        - mountPath: /csi
          name: plugin-dir
        - mountPath: /registration
          name: registration-dir
        - mountPath: /var/lib/kubelet/plugins/ebs.csi.aws.com/
          name: probe-dir
      - args:
        - --csi-address=/csi/csi.sock
        image: public.ecr.aws/eks-distro/kubernetes-csi/livenessprobe:v2.13.0-eks-1-30-10
        imagePullPolicy: IfNotPresent
        name: liveness-probe
        resources:
          limits:
            memory: 256Mi
          requests:
            cpu: 10m
            memory: 40Mi
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
        volumeMounts:
        - mountPath: /csi
          name: plugin-dir
      hostNetwork: false
      nodeSelector:
        kubernetes.io/os: linux
      priorityClassName: system-node-critical
      securityContext:
        fsGroup: 0
        runAsGroup: 0
        runAsNonRoot: false
        runAsUser: 0
      serviceAccountName: ebs-csi-node-sa
      terminationGracePeriodSeconds: 30
      tolerations:
      - operator: Exists
      volumes:
      - hostPath:
          path: /var/lib/kubelet
          type: Directory
        name: kubelet-dir
      - hostPath:
          path: /var/lib/kubelet/plugins/ebs.csi.aws.com/
          type: DirectoryOrCreate
        name: plugin-dir
      - hostPath:
          path: /var/lib/kubelet/plugins_registry/
          type: Directory
        name: registration-dir
      - hostPath:
          path: /dev
          type: Directory
        name: device-dir
      - emptyDir: {}
        name: probe-dir
  updateStrategy:
    rollingUpdate:
      maxUnavailable: 10%
    type: RollingUpdate
---
apiVersion: storage.k8s.io/v1
kind: CSIDriver
metadata:
  labels:
    app.kubernetes.io/name: aws-ebs-csi-driver
  name: ebs.csi.aws.com
spec:
  attachRequired: true
  fsGroupPolicy: File
  podInfoOnMount: false
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
  labels:
    app.kubernetes.io/name: aws-ebs-csi-driver
  name: ebs-sc
provisioner: ebs.csi.aws.com

# vim:syntax=yaml
