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
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	zalandov1 "github.com/zalando/postgres-operator/pkg/apis/acid.zalan.do/v1"
	hyperv1 "hyperspike.io/gitea-operator/api/v1"
	valkeyv1 "hyperspike.io/valkey-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	buf := make([]byte, 1)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		panic(fmt.Sprintf("crypto/rand is unavailable: Read() failed %#v", err))
	}
}

func randString(n int) (string, error) {
	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-"
	ret := make([]byte, n)
	for i := 0; i < n; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", err
		}
		ret[i] = letters[num.Int64()]
	}

	return string(ret), nil
}

// GiteaReconciler reconciles a Gitea object
type GiteaReconciler struct {
	client.Client
	Recorder record.EventRecorder
	Scheme   *runtime.Scheme
}

// +kubebuilder:rbac:groups=hyperspike.io,resources=gitea,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hyperspike.io,resources=valkeys,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hyperspike.io,resources=gitea/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=hyperspike.io,resources=gitea/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=serviceaccounts;secrets;services,verbs=create;delete;get;list;watch;update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;patch;update;watch;delete
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=create;delete;deletecollection;get;list;patch;update;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=create;delete;get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=create;delete;get;list;watch
// +kubebuilder:rbac:groups=acid.zalan.do,resources=postgresqls,verbs=create;delete;get;list;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=create;delete;get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Gitea object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *GiteaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var gitea hyperv1.Gitea
	if err := r.Get(ctx, req.NamespacedName, &gitea); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to get Gitea")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	res, err := r.reconcileGitea(ctx, &gitea)
	if err != nil {
		return res, err
	}

	return ctrl.Result{}, nil
}

//nolint:unparam
func (r *GiteaReconciler) setCondition(ctx context.Context, gitea *hyperv1.Gitea,
	typeName string, status metav1.ConditionStatus, reason string, message string) error {
	logger := log.FromContext(ctx)

	condition := metav1.Condition{Type: typeName, Status: status, Reason: reason, Message: message}
	meta.SetStatusCondition(&gitea.Status.Conditions, condition)

	if err := r.Client.Status().Update(ctx, gitea); err != nil {
		logger.Info("Gitea status update failed.")
	}
	return nil
}

