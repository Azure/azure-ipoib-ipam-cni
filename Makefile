# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

ARCH ?= amd64


.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: goimports ## Run go fmt against code.
	${GOIMPORTS} -local github.com/Azure/azure-ipoib-ipam-cni -w .

.PHONY: vet
vet: golangci-lint ## Run go vet against code.
	$(LOCALBIN)/golangci-lint run --timeout 10m ./...
##@ Build

.PHONY: build
build: $(LOCALBIN) fmt vet ## Build manager binary.
	CGO_ENABLED=0 GOOS=linux GOARCH=${ARCH} go build -o bin/azure-ipoib-ipam-cni ./cmd/
##@ Build Dependencies

.PHONY: install-dependencies
install-dependencies: cobra-cli golangci-lint goimports ## Install all build dependencies.
	export PATH=$$PATH:$(LOCALBIN)

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
.PHONY: $(LOCALBIN)
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

GOIMPORTS ?= $(LOCALBIN)/goimports
.PHONY: goimports
goimports: $(GOIMPORTS) ## Download goimports locally if necessary.
$(GOIMPORTS): $(LOCALBIN)
	test -s $(LOCALBIN)/goimports || GOBIN=$(LOCALBIN) go install golang.org/x/tools/cmd/goimports@latest

COBRA_CLI ?=$(LOCALBIN)/cobra-CLI
.PHONY: cobra-cli 
cobra-cli: $(COBRA_CLI) ## Download cobra cli locally if necessary.
${COBRA_CLI}: $(LOCALBIN)
	test -s $(LOCALBIN)/cobra-cli || GOBIN=$(LOCALBIN) go install github.com/spf13/cobra-cli@latest

GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint
.PHONY: golangci-lint 
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	test -s $(LOCALBIN)/golangci-lint || curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(LOCALBIN) latest