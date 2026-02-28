#!/usr/bin/env bash

# Copyright 2026 The KubeFlag Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


# Generates certs, configures the Kind cluster, and starts the webhook locally.
#
# Usage:
#   ./hack/run-webhook.sh             # uses default context kind-kubeflag
#   KUBE_CONTEXT=kind-other ./hack/run-webhook.sh
#
# Environment overrides:
#   KUBE_CONTEXT   - kubectl context pointing at the Kind cluster (default: kind-kubeflag)
#   WEBHOOK_PORT   - local port for the webhook server          (default: 9876)
#   REGEN_CERTS    - set to "1" to force regenerating certs     (default: auto)

set -euo pipefail

# ── Config ────────────────────────────────────────────────────────────────────
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CERTS_DIR="${REPO_ROOT}/local-certs"
KUBECONFIG_FILE="${REPO_ROOT}/dev.kubeconfig"
BINARY="${REPO_ROOT}/bin/webhook"
CONTEXT="${KUBE_CONTEXT:-kind-kubeflag}"
WEBHOOK_PORT="${WEBHOOK_PORT:-9876}"
KIND_NODE="${CONTEXT#kind-}-control-plane"          # strips "kind-" prefix → e.g. kubeflag-control-plane

# ── Helpers ───────────────────────────────────────────────────────────────────
info()  { echo "  $*"; }
step()  { echo; echo "── $* ──────────────────────────────────────────"; }
die()   { echo "ERROR: $*" >&2; exit 1; }

require() {
  for cmd in "$@"; do
    command -v "$cmd" &>/dev/null || die "'$cmd' is not installed or not in PATH"
  done
}

# ── Prerequisites ─────────────────────────────────────────────────────────────
require kubectl docker openssl go

step "1. Discover Kind host IP"
# The Kind API server calls webhooks from inside the cluster network.
# The host machine is reachable via the default gateway of the Kind container.
KIND_CONTAINER=$(docker ps --filter "name=${KIND_NODE}" --format "{{.Names}}" | head -1)
if [[ -z "${KIND_CONTAINER}" ]]; then
  # Fallback: try generic kind-control-plane container name
  KIND_CONTAINER=$(docker ps --filter "name=kind-control-plane" --format "{{.Names}}" | head -1)
fi
[[ -z "${KIND_CONTAINER}" ]] && die "Could not find Kind control-plane container. Is your cluster running? (context: ${CONTEXT})"

HOST_IP=$(docker exec "${KIND_CONTAINER}" ip route show default | awk '/default/ {print $3}')
[[ -z "${HOST_IP}" ]] && die "Could not determine host gateway IP from Kind container '${KIND_CONTAINER}'"
info "Kind container : ${KIND_CONTAINER}"
info "Host IP (SANs) : ${HOST_IP}"

# ── Kubeconfig ────────────────────────────────────────────────────────────────
step "2. Refresh kubeconfig"
kubectl config view --context "${CONTEXT}" --minify --raw > "${KUBECONFIG_FILE}"
info "API server: $(grep server "${KUBECONFIG_FILE}" | awk '{print $2}')"

# ── Certificates ──────────────────────────────────────────────────────────────
step "3. TLS certificates"
mkdir -p "${CERTS_DIR}"

# Check if existing cert covers the current host IP and is still valid (>7 days)
REGEN=0
if [[ "${REGEN_CERTS:-0}" == "1" ]]; then
  REGEN=1
  info "Forced regeneration requested."
elif [[ ! -f "${CERTS_DIR}/webhook.crt" ]]; then
  REGEN=1
  info "No existing cert found — generating."