//nolint:unparam
func (r *GiteaReconciler) reconcileGitea(ctx context.Context, gitea *hyperv1.Gitea) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	if err := r.upsertPG(ctx, gitea); err != nil {
		return ctrl.Result{}, err
	}
	if !gitea.Status.Ready {
		if err := r.setCondition(ctx, gitea, "DatabaseReady", "False", "DatabaseReady", "database still provisioning"); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.setCondition(ctx, gitea, "CacheReady", "False", "CacheReady", "valkey still provisioning"); err != nil {
			return ctrl.Result{}, err
		}
	}
	pgUp, _ := r.pgRunning(ctx, gitea)
	if !pgUp {
		return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, nil
	}
	if err := r.upsertValkey(ctx, gitea); err != nil {
		return ctrl.Result{}, err
	}
	vkUp, _ := r.valkeyRunning(ctx, gitea)
	if !vkUp {
		return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, nil
	}
	if !gitea.Status.Ready {
		if err := r.setCondition(ctx, gitea, "DatabaseReady", "True", "DatabaseReady", "database ready"); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.setCondition(ctx, gitea, "CacheReady", "True", "CacheReady", "valkey ready"); err != nil {
			return ctrl.Result{}, err
		}
	}
	r.Recorder.Event(gitea, "Normal", "Running",
		fmt.Sprintf("Postgres %s is running",
			gitea.Name+"-"+gitea.Name))
	r.Recorder.Event(gitea, "Normal", "Running",
		fmt.Sprintf("Valkey %s is running",
			gitea.Name+"-valkey"))

	if err := r.upsertGiteaSvc(ctx, gitea); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.upsertGiteaSa(ctx, gitea); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.upsertSecret(ctx, gitea, gitea.Name+"-init", map[string]string{
		"configure_gpg_environment.sh": `#!/usr/bin/env bash
set -eu

gpg --batch --import /raw/private.asc`,
		"init_directory_structure.sh": `#!/usr/bin/env bash

set -euo pipefail

set -x

mkdir -p /data/git/.ssh
chmod -R 700 /data/git/.ssh
mkdir -p /data/ssh
chmod -R 700 /data/
chown 1000 -R /data 
[ ! -d /data/gitea/conf ] && mkdir -p /data/gitea/conf

# prepare temp directory structure
mkdir -p "${GITEA_TEMP}"
chown 1000:1000 "${GITEA_TEMP}"
chmod ug+rwx "${GITEA_TEMP}"`,
		"configure_gitea.sh": `#!/usr/bin/env bash

set -euo pipefail

echo '==== BEGIN GITEA CONFIGURATION ===='

{ # try
	gitea migrate
} || { # catch
	echo "Gitea migrate might fail due to database connection...This init-container will try again in a few seconds"
	exit 1
}

function test_redis_connection() {
	local RETRY=0
	local MAX=30

	echo 'Wait for redis to become available...'
	until [ "${RETRY}" -ge "${MAX}" ]; do
		nc -vz -w2 gitea-redis-cluster-headless.default.svc.cluster.local 6379 && break
		RETRY=$[${RETRY}+1]
		echo "...not ready yet (${RETRY}/${MAX})"
	done

	if [ "${RETRY}" -ge "${MAX}" ]; then
		echo "Redis not reachable after '${MAX}' attempts!"
		exit 1
	fi
}

# test_redis_connection

function configure_admin_user() {
	local full_admin_list=$(gitea admin user list --admin)
	local actual_user_table=''

	# We might have distorted output due to warning logs, so we have to detect the actual user table by its headline and trim output above that line
	local regex="(.*)(ID\s+Username\s+Email\s+IsActive.*)"
	if [[ "${full_admin_list}" =~ $regex ]]; then
		actual_user_table=$(echo "${BASH_REMATCH[2]}" | tail -n+2) # tail'ing to drop the table headline
	else
		# This code block should never be reached, as long as the output table header remains the same.
		# If this code block is reached, the regex doesn't match anymore and we probably have to adjust this script.

		echo "ERROR: 'configure_admin_user' was not able to determine the current list of admin users."
		echo "       Please review the output of 'gitea admin user list --admin' shown below."
		echo "       If you think it is an issue with the Helm Chart provisioning, file an issue at https://gitea.com/gitea/helm-chart/issues."
		echo "DEBUG: Output of 'gitea admin user list --admin'"
		echo "--"
		echo "${full_admin_list}"
		echo "--"
		exit 1
	fi

	local ACCOUNT_ID=$(echo "${actual_user_table}" | grep -E "\s+${GITEA_ADMIN_USERNAME}\s+" | awk -F " " "{printf \$1}")
	if [[ -z "${ACCOUNT_ID}" ]]; then
		echo "No admin user '${GITEA_ADMIN_USERNAME}' found. Creating now..."
		gitea admin user create --admin --username "${GITEA_ADMIN_USERNAME}" --password "${GITEA_ADMIN_PASSWORD}" --email "gitea@local.domain" --must-change-password=false
		echo '...created.'
	else
		echo "Admin account '${GITEA_ADMIN_USERNAME}' already exist. Running update to sync password..."
		gitea admin user change-password --username "${GITEA_ADMIN_USERNAME}" --password "${GITEA_ADMIN_PASSWORD}"
		echo '...password sync done.'
	fi
}

configure_admin_user

function configure_ldap() {
        echo 'no ldap configuration... skipping.'
}

configure_ldap

function configure_oauth() {
        echo 'no oauth configuration... skipping.'
}

configure_oauth

echo '==== END GITEA CONFIGURATION ===='`,
	}, false); err != nil {
		return ctrl.Result{}, err
	}
	hostname := gitea.Spec.Ingress.Host
	if hostname == "" {
		hostname = "git.example.com"
	}
	password, err := r.getValkeyPassword(ctx, gitea)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.upsertSecret(ctx, gitea, gitea.Name+"-inline-config", map[string]string{
		"_generals_": "",
		"cache":      cacheSvc(gitea, password),
		/* "database": `DB_TYPE=postgres
		HOST=gitea-postgresql-ha-pgpool.default.svc.cluster.local:5432
		NAME=gitea
		PASSWD=gitea
		USER=gitea`, */
		"indexer":    "ISSUE_INDEXER_TYPE=db",
		"metrics":    "ENABLED=false",
		"queue":      queueSvc(gitea, password),
		"repository": "ROOT=/data/git/gitea-repositories",
		"security":   "INSTALL_LOCK=true",
		"server": `APP_DATA_PATH=/data
DOMAIN=` + hostname + `
ENABLE_PPROF=false
HTTP_PORT=3000
PROTOCOL=http
ROOT_URL=https://` + hostname + `
SSH_DOMAIN=` + hostname + `
SSH_LISTEN_PORT=2222
SSH_PORT=22
START_SSH_SERVER=true`,
		"session": sessionSvc(gitea, password),
	}, false); err != nil {
		return ctrl.Result{}, err
	}

	rs, err := randString(14)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.upsertSecret(ctx, gitea, gitea.Name+"-admin", map[string]string{"username": "gitea", "password": rs}, true); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.upsertSecret(ctx, gitea, gitea.Name+"-config", map[string]string{
		"assertions": "",
		"config_environment.sh": `#!/usr/bin/env bash
    set -euo pipefail

    function env2ini::log() {
      printf "${1}\n"
    }

    function env2ini::read_config_to_env() {
      local section="${1}"
      local line="${2}"

      if [[ -z "${line}" ]]; then
        # skip empty line
        return
      fi
      
      # 'xargs echo -n' trims all leading/trailing whitespaces and a trailing new line
      local setting="$(awk -F '=' '{print $1}' <<< "${line}" | xargs echo -n)"

      if [[ -z "${setting}" ]]; then
        env2ini::log '  ! invalid setting'
        exit 1
      fi

      local value=''
      local regex="^${setting}(\s*)=(\s*)(.*)"
      if [[ $line =~ $regex ]]; then
        value="${BASH_REMATCH[3]}"
      else
        env2ini::log '  ! invalid setting'
        exit 1
      fi

      env2ini::log "    + '${setting}'"

      if [[ -z "${section}" ]]; then
        export "GITEA____${setting^^}=${value}"                           # '^^' makes the variable content uppercase
        return
      fi

      local masked_section="${section//./_0X2E_}"                            # '//' instructs to replace all matches
      masked_section="${masked_section//-/_0X2D_}"

      export "GITEA__${masked_section^^}__${setting^^}=${value}"        # '^^' makes the variable content uppercase
    }

    function env2ini::reload_preset_envs() {
      env2ini::log "Reloading preset envs..."

      while read -r line; do
        if [[ -z "${line}" ]]; then
          # skip empty line
          return
        fi

        # 'xargs echo -n' trims all leading/trailing whitespaces and a trailing new line
        local setting="$(awk -F '=' '{print $1}' <<< "${line}" | xargs echo -n)"

        if [[ -z "${setting}" ]]; then
          env2ini::log '  ! invalid setting'
          exit 1
        fi

        local value=''
        local regex="^${setting}(\s*)=(\s*)(.*)"
        if [[ $line =~ $regex ]]; then
          value="${BASH_REMATCH[3]}"
        else
          env2ini::log '  ! invalid setting'
          exit 1
        fi

        env2ini::log "  + '${setting}'"

        export "${setting^^}=${value}"                           # '^^' makes the variable content uppercase
      done < "/tmp/existing-envs"

      rm /tmp/existing-envs
    }


    function env2ini::process_config_file() {
      local config_file="${1}"
      local section="$(basename "${config_file}")"

      if [[ $section == '_generals_' ]]; then
        env2ini::log "  [ini root]"
        section=''
      else
        env2ini::log "  ${section}"
      fi

      while read -r line; do
        env2ini::read_config_to_env "${section}" "${line}"
      done < <(awk 1 "${config_file}")                             # Helm .toYaml trims the trailing new line which breaks line processing; awk 1 ... adds it back while reading
    }

    function env2ini::load_config_sources() {
      local path="${1}"

      if [[ -d "${path}" ]]; then
        env2ini::log "Processing $(basename "${path}")..."

        while read -d '' configFile; do
          env2ini::process_config_file "${configFile}"
        done < <(find "${path}" -type l -not -name '..data' -print0)

        env2ini::log "\n"
      fi
    }

    function env2ini::generate_initial_secrets() {
      # These environment variables will either be
      #   - overwritten with user defined values,
      #   - initially used to set up Gitea
      # Anyway, they won't harm existing app.ini files

      export GITEA__SECURITY__INTERNAL_TOKEN=$(gitea generate secret INTERNAL_TOKEN)
      export GITEA__SECURITY__SECRET_KEY=$(gitea generate secret SECRET_KEY)
      export GITEA__OAUTH2__JWT_SECRET=$(gitea generate secret JWT_SECRET)
      export GITEA__SERVER__LFS_JWT_SECRET=$(gitea generate secret LFS_JWT_SECRET)

      env2ini::log "...Initial secrets generated\n"
    }
    
    # save existing envs prior to script execution. Necessary to keep order of preexisting and custom envs
    env | (grep -e '^GITEA__' || [[ $? == 1 ]]) > /tmp/existing-envs
    
    # MUST BE CALLED BEFORE OTHER CONFIGURATION
    env2ini::generate_initial_secrets

    env2ini::load_config_sources '/env-to-ini-mounts/inlines/'
    env2ini::load_config_sources '/env-to-ini-mounts/additionals/'

    # load existing envs to override auto generated envs
    env2ini::reload_preset_envs

    env2ini::log "=== All configuration sources loaded ===\n"

    # safety to prevent rewrite of secret keys if an app.ini already exists
    if [ -f ${GITEA_APP_INI} ]; then
      env2ini::log 'An app.ini file already exists. To prevent overwriting secret keys, these settings are dropped and remain unchanged:'
      env2ini::log '  - security.INTERNAL_TOKEN'
      env2ini::log '  - security.SECRET_KEY'
      env2ini::log '  - oauth2.JWT_SECRET'
      env2ini::log '  - server.LFS_JWT_SECRET'

      unset GITEA__SECURITY__INTERNAL_TOKEN
      unset GITEA__SECURITY__SECRET_KEY
      unset GITEA__OAUTH2__JWT_SECRET
      unset GITEA__SERVER__LFS_JWT_SECRET
    fi

    environment-to-ini -o $GITEA_APP_INI`,
	}, false); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.upsertGiteaSts(ctx, gitea); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.upsertGiteaIngress(ctx, gitea); err != nil {
		return ctrl.Result{}, err
	}

	up, _ := r.podUP(ctx, gitea)
	if !up {
		return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, nil
	}
	logger.Info("pod up", "sts", gitea.Name)
	if !r.apiUP(ctx, gitea) {
		return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, nil
	}
	logger.Info("api is up", "sts", gitea.Name)
	if err := r.adminToken(ctx, gitea); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func labels(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      "gitea",
		"app.kubernetes.io/instance":  name,
		"app.kubernetes.io/component": "deployment",
		"app.kubernetes.io/part-of":   "gitea",
	}
}

