.PHONY: build push deploy clean generate

IMAGE_REPO ?= harbor.oci-slc.primus-safe.amd.com/primus-lens
IMAGE_TAG ?= latest
IMAGE = $(IMAGE_REPO)/rccl-socket-tracer:$(IMAGE_TAG)

KUBECONFIG ?= ~/workspace/k-oci-slc

# Generate vmlinux.h (run on a machine with matching kernel)
bpf/vmlinux.h:
	bpftool btf dump file /sys/kernel/btf/vmlinux format c > bpf/vmlinux.h

# Build container image
build:
	docker build -t $(IMAGE) .

# Push to registry
push: build
	docker push $(IMAGE)

# Deploy DaemonSet
deploy:
	KUBECONFIG=$(KUBECONFIG) kubectl apply -f deploy/daemonset.yaml

# Undeploy
undeploy:
	KUBECONFIG=$(KUBECONFIG) kubectl delete -f deploy/daemonset.yaml --ignore-not-found

# Check logs on a specific node
# Usage: make logs NODE=uswslocpm2m-106-2111
logs:
	@echo "=== Trace files on $(NODE) ==="
	KUBECONFIG=$(KUBECONFIG) kubectl exec -n primus-lens \
		$$(KUBECONFIG=$(KUBECONFIG) kubectl get pods -n primus-lens -l app=rccl-socket-tracer \
			--field-selector spec.nodeName=$(NODE) -o name | head -1) \
		-- find /var/log/rccl-tracer -name "*.log" -ls

# Collect trace files from a node
# Usage: make collect NODE=uswslocpm2m-106-2111 DEST=./traces
collect:
	@mkdir -p $(DEST)/$(NODE)
	KUBECONFIG=$(KUBECONFIG) kubectl cp -n primus-lens \
		$$(KUBECONFIG=$(KUBECONFIG) kubectl get pods -n primus-lens -l app=rccl-socket-tracer \
			--field-selector spec.nodeName=$(NODE) -o jsonpath='{.items[0].metadata.name}'):/var/log/rccl-tracer \
		$(DEST)/$(NODE)/

clean:
	rm -f bpf/vmlinux.h bpf/*.o cmd/tracer_bpf*.go
