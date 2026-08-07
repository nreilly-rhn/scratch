#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NAMESPACE="${NAMESPACE:-ocp-map-webhooks}"

oc delete -f "${ROOT}/manifests/mutatingwebhookconfiguration.yaml" --ignore-not-found
oc delete -f "${ROOT}/examples/test-deployment.yaml" --ignore-not-found
oc delete project "$NAMESPACE" --ignore-not-found

echo "Removed webhook config and project ${NAMESPACE}."