// upsertPG - Create or update a postgres cluster {{{
func (r *GiteaReconciler) upsertPG(ctx context.Context, gitea *hyperv1.Gitea) error {
	logger := log.FromContext(ctx)
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   gitea.Name,
			Labels: labels(gitea.Name),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "postgres-pod",
				Namespace: gitea.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "postgres-pod",
		},
	}
	// dont set owner here
	err := r.Get(ctx, types.NamespacedName{Name: gitea.Name, Namespace: gitea.Namespace}, crb)
	if err != nil && errors.IsNotFound(err) {
		if err := r.Create(ctx, crb); err != nil {
			logger.Error(err, "failed to create postgres crb")
			return err
		}
	} else {
		return err
	}
	l := labels(gitea.Name)
	l["app.kubernetes.io/component"] = "database"
	pg := &zalandov1.Postgresql{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gitea.Name + "-" + gitea.Name,
			Namespace: gitea.Namespace,
			Labels:    l,
		},
		Spec: zalandov1.PostgresSpec{
			TeamID: gitea.Name,
			Volume: zalandov1.Volume{
				Size: "15Gi",
			},
			NumberOfInstances: int32(2),
			Resources: &zalandov1.Resources{
				ResourceRequests: zalandov1.ResourceDescription{
					CPU:    ptrString("10m"),
					Memory: ptrString("128Mi"),
				},
				ResourceLimits: zalandov1.ResourceDescription{
					CPU:    ptrString("1500m"),
					Memory: ptrString("1280Mi"),
				},
			},
			Users: map[string]zalandov1.UserFlags{
				gitea.Name: {
					"superuser",
					"createdb",
				},
			},
			Databases: map[string]string{
				gitea.Name: gitea.Name,
			},
			PostgresqlParam: zalandov1.PostgresqlParam{
				PgVersion: "16",
				Parameters: map[string]string{
					"shared_preload_libraries": "bg_mon,pg_stat_statements,pgextwlist,pg_auth_mon,set_user,timescaledb,pg_cron,pg_stat_kcache,pgaudit",
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(gitea, pg, r.Scheme); err != nil {
		return err
	}
	err = r.Get(ctx, types.NamespacedName{Name: gitea.Name + "-" + gitea.Name, Namespace: gitea.Namespace}, pg)
	if err != nil && errors.IsNotFound(err) {
		r.Recorder.Event(gitea, "Normal", "Creating",
			fmt.Sprintf("Postgres %s is being created", gitea.Name+"-"+gitea.Name))
		if err := r.Create(ctx, pg); err != nil {
			logger.Error(err, "failed to create postgres")
			return err
		}
	} else {
		return err
	}
	return nil
}

// }}}

// pgRunning - check the postgres CR for state {{{
func (r *GiteaReconciler) pgRunning(ctx context.Context, gitea *hyperv1.Gitea) (bool, error) {
	l := labels(gitea.Name)
	l["app.kubernetes.io/component"] = "database"
	pg := &zalandov1.Postgresql{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gitea.Name + "-" + gitea.Name,
			Namespace: gitea.Namespace,
			Labels:    l,
		},
	}
	if err := r.Get(ctx, types.NamespacedName{Name: gitea.Name + "-" + gitea.Name, Namespace: gitea.Namespace}, pg); err != nil {
		return false, err
	}
	if pg.Status.PostgresClusterStatus == "Running" {
		return true, nil
	}
	return false, nil
}

// }}}

// upsertValkey - Create or update a valkey cluster {{{
func (r *GiteaReconciler) upsertValkey(ctx context.Context, gitea *hyperv1.Gitea) error {
	logger := log.FromContext(ctx)
	l := labels(gitea.Name)
	l["app.kubernetes.io/component"] = "cache"
	vk := &valkeyv1.Valkey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gitea.Name + "-valkey",
			Namespace: gitea.Namespace,
			Labels:    l,
		},
		Spec: valkeyv1.ValkeySpec{
			Nodes:             3,
			VolumePermissions: true,
		},
	}
	if err := controllerutil.SetControllerReference(gitea, vk, r.Scheme); err != nil {
		return err
	}
	err := r.Get(ctx, types.NamespacedName{Name: gitea.Name + "-valkey", Namespace: gitea.Namespace}, vk)
	if err != nil && errors.IsNotFound(err) {
		r.Recorder.Event(gitea, "Normal", "Creating",
			fmt.Sprintf("Valkey %s is being created", gitea.Name+"-valkey"))
		if err := r.Create(ctx, vk); err != nil {
			logger.Error(err, "failed to create valkey")
			return err
		}
	} else {
		return err
	}
	return nil
}

