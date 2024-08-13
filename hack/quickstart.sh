#!/bin/sh

export SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

kubectl apply -f $SCRIPT_DIR/postgres-operator.yaml
if [ ! -z ${VALKEY} ]; then
	kubectl apply -f https://raw.githubusercontent.com/hyperspike/valkey-operator/main/dist/install.yaml
fi
if [ ! -z ${TLS} ]; then
	LATEST=$(curl -s curl https://api.github.com/repos/cert-manager/cert-manager/releases/latest  | jq -cr .tag_name)
	kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/${LATEST}/cert-manager.yaml
	sleep 2
	kubectl apply -f $SCRIPT_DIR/issuer.yaml
fi
if [ ! -z ${PROMETHEUS} ]; then
	LATEST=$(curl -s https://api.github.com/repos/prometheus-operator/prometheus-operator/releases/latest | jq -cr .tag_name)
	curl -sL https://github.com/prometheus-operator/prometheus-operator/releases/download/${LATEST}/bundle.yaml | kubectl create -f -
	kubectl apply -f $SCRIPT_DIR/prometheus.yaml
fi

if [ "${PROVIDER}" == "aws" ] ; then
	LATEST=$(curl -s https://api.github.com/repos/kubernetes/ingress-nginx/releases/latest | jq -cr .tag_name)
	kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/${LATEST}/deploy/static/provider/aws/deploy.yaml
fi