else
  # Expired in the next 7 days?
  if ! openssl x509 -checkend 604800 -noout -in "${CERTS_DIR}/webhook.crt" 2>/dev/null; then
    REGEN=1
    info "Cert expires within 7 days — regenerating."
  # Missing the current host IP in SANs?
  elif ! openssl x509 -text -noout -in "${CERTS_DIR}/webhook.crt" 2>/dev/null \
        | grep -q "IP Address:${HOST_IP}"; then
    REGEN=1
    info "Cert does not cover current host IP ${HOST_IP} — regenerating."
  else
    info "Existing cert is valid and covers ${HOST_IP} — reusing."
  fi
fi

if [[ "${REGEN}" == "1" ]]; then
  info "Generating CA key + self-signed cert..."
  openssl genrsa -out "${CERTS_DIR}/ca.key" 4096 2>/dev/null
  openssl req -new -x509 -days 365 -key "${CERTS_DIR}/ca.key" \
    -subj "/CN=kubeflag-local-ca" \
    -out "${CERTS_DIR}/ca.crt" 2>/dev/null

  info "Generating webhook key + cert (SANs: localhost, 127.0.0.1, ${HOST_IP})..."
  cat > "${CERTS_DIR}/san.cnf" <<EOF
[req]
distinguished_name = req_distinguished_name
req_extensions     = v3_req
prompt             = no

[req_distinguished_name]
CN = kubeflag-webhook

[v3_req]
keyUsage         = keyEncipherment, dataEncipherment
extendedKeyUsage = serverAuth
subjectAltName   = @alt_names

[alt_names]
DNS.1 = localhost
DNS.2 = kubeflag-webhook-service.kubeflag-system.svc
DNS.3 = kubeflag-webhook-service.kubeflag-system.svc.cluster.local
IP.1  = 127.0.0.1
IP.2  = ${HOST_IP}
EOF

  openssl genrsa -out "${CERTS_DIR}/webhook.key" 2048 2>/dev/null
  openssl req -new -key "${CERTS_DIR}/webhook.key" \
    -out "${CERTS_DIR}/webhook.csr" \
    -config "${CERTS_DIR}/san.cnf" 2>/dev/null
  openssl x509 -req \
    -in "${CERTS_DIR}/webhook.csr" \
    -CA "${CERTS_DIR}/ca.crt" -CAkey "${CERTS_DIR}/ca.key" -CAcreateserial \
    -out "${CERTS_DIR}/webhook.crt" \
    -days 365 -extensions v3_req -extfile "${CERTS_DIR}/san.cnf" 2>/dev/null

  EXPIRY=$(openssl x509 -noout -enddate -in "${CERTS_DIR}/webhook.crt" | cut -d= -f2)
  info "Cert generated — expires: ${EXPIRY}"
fi

# ── Webhook configurations ────────────────────────────────────────────────────
step "4. Apply webhook configurations to cluster"
CA_BUNDLE=$(base64 -w0 "${CERTS_DIR}/ca.crt")

kubectl apply --context "${CONTEXT}" -f - <<EOF
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: kubeflag-challenge-validating
webhooks:
  - name: vchallenge.kubeflag.io
    admissionReviewVersions: ["v1", "v1beta1"]
    sideEffects: None
    failurePolicy: Fail
    timeoutSeconds: 10
    matchPolicy: Equivalent
    clientConfig:
      url: "https://${HOST_IP}:${WEBHOOK_PORT}/validate-kubeflag-io-v1alpha1-challenge"
      caBundle: ${CA_BUNDLE}
    rules:
      - apiGroups:   ["kubeflag.io"]
        apiVersions: ["v1alpha1"]
        operations:  ["CREATE", "UPDATE"]
        resources:   ["challenges"]
        scope:       "*"
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: kubeflag-consumer-validating
webhooks:
  - name: vconsumer.kubeflag.io
    admissionReviewVersions: ["v1", "v1beta1"]
    sideEffects: None
    failurePolicy: Fail
    timeoutSeconds: 10
    matchPolicy: Equivalent
    clientConfig:
      url: "https://${HOST_IP}:${WEBHOOK_PORT}/validate-kubeflag-io-v1alpha1-consumer"
      caBundle: ${CA_BUNDLE}
    rules:
      - apiGroups: ["kubeflag.io"]
        apiVersions: ["v1alpha1"]
        operations: ["CREATE", "UPDATE"]
        resources: ["consumers"]
        scope: "*"