// }}}

// valkeyRunning - check the valkey CR for state {{{
func (r *GiteaReconciler) valkeyRunning(ctx context.Context, gitea *hyperv1.Gitea) (bool, error) {
	l := labels(gitea.Name)
	l["app.kubernetes.io/component"] = "cache"
	vk := &valkeyv1.Valkey{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gitea.Name + "-valkey",
			Namespace: gitea.Namespace,
			Labels:    l,
		},
	}
	if err := r.Get(ctx, types.NamespacedName{Name: gitea.Name + "-valkey", Namespace: gitea.Namespace}, vk); err != nil {
		return false, err
	}
	if vk.Status.Ready {
		return true, nil
	}
	return false, nil
}

// }}}

func (r *GiteaReconciler) upsertGiteaSvc(ctx context.Context, gitea *hyperv1.Gitea) error {
	logger := log.FromContext(ctx)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gitea.Name,
			Namespace: gitea.Namespace,
			Labels:    labels(gitea.Name),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels(gitea.Name),
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromString("http"),
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(gitea, svc, r.Scheme); err != nil {
		return err
	}
	err := r.Get(ctx, types.NamespacedName{Name: gitea.Name, Namespace: gitea.Namespace}, svc)
	if err != nil && errors.IsNotFound(err) {
		r.Recorder.Event(gitea, "Normal", "Created",
			fmt.Sprintf("Service %s is created", gitea.Name))
		if err := r.Create(ctx, svc); err != nil {
			logger.Error(err, "failed to create service")
			return err
		} else {
			logger.Info("created gitea service", "ServiceName", gitea.Name)
		}
	} else {
		return err
	}
	return nil
}

