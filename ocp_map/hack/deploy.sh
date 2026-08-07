#!/usr/bin/env bash
# Build (via OpenShift BuildConfig) and deploy the revisionHistoryLimit mutating webhook.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NAMESPACE="${NAMESPACE:-ocp-map-webhooks}"
APP_NAME="${APP_NAME:-revision-history-webhook}"
IMAGE_TAG="${IMAGE_TAG:-latest}"
IMAGE="${IMAGE:-image-registry.openshift-image-registry.svc:5000/${NAMESPACE}/${APP_NAME}:${IMAGE_TAG}}"

echo "==> Applying Namespace, ServiceAccount, Service (serving-cert)"
oc apply -f "${ROOT}/manifests/namespace.yaml"
oc apply -f "${ROOT}/manifests/serviceaccount.yaml"
oc apply -f "${ROOT}/manifests/service.yaml"

echo "==> Waiting for service-CA secret revision-history-webhook-tls"
for _ in $(seq 1 60); do
  if oc -n "$NAMESPACE" get secret revision-history-webhook-tls >/dev/null 2>&1; then
    echo "secret ready"
    break
  fi
  sleep 2
done
oc -n "$NAMESPACE" get secret revision-history-webhook-tls -o name

if [[ "${SKIP_BUILD:-0}" != "1" ]]; then
  echo "==> Ensuring ImageStream + BuildConfig"
  if ! oc -n "$NAMESPACE" get imagestream "$APP_NAME" >/dev/null 2>&1; then
    oc -n "$NAMESPACE" create imagestream "$APP_NAME"
  fi
  if ! oc -n "$NAMESPACE" get buildconfig "$APP_NAME" >/dev/null 2>&1; then
    oc -n "$NAMESPACE" new-build --binary --name="$APP_NAME" \
      --strategy=docker \
      --to="${APP_NAME}:${IMAGE_TAG}"
  fi

  echo "==> Starting binary build from ${ROOT}"
  oc -n "$NAMESPACE" start-build "$APP_NAME" --from-dir="$ROOT" --follow --wait
fi

echo "==> Deploying webhook workload (image=${IMAGE})"
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT
sed "s|IMAGE_PLACEHOLDER|${IMAGE}|g" \
  "${ROOT}/manifests/deployment.yaml" > "${tmpdir}/deployment.yaml"
oc apply -f "${tmpdir}/deployment.yaml"

echo "==> Waiting for Deployment rollout"
oc -n "$NAMESPACE" rollout status deployment/"$APP_NAME" --timeout=180s

echo "==> Applying MutatingWebhookConfiguration"
oc apply -f "${ROOT}/manifests/mutatingwebhookconfiguration.yaml"

echo "==> Waiting for caBundle injection"
for _ in $(seq 1 30); do
  size="$(oc get mutatingwebhookconfiguration set-revision-history-limit \
    -o jsonpath='{.webhooks[0].clientConfig.caBundle}' | wc -c | tr -d ' ')"
  if [[ "${size}" -gt 20 ]]; then
    echo "caBundle injected (${size} bytes base64)"
    break
  fi
  sleep 2
done

cat <<EOF

Deployed.

Test:
  oc apply -f ${ROOT}/examples/test-deployment.yaml
  oc -n default get deploy revision-history-demo -o jsonpath='{.spec.revisionHistoryLimit}{"\\n"}'
  # expect: 3

Cleanup:
  ${ROOT}/hack/undeploy.sh
EOF
