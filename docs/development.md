# Development

Install Go and run `make help` to inspect available commands. `make test` installs controller-runtime envtest assets and runs unit plus real API-server tests.

```bash
make generate manifests
make fmt vet test build
make lint
make chart-test
```

Provider implementations belong under `internal/discovery`; controllers must remain provider-agnostic. Generated files must be committed. Optional pre-commit hooks are installed with `pre-commit install --hook-type pre-commit --hook-type pre-push`.