func cacheSvc(gitea *hyperv1.Gitea, password string) string {
	cache := "ADAPTER=memory"
	if gitea.Spec.Valkey {
		cache = `ADAPTER=redis
HOST=redis+cluster://:` + password + "@" + gitea.Name + "-valkey." + gitea.Namespace + ".svc:6379/0?pool_size=100&idle_timeout=180s&"
	}
	return cache
}

func sessionSvc(gitea *hyperv1.Gitea, password string) string {
	session := "PROVIDER=db"
	if gitea.Spec.Valkey {
		session = `PROVIDER=redis
PROVIDER_CONFIG=redis+cluster://:` + password + "@" + gitea.Name + "-valkey." + gitea.Namespace + ".svc:6379/0?pool_size=100&idle_timeout=180s&"
	}
	return session
}

func queueSvc(gitea *hyperv1.Gitea, password string) string {
	queue := "TYPE=level"
	if gitea.Spec.Valkey {
		queue = `TYPE=redis
CONN_STR=redis+cluster://:` + password + "@" + gitea.Name + "-valkey." + gitea.Namespace + ".svc:6379/0?pool_size=100&idle_timeout=180s&"
	}
	return queue
}

func (r *GiteaReconciler) getValkeyPassword(ctx context.Context, gitea *hyperv1.Gitea) (string, error) {
	if !gitea.Spec.Valkey {
		return "", nil
	}
	vk := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gitea.Name + "-valkey",
			Namespace: gitea.Namespace,
		},
	}
	if err := r.Get(ctx, types.NamespacedName{Name: gitea.Name + "-valkey", Namespace: gitea.Namespace}, vk); err != nil {
		return "", err
	}
	_, ok := vk.Data["password"]
	if !ok {
		return "", fmt.Errorf("password not found")
	}
	return string(vk.Data["password"]), nil
}

func (r *GiteaReconciler) upsertGiteaSa(ctx context.Context, gitea *hyperv1.Gitea) error {
	logger := log.FromContext(ctx)
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gitea.Name,
			Namespace: gitea.Namespace,
			Labels:    labels(gitea.Name),
		},
	}
	if err := controllerutil.SetControllerReference(gitea, sa, r.Scheme); err != nil {
		return err
	}
	err := r.Get(ctx, types.NamespacedName{Name: gitea.Name, Namespace: gitea.Namespace}, sa)
	if err != nil && errors.IsNotFound(err) {
		r.Recorder.Event(gitea, "Normal", "Created",
			fmt.Sprintf("ServiceAccount %s is created", gitea.Name))
		if err := r.Create(ctx, sa); err != nil {
			logger.Error(err, "failed to create serviceaccount")
			return err
		}
	} else {
		return err
	}
	return nil
}

func matchSecrets(base, match corev1.Secret) bool {
	for k, v := range base.Data {
		val, ok := match.Data[k]
		if !ok {
			return false
		}
		if !bytes.Equal(val, v) {
			return false
		}
	}
	for k, v := range match.Data {
		val, ok := base.Data[k]
		if !ok {
			return false
		}
		if !bytes.Equal(val, v) {
			return false
		}
	}
	return true
}

func (r *GiteaReconciler) upsertSecret(ctx context.Context, gitea *hyperv1.Gitea, name string, strData map[string]string, once bool) error {
	logger := log.FromContext(ctx)
	data := map[string][]byte{}
	for k, v := range strData {
		data[k] = []byte(v)
	}
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: gitea.Namespace,
			Labels:    labels(gitea.Name),
		},
		Data: data,
		Type: "Opaque",
	}
	fetched := secret
	if err := controllerutil.SetControllerReference(gitea, &secret, r.Scheme); err != nil {
		return err
	}
	if err := controllerutil.SetControllerReference(gitea, &fetched, r.Scheme); err != nil {
		return err
	}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: gitea.Namespace}, &fetched)
	if err != nil && errors.IsNotFound(err) {
		r.Recorder.Event(gitea, "Normal", "Created",
			fmt.Sprintf("Secret %s is created", name))
		if err := r.Create(ctx, &secret); err != nil {
			logger.Error(err, "failed to create secret")
			return err
		}
	} else if err == nil && !once {
		if !matchSecrets(fetched, secret) {
			logger.Info("secrets unmatched, updating and restarting")
			r.Recorder.Event(gitea, "Normal", "Updated",
				fmt.Sprintf("Secret %s is updated", name))
			if err := r.Update(ctx, &secret); err != nil {
				logger.Error(err, "failed to update secret")
				return err
			}
			if err := r.restart(ctx, gitea); err != nil {
				logger.Error(err, "failed to restart")
			}
		}
	} else {
		return err
	}
	return nil
}

