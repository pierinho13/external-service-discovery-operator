# Security policy

## Reporting a vulnerability

Do not open a public issue. Use GitHub private vulnerability reporting at:

https://github.com/pierinho13/external-service-discovery-operator/security/advisories/new

Include the affected version, impact, reproduction steps, and any suggested mitigation without including production credentials or private infrastructure data.

## Supported versions

Security fixes are provided for the latest released minor version. This project currently exposes a `v1alpha1` API; review release notes before upgrading.

## Security model

The controller runs as a non-root user with a read-only root filesystem in the Helm chart. RBAC is limited to `DiscoveredService`, Service, EndpointSlice, and leader-election resources. It has no data plane and does not proxy workload traffic.

DNS answers and static addresses are control-plane inputs. Restrict who may create `DiscoveredService` resources and ensure cluster DNS is trusted. Do not place credentials or other secrets in custom resources.
