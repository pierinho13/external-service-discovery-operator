SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec
LOCALBIN ?= $(shell pwd)/bin
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_TOOLS_VERSION ?= v0.20.0
ENVTEST_VERSION ?= release-0.24
ENVTEST_K8S_VERSION ?= 1.36
KUSTOMIZE_VERSION ?= v5.6.0
GOLANGCI_LINT_VERSION ?= v2.11.4
GORELEASER_VERSION ?= v2.12.7
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint
GORELEASER ?= $(LOCALBIN)/goreleaser
GOLANGCI_LINT_CACHE ?= $(LOCALBIN)/.golangci-cache
IMG ?= controller:latest

.PHONY: all manifests generate fmt vet test build run install deploy lint chart-test release-check
all: build
manifests: controller-gen
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd paths="./..." output:crd:artifacts:config=config/crd/bases
	./hack/sync-helm-rbac.sh
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."
fmt:
	go fmt ./...
vet:
	go vet ./...
test: manifests generate fmt vet setup-envtest
	KUBEBUILDER_ASSETS="$$($(ENVTEST) use -i $(ENVTEST_K8S_VERSION) -p path)" go test ./... -coverprofile cover.out
build: manifests generate fmt vet
	go build -o bin/manager ./cmd/main.go
run: manifests generate fmt vet
	go run ./cmd/main.go
install: manifests kustomize
	$(KUSTOMIZE) build config/crd | kubectl apply -f -
deploy: manifests kustomize
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/default | kubectl apply -f -
lint: golangci-lint
	GOLANGCI_LINT_CACHE=$(GOLANGCI_LINT_CACHE) $(GOLANGCI_LINT) run --timeout=5m
chart-test: manifests
	./hack/package-release-helm-chart.sh 0.0.0-snapshot v0.0.0-snapshot
	helm lint dist/helm/external-service-discovery-operator-0.0.0-snapshot.tgz
	rendered="$$(mktemp)"; trap 'rm -f "$${rendered}"' EXIT; \
		helm template external-service-discovery-operator dist/helm/external-service-discovery-operator-0.0.0-snapshot.tgz --namespace external-service-discovery-operator-system >"$${rendered}"; \
		go run ./hack/verify-rbac config/rbac/role.yaml config/rbac/leader_election_role.yaml "$${rendered}"
release-check: chart-test goreleaser
	$(GORELEASER) check
	$(GORELEASER) release --snapshot --clean

$(LOCALBIN):
	mkdir -p $(LOCALBIN)
controller-gen: $(CONTROLLER_GEN)
$(CONTROLLER_GEN): | $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)
setup-envtest: $(ENVTEST)
	$(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path
$(ENVTEST): | $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(ENVTEST_VERSION)
kustomize: $(KUSTOMIZE)
$(KUSTOMIZE): | $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/kustomize/kustomize/v5@$(KUSTOMIZE_VERSION)
golangci-lint: $(GOLANGCI_LINT)
$(GOLANGCI_LINT): | $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
goreleaser: $(GORELEASER)
$(GORELEASER): | $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION)