func ptrInt(num int) *int64 {
	num64 := int64(num)
	return &num64
}
func ptrInt32(num int) *int32 {
	num32 := int32(num)
	return &num32
}

func ptrBool(bol bool) *bool {
	return &bol
}

func ptrString(str string) *string {
	return &str
}

var ENV = map[string]string{
	"GITEA_APP_INI":  "/data/gitea/conf/app.ini",
	"GITEA_CUSTOM":   "/data/gitea",
	"GITEA_WORK_DIR": "/data",
	"GITEA_TEMP":     "/tmp/gitea",
}

func envUpsert(envs []corev1.EnvVar, env corev1.EnvVar) []corev1.EnvVar {
	for i, e := range envs {
		if e.Name == env.Name {
			envs[i] = env
			return envs
		}
	}
	return append(envs, env)
}

func env(in map[string]string) []corev1.EnvVar {
	cpy := ENV
	var ret = []corev1.EnvVar{}
	for k, v := range cpy {
		ret = envUpsert(ret, corev1.EnvVar{Name: k, Value: v})
	}
	for k, v := range in {
		ret = envUpsert(ret, corev1.EnvVar{Name: k, Value: v})
	}
	return ret
}

func image(gitea *hyperv1.Gitea) string {
	//return "gitea/gitea:1.21.11-rootless"
	return gitea.Spec.Image
}

var vol = map[string]string{
	"temp": "/tmp",
	"data": "/data",
}

func volUpsert(vols []corev1.VolumeMount, vol corev1.VolumeMount) []corev1.VolumeMount {
	for i, v := range vols {
		if v.Name == vol.Name {
			vols[i] = vol
			return vols
		}
	}
	return append(vols, vol)
}

func volumes(in map[string]string) []corev1.VolumeMount {
	cpy := vol
	var ret = []corev1.VolumeMount{}
	for k, v := range cpy {
		ret = volUpsert(ret, corev1.VolumeMount{Name: k, MountPath: v})
	}
	for k, v := range in {
		ret = volUpsert(ret, corev1.VolumeMount{Name: k, MountPath: v})
	}

	return ret
}

//nolint:unparam
func (r *GiteaReconciler) restart(ctx context.Context, gitea *hyperv1.Gitea) error {
	logger := log.FromContext(ctx)
	pods := &corev1.PodList{}
	err := r.List(ctx, pods, client.InNamespace(gitea.Namespace))
	if err != nil {
		logger.Error(err, "failed to fetch pods")
		return nil
	}
	for _, pod := range pods.Items {
		if pod.ObjectMeta.Labels["app.kubernetes.io/name"] != gitea.Name {
			continue
		}
		if pod.ObjectMeta.Labels["app.kubernetes.io/component"] == "deployment" && pod.ObjectMeta.Labels["app.kubernetes.io/instance"] == gitea.Name {
			if err := r.Delete(ctx, &pod); err != nil {
				logger.Error(err, "failed deleting pod")
			}
			r.Recorder.Event(gitea, "Normal", "Restarting",
				fmt.Sprintf("Secret update requires restart of pod %s", pod.ObjectMeta.Name))
		}
	}
	return nil
}
func (r *GiteaReconciler) podUP(ctx context.Context, gitea *hyperv1.Gitea) (bool, error) {
	logger := log.FromContext(ctx)
	if gitea.Status.Ready {
		return true, nil
	}
	pods := &corev1.PodList{}
	err := r.List(ctx, pods, client.InNamespace(gitea.Namespace))
	if err != nil {
		logger.Error(err, "failed to fetch pods")
		return false, err
	}
	for _, pod := range pods.Items {
		if pod.ObjectMeta.Labels["app.kubernetes.io/name"] != gitea.Name {
			continue
		}
		if pod.ObjectMeta.Labels["app.kubernetes.io/component"] == "deployment" && pod.ObjectMeta.Labels["app.kubernetes.io/instance"] == gitea.Name {
			for _, cond := range pod.Status.Conditions {
				if cond.Type == "Ready" && cond.Status == "True" {
					if err := r.setCondition(ctx, gitea, "InstanceReady", "True", "InstanceReady", "Pod Up"); err != nil {
						return true, err
					}
					return true, nil
				}
			}
		}
	}
	if err := r.setCondition(ctx, gitea, "InstanceReady", "False", "InstanceReady", "Pod Down"); err != nil {
		return false, err
	}
	return false, nil
}

func (r *GiteaReconciler) apiUP(ctx context.Context, gitea *hyperv1.Gitea) bool {
	logger := log.FromContext(ctx)
	_, err := http.Get("http://" + gitea.Name + "." + gitea.Namespace + ".svc")
	if err != nil {
		if err := r.setCondition(ctx, gitea, "Ready", "False", "Ready", "Api Down"); err != nil {
			logger.Error(err, "Gitea status update failed.")
			return false
		}
		return false
	}
	if !gitea.Status.Ready {
		if err := r.setCondition(ctx, gitea, "Ready", "True", "Ready", "Api Up"); err != nil {
			logger.Error(err, "Gitea status update failed.")
			return false
		}
		gitea.Status.Ready = true
		if err := r.Client.Status().Update(ctx, gitea); err != nil {
			logger.Error(err, "Gitea status update failed.")
			return false
		}
	}

	return true
}

