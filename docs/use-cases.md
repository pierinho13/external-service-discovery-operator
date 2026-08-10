# When should I use this operator?

`external-service-discovery-operator` represents workloads outside a Kubernetes cluster as a selectorless `Service` and an `EndpointSlice`. It is intended for cases where Kubernetes consumers benefit from seeing concrete external addresses through native endpoint resources.

It does not replace `ExternalName`. The two approaches expose external systems differently:

```text
ExternalName

Kubernetes Service
        |
        v
    DNS alias
        |
        v
external hostname
```

The consumer continues to resolve and use the external DNS name.

```text
DiscoveredService

DiscoveredService
        |
        v
  DNS resolution
        |
        v
concrete IPv4 addresses
        |
        v
   EndpointSlice
        |
        v
selectorless Service
        |
        v
Kubernetes consumer
```

The operator materializes the resolved targets as Kubernetes-native endpoints. It does not proxy traffic or load balance traffic itself; those remain responsibilities of clients and other Kubernetes networking components.

## ExternalName compared with DiscoveredService

| Capability | `ExternalName` | `DiscoveredService` |
| --- | --- | --- |
| Reference an external hostname | Yes | Yes, with DNS discovery |
| Create concrete `EndpointSlice` addresses | No | Yes |
| Represent several discovered backend IPs as Kubernetes endpoints | No; resolved addresses remain DNS results | Yes |
| Refresh changing DNS addresses | Delegated to DNS clients and their caching behavior | Yes, periodically updates the `EndpointSlice` |
| Represent static external IPs | Not its purpose | Yes, with static discovery |
| Expose Kubernetes-native endpoint visibility | No generated endpoints | Yes |
| Require a cloud-specific SDK | No | No |
| Depend on an infrastructure provider | No | No |
| Load balance traffic itself | No | No |
| Actively health-check external workloads | No | No |

An `ExternalName` may ultimately resolve to several addresses, and clients may distribute connections among them. The architectural distinction is that Kubernetes does not materialize those results as `EndpointSlice` addresses. Choose based on whether DNS aliasing or endpoint materialization is the required abstraction.

## When ExternalName is the better choice

### Simple external dependency

If an application only needs to reach `database.example.internal` and does not need visibility into individual backend IPs, an `ExternalName` Service is direct and sufficient:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: legacy-db
spec:
  type: ExternalName
  externalName: database.example.internal
```

Installing and operating a controller would add unnecessary complexity in this case.

### Stable external SaaS or API

Workloads that only need a stable in-cluster alias for an external name such as `api.example.com` may also be well served by `ExternalName`. The application continues to rely on normal DNS resolution, which is often exactly the desired behavior.

If all that is required is:

```text
Service name -> external DNS name
```

use `ExternalName`.

## When DiscoveredService is useful

### Multiple external backends

Suppose `tomcat.internal.example.com` resolves to three VMs:

```text
10.140.0.11
10.140.0.12
10.140.0.13
```

The following resource turns those DNS results into three concrete endpoints:

```yaml
apiVersion: discovery.k8sready.com/v1alpha1
kind: DiscoveredService
metadata:
  name: tomcat
spec:
  discovery:
    dns:
      names:
      - tomcat.internal.example.com
  ports:
  - name: http
    port: 8080
```

Conceptually, the result is:

```text
Service/tomcat
        |
EndpointSlice/tomcat
        |
        +-- 10.140.0.11:8080
        +-- 10.140.0.12:8080
        +-- 10.140.0.13:8080
```

This is useful when a workload or Kubernetes networking component naturally consumes ordinary Services and EndpointSlices. Examples can include Traefik, ingress controllers, Gateway API implementations, other controllers, and applications using a normal Service. Exact support and behavior depend on the consumer and its version.

### External VM address changes

A stable name can outlive the VM behind it. For example, `tomcat-02.internal.example.com` may initially resolve to `10.140.0.12`, then resolve to `10.140.0.57` after the VM is recreated:

```text
Git / DiscoveredService
        |
        | unchanged
        v
tomcat-02.internal.example.com
        |
        +-- before: 10.140.0.12
        |
        +-- after:  10.140.0.57
        |
        v
EndpointSlice updated
```

The operator periodically resolves the name and replaces the old endpoint with the new address. The `DiscoveredService` configuration in Git remains unchanged. If resolution fails, the operator reports `Ready=False` with reason `DiscoveryFailed` and preserves the last successfully materialized endpoints rather than replacing them with an incomplete result.

### Legacy applications on VMs

An organization might run Traefik in Kubernetes while legacy Tomcat applications remain on VMs:

```text
Internet
   |
