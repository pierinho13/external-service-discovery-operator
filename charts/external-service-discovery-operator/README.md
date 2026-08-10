# External Service Discovery Operator Helm chart

Installs the controller, CRD, ServiceAccount, manager RBAC, and leader-election RBAC.

```bash
helm upgrade --install external-service-discovery-operator \
  oci://ghcr.io/pierinho13/charts/external-service-discovery-operator \
  --version <VERSION> \
  --namespace external-service-discovery-operator-system \
  --create-namespace
```

Important values:

| Value | Default | Purpose |
|---|---|---|
| `image.repository` | `ghcr.io/pierinho13/external-service-discovery-operator` | Controller image |
| `image.tag` | chart `appVersion` | Image tag override |
| `leaderElection.enabled` | `true` | Enable leader election |
| `discovery.refreshInterval` | `1m` | Controller-wide dynamic discovery refresh |
| `resources` | small requests/limits | Controller resources |

CRDs are installed from the chart's `crds/` directory and are intentionally retained by Helm on uninstall.