func (r *GiteaReconciler) adminToken(ctx context.Context, gitea *hyperv1.Gitea) error {
	logger := log.FromContext(ctx)
	logger.Info("checking for admin token", "Secret", gitea.Name+"-admin")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gitea.Name + "-admin",
			Namespace: gitea.Namespace,
		},
	}
	if err := controllerutil.SetControllerReference(gitea, secret, r.Scheme); err != nil {
		return err
	}
	if err := r.Get(ctx, types.NamespacedName{Name: gitea.Name + "-admin", Namespace: gitea.Namespace}, secret); err != nil {
		logger.Error(err, "failed getting admin secret "+gitea.Name+"-admin ")
		return err
	}
	tok, ok := secret.Data["token"]
	if ok {
		logger.Info("token detected, skipping", "Secret", gitea.Name+"-admin")
		return nil
	}
	if len(tok) != 0 {
		logger.Info("token detected, skipping", "Secret", gitea.Name+"-admin")
		return nil
	}
	logger.Info("creating admin token", "SecretName", gitea.Name+"-admin")
	body := []byte(`{"name":"admin","scopes": ["write:admin","write:organization","write:repository","write:user"]}`)
	url := "http://" + gitea.Name + "." + gitea.Namespace + ".svc/api/v1/users/gitea/tokens"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		logger.Error(err, "failed creating new http request")
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	auth := base64.StdEncoding.EncodeToString([]byte(string(secret.Data["username"]) + ":" + string(secret.Data["password"])))
	req.Header.Set("Authorization", "Basic "+auth)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Error(err, "failed posting to url "+url)
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Info("failed to close token body")
		}
	}()
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		logger.Error(err, "failed reading body from url "+url)
		return err
	}
	var tresp tokenResponse
	err = json.Unmarshal(body, &tresp)
	if err != nil {
		logger.Error(err, "failed json unmarshal "+url)
		return err
	}
	secret.Data["token"] = []byte(tresp.Token)
	if err := r.Update(ctx, secret); err != nil {
		logger.Error(err, "failed to update secret "+gitea.Name+"-admin."+gitea.Namespace)
		return err
	}
	r.Recorder.Event(gitea, "Normal", "Token", fmt.Sprintf("Admin token created added to %s secret", gitea.Name+"-admin"))

	return nil
}

type tokenResponse struct {
	Id     int      `json:"id"`
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
	Token  string   `json:"sha1"`
}