Traefik in Kubernetes
   |
Service/tomcat-erp
   |
EndpointSlice
   |
   +-- VM tomcat-01
   +-- VM tomcat-02
   +-- VM tomcat-03
```

DNS identifies the VMs, while the generated Service and EndpointSlice provide a Kubernetes-native backend representation. This can act as a migration bridge without requiring the external application to move immediately. Tomcat is only an example; the same model applies to other reachable external workloads.

### Hybrid and migration environments

During a gradual migration, applications may remain on cloud VMs, VMware, bare metal, OpenStack, or other on-premises infrastructure for months or years. Kubernetes consumers can reference those workloads through standard Service and EndpointSlice primitives while the underlying placement remains external.

No cloud integration is required. The cluster must be able to:

1. Resolve the configured DNS names.
2. Route traffic to the resulting addresses and ports.

### Provider-independent DNS discovery

The operator does not need to know whether `erp-backend.internal.company` belongs to GCP, AWS, Azure, VMware, bare metal, or another environment. Its contract is deliberately small:

```text
DNS -> IPv4 addresses -> EndpointSlice
```

This avoids cloud inventory APIs, credentials, labels, and provider-specific SDKs. It also means the operator cannot discover workloads by cloud tags or other inventory metadata.

### Static external targets

The static provider is the simplest integration: it represents known IPv4 addresses without DNS.

```yaml
apiVersion: discovery.k8sready.com/v1alpha1
kind: DiscoveredService
metadata:
  name: appliance
spec:
  discovery:
    static:
      addresses:
      - 10.20.0.10
      - 10.20.0.11
  ports:
  - name: https
    port: 443
```

Static discovery is useful for network appliances, legacy systems, temporary migrations, or systems without usable DNS. It does not handle IP drift: the resource must be updated when an address changes. Prefer DNS discovery when stable names exist and their addresses may change.

For either provider, `port` is the port exposed by the generated Service. Set `targetPort` when the external workload listens on a different port; if omitted, it logically defaults to `port`.

## Which should I use?

```text
Do you only need a Kubernetes name pointing to an external DNS name?
        |
       yes --> Use ExternalName
        |
       no
        v
Do you need concrete external addresses represented as Kubernetes endpoints?
        |
       yes
        v
Do addresses change behind stable DNS names?
        |
        +-- yes --> Use DiscoveredService DNS discovery
        |
        +-- no, only fixed IPs --> Use DiscoveredService static discovery
```

Use `ExternalName` when:

- DNS aliasing is enough.
- Individual backends do not need to appear as `EndpointSlice` endpoints.
- The simplest possible solution is preferred.

Use `DiscoveredService` when:

- External addresses need to become Kubernetes `EndpointSlice` endpoints.
- Consumers expect normal Kubernetes Service endpoint semantics.
- One DNS name may represent multiple backends.
- Endpoint IPs can change behind stable names.
- Hybrid VM and Kubernetes environments need a Kubernetes-native bridge.

## Network and DNS requirements

The operator creates endpoint resources; it does not create network connectivity. If DNS returns `10.140.0.11` but cluster nodes or pods cannot route to `10.140.0.11:8080`, the operator cannot make that backend reachable. Connectivity must already exist through mechanisms such as the same VPC, VPC peering, a VPN, private interconnect, or a routable on-premises network.

DNS discovery uses the DNS resolver available to the operator process. Internal names such as `tomcat.internal.example.com` must therefore resolve from the environment where the controller runs.

## When not to use it

Do not use this operator when:

- `ExternalName` already provides the required DNS alias.
- The external workload already has a suitable Kubernetes-native Service.
- Applications can use the external hostname directly.
- Active workload or application health checks are required.
- Service-mesh identity or integration is required.
- Traffic proxying is required.
- Cloud inventory discovery by labels or tags is required.
- Weighted routing or advanced load-balancing policy must be supplied by this operator.

Those responsibilities are intentionally outside this controller's scope.

## Current limitations

- Generated EndpointSlices use the IPv4 address type.
- DNS discovery materializes only IPv4 results, even if lookup also returns IPv6 addresses.
- DNS refresh is periodic and controller-wide; it is not based on individual DNS TTLs.
- There are no active application health checks.
- There is no cloud inventory discovery.
- There is no service-mesh functionality.
- There is no proxy or data plane.
- The operator creates no Ingress, ingress-controller-specific, or Gateway API resources.
