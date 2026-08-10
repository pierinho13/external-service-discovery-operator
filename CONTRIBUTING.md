# Contributing

Contributions to External Service Discovery Operator are welcome. Keep changes focused on Kubernetes-native projection of external workloads into selectorless Services and EndpointSlices; the operator is not a proxy, ingress controller, or service mesh.

## Development

Requirements are Go, Docker, kubectl, Helm, and the tools installed by the Makefile. Fork the repository, branch from `main`, and use Conventional Commit messages.

Before opening a pull request, run:

```bash
make generate manifests
make fmt
make vet
make test
make lint
make chart-test
make release-check
git diff --check
```

Generated API deep-copies, CRDs, and RBAC must be committed. Add unit tests for provider logic and envtest coverage for API-server behavior. Tests must not depend on public DNS or cloud credentials.

Never commit credentials, kubeconfig data, signing keys, DNS infrastructure details, or sensitive endpoint addresses. Report vulnerabilities according to [SECURITY.md](SECURITY.md).
