# external-service-discovery-operator

[![CI](https://github.com/pierinho13/external-service-discovery-operator/actions/workflows/ci.yaml/badge.svg)](https://github.com/pierinho13/external-service-discovery-operator/actions/workflows/ci.yaml)
[![Lint](https://github.com/pierinho13/external-service-discovery-operator/actions/workflows/lint.yml/badge.svg)](https://github.com/pierinho13/external-service-discovery-operator/actions/workflows/lint.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/pierinho13/external-service-discovery-operator)](https://goreportcard.com/report/github.com/pierinho13/external-service-discovery-operator)
[![License](https://img.shields.io/github/license/pierinho13/external-service-discovery-operator)](LICENSE)

This Kubernetes operator discovers workloads running outside Kubernetes and projects them into native, selectorless `Service` and `EndpointSlice` resources. Kubernetes consumers such as Traefik, Gateway API implementations, and NGINX can use the generated Service normally.

The operator is a control plane only: it contains no proxy or data plane, performs no load balancing, and creates no ingress-controller-specific resources.

## Architecture

```text
External workloads
        |
        v
DiscoveredService
        |
        v
Discovery Provider
        |
        v
external-service-discovery-operator
        |
        +--> Service
        |
        +--> EndpointSlice
        |
        v
Kubernetes consumers
Traefik / Gateway API / etc.
```

Discovery is separated behind a small provider interface. Static discovery accepts IPv4 addresses; DNS discovery resolves A/AAAA lookups but currently materializes only IPv4 addresses. Cloud-specific providers and Service Directory are future work.

## Quick start

Install a published release with Helm:

```sh
helm upgrade --install external-service-discovery-operator \
  oci://ghcr.io/pierinho13/charts/external-service-discovery-operator \
  --version <VERSION> \
  --namespace external-service-discovery-operator-system \
  --create-namespace
```

For local development:

```sh
make install
make run
kubectl apply -f config/samples/discovery_v1alpha1_discoveredservice.yaml
```

```yaml
apiVersion: discovery.k8sready.com/v1alpha1
kind: DiscoveredService
metadata:
  name: tomcat-erp
  namespace: default
spec:
  discovery:
    static:
      addresses: [10.140.0.11, 10.140.0.12, 10.140.0.13]
  ports:
  - name: http
    port: 80
    targetPort: 8080
    protocol: TCP
```

This creates `Service/default/tomcat-erp`, without a selector, exposing port 80, and `EndpointSlice/default/tomcat-erp`, labeled for that Service and containing the three ready endpoints on target port 8080. When `targetPort` is omitted it defaults logically to `port`. Both children have controller owner references and are continuously restored to the declared state. A preexisting Service or EndpointSlice with the deterministic name is reported as a `ResourceConflict` and is never adopted.

Provider selection is handled by a small resolver between the controller and concrete providers. This keeps provider-specific branching out of reconciliation and allows a future provider to be added by extending the API union and resolver wiring.

## DNS discovery

Several machine names can be resolved into one deterministic endpoint set:

```yaml
spec:
  discovery:
    dns:
      names:
      - tomcat-01.internal.example.com
      - tomcat-02.internal.example.com
      - tomcat-03.internal.example.com
  ports:
  - name: http
    port: 8080
```

```text
DNS names -> IPv4 addresses -> EndpointSlice endpoints
```

A single name may also publish multiple A records:

```yaml
spec:
  discovery:
    dns:
      names:
      - tomcat.internal.example.com
  ports:
  - name: http
    port: 8080
```

```text
tomcat.internal.example.com  A 10.140.0.11
                             A 10.140.0.12
                             A 10.140.0.13

EndpointSlice: 10.140.0.11, 10.140.0.12, 10.140.0.13
```

Unlike an `ExternalName` Service, which delegates resolution to each DNS client, this operator resolves names and materializes every discovered address into an EndpointSlice. DNS resources refresh every minute by default; configure the controller-wide period with `--discovery-refresh-interval`. If any name fails or produces no usable IPv4 address, discovery fails closed: the existing EndpointSlice and its endpoint count remain unchanged while `Ready=False` reports `DiscoveryFailed`.

`make test` runs both fast unit tests and the envtest integration suite. The `setup-envtest` helper downloads the Kubernetes API server and etcd binaries into its standard user data directory; override `ENVTEST_K8S_VERSION` when a different Kubernetes version is required.

## Documentation

- [Getting started](docs/getting-started.md)
- [Development](docs/development.md)
- [Operations](docs/operations.md)
- [Releasing](docs/releasing.md)
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)

## Project boundaries

The current operator scope deliberately excludes cloud APIs, SRV records, custom DNS servers, TTL-aware or per-resource refresh scheduling, IPv6 EndpointSlices, health checking, Traefik or Gateway API CRDs, proxies, VM agents, and finalizers.
