## Location to install dependencies to
LOCALBIN:= $(shell go env GOBIN)
LOCALBIN := $(if $(LOCALBIN),$(LOCALBIN),$(shell pwd)/bin)
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
CONTROLLER_TOOLS_VERSION ?= v0.17.2

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	mkdir -p primus-safe/crds
	$(CONTROLLER_GEN) rbac:roleName=primus-safe-manager crd webhook paths="../apis/pkg/apis/..." output:crd:artifacts:config=primus-safe/crds \
	output:webhook:artifacts:config=primus-safe/templates/webhooks \
	output:rbac:artifacts:config=primus-safe/templates/rbac
	sed -i 's/\r//' hack/webhooks.sh
	bash hack/webhooks.sh

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