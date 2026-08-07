# ocp-map — Deployment `revisionHistoryLimit` mutating webhook

OpenShift mutating admission webhook that forces every `Deployment` to use:

```yaml
spec:
  revisionHistoryLimit: 3
```

Applies on `CREATE` and `UPDATE` for `apps/v1` Deployments cluster-wide.

## Layout

```
/var/tmp/ocp_map/
├── README.md
├── Dockerfile
├── go.mod
├── cmd/webhook/main.go          # HTTPS webhook server
├── manifests/                   # OpenShift / Kubernetes objects
│   ├── namespace.yaml
│   ├── serviceaccount.yaml
│   ├── service.yaml             # service-CA serving cert
│   ├── deployment.yaml
│   ├── mutatingwebhookconfiguration.yaml
│   └── kustomization.yaml
├── examples/test-deployment.yaml
└── hack/
    ├── deploy.sh
    └── undeploy.sh
```

## Behaviour

| Incoming `revisionHistoryLimit` | Result |
|---------------------------------|--------|
| unset | patched to `3` |
| any value other than `3` | patched to `3` |
| already `3` | allowed, no patch |

TLS uses the OpenShift **service CA**:

- Service annotation `service.beta.openshift.io/serving-cert-secret-name`
- Webhook annotation `service.beta.openshift.io/inject-cabundle: "true"`

## Deploy

Requires `oc` logged into a cluster with permission to create namespaces, builds, and cluster-scoped `MutatingWebhookConfiguration`.

```bash
cd /var/tmp/ocp_map
./hack/deploy.sh
```

The script:

1. Creates `ocp-map-webhooks` and the Service (so the serving-cert Secret appears)
2. Builds the image with an OpenShift binary `BuildConfig`
3. Rolls out the webhook Deployment
4. Registers the `MutatingWebhookConfiguration`

Override image / namespace if needed:

```bash
NAMESPACE=ocp-map-webhooks IMAGE_TAG=v1 ./hack/deploy.sh
SKIP_BUILD=1 IMAGE=quay.io/myorg/revision-history-webhook:v1 ./hack/deploy.sh
```

## Test

```bash
oc apply -f examples/test-deployment.yaml
oc -n default get deploy revision-history-demo \
  -o jsonpath='{.spec.revisionHistoryLimit}{"\n"}'
# expect: 3

oc -n ocp-map-webhooks logs deploy/revision-history-webhook | tail
```

Try changing it back:

```bash
oc -n default patch deploy revision-history-demo \
  -p '{"spec":{"revisionHistoryLimit":10}}'
oc -n default get deploy revision-history-demo \
  -o jsonpath='{.spec.revisionHistoryLimit}{"\n"}'
# expect: 3 again
```

## Remove

```bash
./hack/undeploy.sh
```

Delete the webhook configuration **before** (or with) the project so `failurePolicy: Fail` does not block later Deployment changes while the service is gone.

## Local build (optional)

```bash
podman build -t revision-history-webhook:dev /var/tmp/ocp_map
```

## Notes

- `failurePolicy: Fail` — if the webhook pods are down, Deployment CREATE/UPDATE is rejected.
- Scope the webhook with `namespaceSelector` / `objectSelector` in `manifests/mutatingwebhookconfiguration.yaml` if you do not want cluster-wide enforcement.
- The webhook Deployment itself is also mutated once the configuration is registered.
