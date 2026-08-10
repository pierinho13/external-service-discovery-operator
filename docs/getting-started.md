# Getting started

## Helm installation

```bash
helm upgrade --install external-service-discovery-operator \
  oci://ghcr.io/pierinho13/charts/external-service-discovery-operator \
  --version <VERSION> \
  --namespace external-service-discovery-operator-system \
  --create-namespace
```

Verify the Deployment and CRD:

```bash
kubectl rollout status deployment/external-service-discovery-operator -n external-service-discovery-operator-system
kubectl get crd discoveredservices.discovery.k8sready.com
```

Apply either static or DNS discovery from `config/samples/`, then inspect the generated resources with `kubectl get service,endpointslice`.
