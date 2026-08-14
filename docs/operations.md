# Operations

Inspect reconciliation with:

```bash
kubectl get dsvc -A
kubectl describe dsvc <name>
kubectl logs deployment/external-service-discovery-operator -n external-service-discovery-operator-system
```

`Ready=False` with `DiscoveryFailed` indicates provider resolution failed; the last successful EndpointSlice remains materialized. `ResourceConflict` means the deterministic Service or EndpointSlice name is occupied by an unmanaged resource.

DNS resources refresh every minute by default. Configure `discovery.refreshInterval` in Helm values.

Logging defaults to `INFO`. Set `log.level: DEBUG` in Helm values to log each
reconciliation, discovery result, and endpoint health check, including its
address, probe type, port, path, readiness result, counters, and duration.

```yaml
log:
  level: DEBUG
```

Helm installs CRDs only on initial installation and does not upgrade or remove them automatically. Apply changed CRDs before upgrading a release that changes the schema.
