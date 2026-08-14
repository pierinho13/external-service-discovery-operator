# External Service Discovery Operator

Kubernetes-native service discovery for external workloads.

[![CI](https://github.com/pierinho13/external-service-discovery-operator/actions/workflows/ci.yaml/badge.svg)](https://github.com/pierinho13/external-service-discovery-operator/actions/workflows/ci.yaml)
[![Lint](https://github.com/pierinho13/external-service-discovery-operator/actions/workflows/lint.yml/badge.svg)](https://github.com/pierinho13/external-service-discovery-operator/actions/workflows/lint.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/pierinho13/external-service-discovery-operator)](https://github.com/pierinho13/external-service-discovery-operator)
[![GitHub Release](https://img.shields.io/github/v/release/pierinho13/external-service-discovery-operator?display_name=tag&sort=semver)](https://github.com/pierinho13/external-service-discovery-operator/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/pierinho13/external-service-discovery-operator)](https://goreportcard.com/report/github.com/pierinho13/external-service-discovery-operator)
[![License](https://img.shields.io/github/license/pierinho13/external-service-discovery-operator)](LICENSE)
[![Kubernetes Operator](https://img.shields.io/badge/Kubernetes-Operator-326CE5?logo=kubernetes&logoColor=white)](docs/getting-started.md)
[![Helm OCI](https://img.shields.io/badge/Helm-OCI-0F1689?logo=helm&logoColor=white)](charts/external-service-discovery-operator/README.md)



This Kubernetes operator discovers workloads running outside Kubernetes and projects them into native, selectorless `Service` and `EndpointSlice` resources. Kubernetes consumers such as Traefik, Gateway API implementations, ingress controllers, and workloads can consume the generated Service when their implementation supports selectorless Service endpoints.

The operator is a control plane only: it contains no proxy or data plane, performs no load balancing, and creates no ingress-controller-specific resources.


<img width="1448" height="1086" alt="image" src="https://github.com/user-attachments/assets/19c4287d-eb57-4419-afb4-93d8d6bf5520" />



⭐ Drop a star to support External Service Discovery Operator ⭐

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

## Optional active health checks

An optional health check can evaluate every discovered address independently. Without this field, all discovered endpoints remain ready exactly as in previous releases.

```yaml
spec:
  healthCheck:
    type: HTTP
    port: 8080
    path: /estasbien
    expectedStatuses: [200]
    interval: 10s
    timeout: 3s
    successThreshold: 1
    failureThreshold: 3
```

`HTTP` and `HTTPS` checks issue a GET request to each address; `host` optionally controls the HTTP Host header and HTTPS TLS server name. `TCP` checks only establish a connection and therefore reject `path`, `host`, and `expectedStatuses`.

Discovered addresses stay present in the `EndpointSlice`, but unhealthy endpoints have `conditions.ready=false` and `conditions.serving=false`. A new endpoint starts not ready until it reaches `successThreshold`; a healthy endpoint is marked not ready only after `failureThreshold` consecutive failures. The operator records aggregate counts and per-address results under `status`, and health checks run independently from the DNS refresh interval.

## Why not just use ExternalName?

`ExternalName` and `DiscoveredService` solve related but different problems. An `ExternalName` Service exposes a DNS alias and leaves name resolution to consumers. This operator resolves DNS itself and represents the resulting IPv4 addresses as a selectorless Service plus a Kubernetes `EndpointSlice`.

`ExternalName` is usually the simpler choice when a Service name only needs to point to an external hostname. Use `DiscoveredService` when materializing concrete external addresses as Kubernetes-native endpoints is itself useful, for example when one name represents several backends or addresses change behind stable DNS names.

For a detailed comparison and practical scenarios, see [When should I use this operator?](docs/use-cases.md).

`make test` runs both fast unit tests and the envtest integration suite. The `setup-envtest` helper downloads the Kubernetes API server and etcd binaries into its standard user data directory; override `ENVTEST_K8S_VERSION` when a different Kubernetes version is required.

## Documentation

- [Getting started](docs/getting-started.md)
- [Use cases and ExternalName comparison](docs/use-cases.md)
- [Development](docs/development.md)
- [Operations](docs/operations.md)
- [Releasing](docs/releasing.md)
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)

## Non-goals

External Service Discovery Operator is intentionally focused on one thing: keeping Kubernetes `Service` and `EndpointSlice` resources synchronized with workloads that run outside the cluster.

It does not provide:

- Traffic proxying or load balancing
- Service mesh functionality
- Cloud inventory discovery by labels or tags
- Ingress, Traefik, or Gateway API resource management
- VM agents or sidecars

The operator stays in the control plane and leaves traffic handling to Kubernetes consumers such as Traefik, Gateway API implementations, or applications using the generated `Service`.