func (r *GiteaReconciler) upsertGiteaSts(ctx context.Context, gitea *hyperv1.Gitea) error {
	logger := log.FromContext(ctx)
	disk := resource.NewQuantity(50*1024*1024*1024, resource.BinarySI)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gitea.Name,
			Namespace: gitea.Namespace,
			Labels:    labels(gitea.Name),
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels(gitea.Name),
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
					Labels: labels(gitea.Name),
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: gitea.Name,
					/*
						SecurityContext: &corev1.PodSecurityContext{
							RunAsUser:    ptrInt(1000),
							RunAsGroup:   ptrInt(1000),
							FSGroup:      ptrInt(1000),
							RunAsNonRoot: ptrBool(true),
						}, */
					InitContainers: []corev1.Container{
						{
							Name:         "init-directories",
							Env:          env(nil),
							Image:        image(gitea),
							VolumeMounts: volumes(map[string]string{"init": "/usr/sbin"}),
							Command: []string{
								"/usr/sbin/init_directory_structure.sh",
							},
						},
						{
							Name:         "init-app-ini",
							Env:          env(nil),
							Image:        image(gitea),
							VolumeMounts: volumes(map[string]string{"config": "/usr/sbin", "inline-config-sources": "/env-to-ini-mounts/inlines/"}),
							Command: []string{
								"/usr/sbin/config_environment.sh",
							},
						},
						{
							Name:         "configure-gitea",
							Env:          env(map[string]string{"HOME": "/data/gitea/git"}), // "GITEA_ADMIN_USERNAME": "gitea", "GITEA_ADMIN_PASSWORD": "changeme"}),
							Image:        image(gitea),
							VolumeMounts: volumes(map[string]string{"init": "/usr/sbin"}),
							Command: []string{
								"/usr/sbin/configure_gitea.sh",
							},
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: ptrInt(1000),
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:         "gitea",
							Env:          env(map[string]string{"SSH_LISTEN_PORT": "2222", "SSH_PORT": "22", "HOME": "/data/gitea/git", "TMPDIR": "/tmp/gitea"}),
							Image:        image(gitea),
							VolumeMounts: volumes(nil),
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 3000,
								},
								{
									Name:          "ssh",
									ContainerPort: 2222,
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/api/healthz",
										Port: intstr.FromString("http"),
									},
								},
								FailureThreshold:    3,
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
								SuccessThreshold:    1,
								TimeoutSeconds:      1,
							},
							SecurityContext: &corev1.SecurityContext{
								Capabilities: &corev1.Capabilities{
									Add: []corev1.Capability{
										"SYS_CHROOT",
									},
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "init",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  gitea.Name + "-init",
									DefaultMode: ptrInt32(110),
								},
							},
						},
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  gitea.Name + "-config",
									DefaultMode: ptrInt32(110),
								},
							},
						},
						{
							Name: "inline-config-sources",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: gitea.Name + "-inline-config",
								},
							},
						},
						{
							Name: "temp",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{
									Medium: "",
								},
							},
						},
					},
				},
			},
			ServiceName: gitea.Name,
		},
	}
	admins := []corev1.EnvVar{
		{
			Name: "GITEA_ADMIN_USERNAME",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: gitea.Name + "-admin",
					},
					Key: "username",
				},
			},
		},
		{
			Name: "GITEA_ADMIN_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: gitea.Name + "-admin",
					},
					Key: "password",
				},
			},
		},
	}
	sts.Spec.Template.Spec.InitContainers[2].Env = append(sts.Spec.Template.Spec.InitContainers[2].Env, admins...)

	dbs := []corev1.EnvVar{
		{
			Name:  "GITEA__DATABASE__DB_TYPE",
			Value: "postgres",
		},
		{
			Name:  "GITEA__DATABASE__HOST",
			Value: gitea.Name + "-" + gitea.Name + "." + gitea.Namespace + ".svc:5432",
		},
		{
			Name:  "GITEA__DATABASE__NAME",
			Value: gitea.Name,
		},
		{
			Name: "GITEA__DATABASE__USER",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: gitea.Name + "." + gitea.Name + "-" + gitea.Name + ".credentials.postgresql.acid.zalan.do",
					},
					Key: "username",
				},
			},
		},
		{
			Name: "GITEA__DATABASE__PASSWD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: gitea.Name + "." + gitea.Name + "-" + gitea.Name + ".credentials.postgresql.acid.zalan.do",
					},
					Key: "password",
				},
			},
		},
		{
			Name:  "GITEA__DATABASE__SSL_MODE",
			Value: "require",
		},
	}
	sts.Spec.Template.Spec.InitContainers[1].Env = append(sts.Spec.Template.Spec.InitContainers[1].Env, dbs...)
	sts.Spec.Template.Spec.InitContainers[2].Env = append(sts.Spec.Template.Spec.InitContainers[2].Env, dbs...)
	sts.Spec.Template.Spec.Containers[0].Env = append(sts.Spec.Template.Spec.Containers[0].Env, dbs...)
	envs := []corev1.EnvVar{
		{
			Name:  "GITEA__SERVER__START_SSH_SERVER",
			Value: "false",
		},
		{
			Name:  "START_SSH_SERVER",
			Value: "false",
		},
		{
			Name:  "SSH_LOG_LEVEL",
			Value: "INFO",
		},
	}
	sts.Spec.Template.Spec.Containers[0].Env = append(sts.Spec.Template.Spec.Containers[0].Env, envs...)
	if err := controllerutil.SetControllerReference(gitea, sts, r.Scheme); err != nil {
		return err
	}
	err := r.Get(ctx, types.NamespacedName{Name: gitea.Name, Namespace: gitea.Namespace}, sts)
	if err != nil && errors.IsNotFound(err) {
		r.Recorder.Event(gitea, "Normal", "Created",
			fmt.Sprintf("StatefulSet %s is updated", gitea.Name))
		if err := r.Create(ctx, sts); err != nil {
			logger.Error(err, "failed to create statefulset")
			return err
		}
	} else {
		return err
	}
	return nil
}

func (r *GiteaReconciler) upsertGiteaIngress(ctx context.Context, gitea *hyperv1.Gitea) error {
	logger := log.FromContext(ctx)
	hostname := gitea.Spec.Ingress.Host
	if hostname == "" {
		hostname = "git.example.com"
	}
	prefix := netv1.PathTypePrefix
	ing := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        gitea.Name,
			Namespace:   gitea.Namespace,
			Labels:      labels(gitea.Name),
			Annotations: gitea.Spec.Ingress.Annotations,
		},
		Spec: netv1.IngressSpec{
			IngressClassName: ptrString("nginx"),
			Rules: []netv1.IngressRule{
				{
					Host: hostname,
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &prefix,
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: gitea.Name,
											Port: netv1.ServiceBackendPort{
												Name: "http",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			TLS: []netv1.IngressTLS{
				{
					Hosts: []string{
						hostname,
					},
					SecretName: gitea.Name + "-ingress-tls",
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(gitea, ing, r.Scheme); err != nil {
		return err
	}
	err := r.Get(ctx, types.NamespacedName{Name: gitea.Name, Namespace: gitea.Namespace}, ing)
	if err != nil && errors.IsNotFound(err) {
		if err := r.Create(ctx, ing); err != nil {
			logger.Error(err, "failed to create ingress")
			return err
		} else {
			logger.Info("created gitea ingress", "IngressName", gitea.Name)
		}
	} else {
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GiteaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 3}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&zalandov1.Postgresql{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&netv1.Ingress{}).
		Owns(&valkeyv1.Valkey{}).
		For(&hyperv1.Gitea{}).
		Complete(r)
}
