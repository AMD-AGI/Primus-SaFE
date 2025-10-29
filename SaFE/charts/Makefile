#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

## Location to install dependencies to
LOCALBIN:= $(shell go env GOBIN)
LOCALBIN := $(if $(LOCALBIN),$(LOCALBIN),$(shell pwd)/bin)

CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
CONTROLLER_TOOLS_VERSION ?= v0.17.2

.PHONY: controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	@mkdir -p $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.DEFAULT_GOAL := all
.PHONY: all manifests
all: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	@echo "Generating manifests..."
	mkdir -p primus-safe/crds
	$(CONTROLLER_GEN) rbac:roleName=primus-safe crd webhook paths="../apis/pkg/apis/..." \
        output:crd:artifacts:config=primus-safe/crds \
		output:webhook:artifacts:config=primus-safe/templates/webhooks \
		output:rbac:artifacts:config=primus-safe/templates/rbac
	sed -i 's/\r//' hack/webhooks.sh
	bash hack/webhooks.sh
	bash hack/role.sh
	bash hack/add-copyright.sh

clean:
	rm -rf primus-safe/crds
	rm -f primus-safe/templates/webhooks/manifests.yaml
	rm -f primus-safe/templates/rbac/role.yaml

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo $(LOCALBIN) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef
