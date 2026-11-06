IMG ?= sample-app:dev
ENVTEST_K8S_VERSION ?= 1.37.0
SETUP_ENVTEST_VERSION ?= v0.25.2
LOCALBIN ?= $(CURDIR)/bin
# The image build fetches modules from the public Go proxy, not from the host's
# `go env GOPROXY`: the build VM may not reach a private proxy or trust its CA
# (x509: unknown authority).
IMAGE_GOPROXY ?= https://proxy.golang.org,direct

# go run builds setup-envtest for the host, so the Mac and a Linux container machine can
# share this working directory; the API server binaries land in bin/k8s/<ver>-<os>-<arch>.
SETUP_ENVTEST = go run sigs.k8s.io/controller-runtime/tools/setup-envtest@$(SETUP_ENVTEST_VERSION)

.PHONY: test
test: envtest ## Unit tests and envtest; PG_URL adds the Postgres integration test.
	@KUBEBUILDER_ASSETS="$$($(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test ./... -count=1

.PHONY: envtest
envtest: ## Download the envtest API server and etcd for this OS.
	@$(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path >/dev/null

.PHONY: build
build: ## Build bin/sample-app.
	@go build -o $(LOCALBIN)/sample-app ./cmd/sample-app

.PHONY: image
image: ## Build the image with the container CLI; the cluster's nodes see it (--shared-image-store).
	@container build -t $(IMG) --build-arg GOPROXY=$(IMAGE_GOPROXY) .

.PHONY: help
help:
	@awk 'BEGIN {FS = ":.*##"} /^[a-z-]+:.*##/ { printf "  %-10s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