---
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: kubeflag-challenge-mutating
webhooks:
  - name: mchallenge.kubeflag.io
    admissionReviewVersions: ["v1"]
    sideEffects: None
    failurePolicy: Fail
    timeoutSeconds: 10
    matchPolicy: Equivalent
    clientConfig:
      url: "https://${HOST_IP}:${WEBHOOK_PORT}/mutate-challenge-v1"
      caBundle: ${CA_BUNDLE}
    rules:
      - apiGroups:   ["kubeflag.io"]
        apiVersions: ["v1alpha1"]
        operations:  ["CREATE", "UPDATE"]
        resources:   ["challenges"]
        scope:       "*"
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: kubeflag-challengeinstance-validating
webhooks:
  - name: vchallengeinstance.kubeflag.io
    admissionReviewVersions: ["v1", "v1beta1"]
    sideEffects: None
    failurePolicy: Fail
    timeoutSeconds: 10
    matchPolicy: Equivalent
    clientConfig:
      url: "https://${HOST_IP}:${WEBHOOK_PORT}/validate-kubeflag-io-v1alpha1-challengeinstance"
      caBundle: ${CA_BUNDLE}
    rules:
      - apiGroups:   ["kubeflag.io"]
        apiVersions: ["v1alpha1"]
        operations:  ["CREATE", "UPDATE"]
        resources:   ["challengeinstances"]
        scope:       "*"
---
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: kubeflag-challengeinstance-mutating
webhooks:
  - name: mchallengeinstance.kubeflag.io
    admissionReviewVersions: ["v1"]
    sideEffects: None
    failurePolicy: Fail
    timeoutSeconds: 10
    matchPolicy: Equivalent
    clientConfig:
      url: "https://${HOST_IP}:${WEBHOOK_PORT}/mutate-challengeinstance-v1"
      caBundle: ${CA_BUNDLE}
    rules:
      - apiGroups:   ["kubeflag.io"]
        apiVersions: ["v1alpha1"]
        operations:  ["CREATE"]
        resources:   ["challengeinstances"]
        scope:       "*"
EOF
info "ValidatingWebhookConfiguration and MutatingWebhookConfiguration applied."
info "Webhook URL: https://${HOST_IP}:${WEBHOOK_PORT}"

# ── Kill existing webhook on the same port ────────────────────────────────────
step "5. Free port ${WEBHOOK_PORT}"
OLD_PID=$(lsof -ti :"${WEBHOOK_PORT}" 2>/dev/null || true)
if [[ -n "${OLD_PID}" ]]; then
  info "Killing existing process on :${WEBHOOK_PORT} (PID ${OLD_PID})"
  kill "${OLD_PID}" 2>/dev/null || true
  sleep 1
else
  info "Port ${WEBHOOK_PORT} is free."
fi

# ── Build ─────────────────────────────────────────────────────────────────────
step "6. Build webhook binary"
go build -o "${BINARY}" "${REPO_ROOT}/cmd/webhook/"
info "Built: ${BINARY}"

# ── Run ───────────────────────────────────────────────────────────────────────
step "7. Start webhook server (Ctrl-C to stop)"
info "Listening on :${WEBHOOK_PORT}"
info "Certs: ${CERTS_DIR}"
info ""

KUBECONFIG="${KUBECONFIG_FILE}" exec "${BINARY}" \
  --tls-cert-path="${CERTS_DIR}/webhook.crt" \
  --tls-key-path="${CERTS_DIR}/webhook.key"  \
  --ca-bundle="${CERTS_DIR}/ca.crt"          \
  --listen-port="${WEBHOOK_PORT}"             \
  --namespace=default                         \
  --log-format=Console
