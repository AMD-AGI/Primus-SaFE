# Makefile for Primus Lens Helm Chart
# Provides convenient commands for chart development and deployment

.PHONY: help
help: ## Display this help message
	@echo "Primus Lens Helm Chart - Makefile Commands"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: deps
deps: ## Download and update chart dependencies
	@echo "📦 Downloading chart dependencies..."
	helm dependency update

.PHONY: lint
lint: ## Lint the chart for potential issues
	@echo "🔍 Linting chart..."
	helm lint .

.PHONY: template
template: ## Render chart templates to stdout
	@echo "📋 Rendering chart templates..."
	helm template primus-lens . -f values.yaml

.PHONY: template-debug
template-debug: ## Render chart templates with debug info
	@echo "🐛 Rendering chart templates (debug mode)..."
	helm template primus-lens . -f values.yaml --debug

.PHONY: dry-run
dry-run: ## Simulate an install (dry-run)
	@echo "🏃 Running dry-run installation..."
	helm install primus-lens . -f values.yaml --dry-run --debug --namespace primus-lens

.PHONY: install
install: deps ## Install the chart with default values
	@echo "🚀 Installing Primus Lens..."
	helm install primus-lens . \
		--namespace primus-lens \
		--create-namespace \
		--timeout 30m \
		--wait

.PHONY: install-dev
install-dev: deps ## Install the chart with dev values
	@echo "🔧 Installing Primus Lens (dev environment)..."
	helm install primus-lens-dev . \
		-f values-dev.yaml \
		--namespace primus-lens-dev \
		--create-namespace \
		--timeout 30m \
		--wait

.PHONY: install-prod
install-prod: deps ## Install the chart with prod values
	@echo "🏭 Installing Primus Lens (production environment)..."
	@if [ -z "$(GRAFANA_PASSWORD)" ]; then \
		echo "❌ Error: GRAFANA_PASSWORD environment variable is not set"; \
		echo "   Usage: make install-prod GRAFANA_PASSWORD=your-secure-password"; \
		exit 1; \
	fi
	helm install primus-lens . \
		-f values-prod.yaml \
		--set grafana.adminPassword=$(GRAFANA_PASSWORD) \
		--namespace primus-lens \
		--create-namespace \
		--timeout 30m \
		--wait

.PHONY: upgrade
upgrade: deps ## Upgrade the existing release
	@echo "⬆️  Upgrading Primus Lens..."
	helm upgrade primus-lens . \
		-f values.yaml \
		--namespace primus-lens \
		--timeout 30m \
		--wait

.PHONY: uninstall
uninstall: ## Uninstall the release
	@echo "🗑️  Uninstalling Primus Lens..."
	helm uninstall primus-lens --namespace primus-lens

.PHONY: uninstall-dev
uninstall-dev: ## Uninstall the dev release
	@echo "🗑️  Uninstalling Primus Lens (dev)..."
	helm uninstall primus-lens-dev --namespace primus-lens-dev

.PHONY: status
status: ## Show release status
	@echo "📊 Primus Lens Status:"
	@helm status primus-lens -n primus-lens

.PHONY: history
history: ## Show release history
	@echo "📜 Primus Lens Release History:"
	@helm history primus-lens -n primus-lens

.PHONY: rollback
rollback: ## Rollback to previous release
	@echo "⏪ Rolling back Primus Lens..."
	helm rollback primus-lens --namespace primus-lens

.PHONY: get-pods
get-pods: ## Get all pods in the namespace
	@echo "🔍 Primus Lens Pods:"
	@kubectl get pods -n primus-lens -o wide

.PHONY: get-jobs
get-jobs: ## Get all jobs in the namespace
	@echo "💼 Primus Lens Jobs:"
	@kubectl get jobs -n primus-lens

.PHONY: get-pvc
get-pvc: ## Get all PVCs in the namespace
	@echo "💾 Primus Lens PVCs:"
	@kubectl get pvc -n primus-lens

.PHONY: logs-init
logs-init: ## Show logs from initialization jobs
	@echo "📝 Wait Operators Job Logs:"
	@kubectl logs -n primus-lens job/primus-lens-wait-operators --tail=50 || echo "Job not found"
	@echo ""
	@echo "📝 PostgreSQL Init Job Logs:"
	@kubectl logs -n primus-lens job/primus-lens-postgres-init --tail=50 || echo "Job not found"

.PHONY: logs-api
logs-api: ## Show logs from API pods
	@echo "📝 API Service Logs:"
	@kubectl logs -n primus-lens -l app=primus-lens-api --tail=100 --prefix

.PHONY: port-forward-web
port-forward-web: ## Port forward to web console
	@echo "🌐 Port forwarding to Web Console..."
	@echo "   Access at: http://localhost:30180"
	@kubectl port-forward -n primus-lens svc/primus-lens-web 30180:80

.PHONY: port-forward-grafana
port-forward-grafana: ## Port forward to Grafana
	@echo "📊 Port forwarding to Grafana..."
	@echo "   Access at: http://localhost:30182/grafana"
	@echo "   Default credentials: admin / admin"
	@kubectl port-forward -n primus-lens svc/grafana-service 30182:3000

.PHONY: wait-ready
wait-ready: ## Wait for all pods to be ready
	@echo "⏳ Waiting for all pods to be ready..."
	@kubectl wait --for=condition=ready pod \
		--all \
		-n primus-lens \
		--timeout=600s

.PHONY: describe-postgres
describe-postgres: ## Describe PostgreSQL cluster
	@echo "🐘 PostgreSQL Cluster:"
	@kubectl describe postgrescluster primus-lens -n primus-lens

.PHONY: describe-opensearch
describe-opensearch: ## Describe OpenSearch cluster
	@echo "🔍 OpenSearch Cluster:"
	@kubectl describe opensearchcluster primus-lens-logs -n primus-lens

.PHONY: describe-vmcluster
describe-vmcluster: ## Describe VictoriaMetrics cluster
	@echo "📊 VictoriaMetrics Cluster:"
	@kubectl describe vmcluster primus-lens-vmcluster -n primus-lens

.PHONY: clean-namespace
clean-namespace: ## Delete the namespace (WARNING: deletes all data!)
	@echo "⚠️  WARNING: This will delete the namespace and ALL data!"
	@echo "   Press Ctrl+C to cancel, or wait 5 seconds to continue..."
	@sleep 5
	@kubectl delete namespace primus-lens

.PHONY: package
package: deps ## Package the chart into a versioned archive
	@echo "📦 Packaging chart..."
	helm package .

.PHONY: docs
docs: ## Generate documentation
	@echo "📚 Generating documentation..."
	helm-docs || echo "helm-docs not installed, skipping..."

.PHONY: test
test: lint template dry-run ## Run all tests (lint, template, dry-run)
	@echo "✅ All tests passed!"

.PHONY: verify
verify: ## Verify the installation
	@echo "✅ Verifying Primus Lens installation..."
	@echo ""
	@echo "📊 Release Status:"
	@helm status primus-lens -n primus-lens
	@echo ""
	@echo "🔍 Pods:"
	@kubectl get pods -n primus-lens
	@echo ""
	@echo "💼 Jobs:"
	@kubectl get jobs -n primus-lens
	@echo ""
	@echo "📈 Custom Resources:"
	@kubectl get postgrescluster,opensearchcluster,vmcluster -n primus-lens

# Development helpers
.PHONY: watch-pods
watch-pods: ## Watch pods status (requires watch command)
	@watch -n 2 kubectl get pods -n primus-lens

.PHONY: shell-api
shell-api: ## Open shell in API pod
	@kubectl exec -it -n primus-lens $$(kubectl get pod -n primus-lens -l app=primus-lens-api -o jsonpath='{.items[0].metadata.name}') -- /bin/bash

.PHONY: shell-db
shell-db: ## Open PostgreSQL shell
	@kubectl exec -it -n primus-lens $$(kubectl get pod -n primus-lens -l postgres-operator.crunchydata.com/cluster=primus-lens,postgres-operator.crunchydata.com/role=master -o jsonpath='{.items[0].metadata.name}') -- psql -U postgres

.DEFAULT_GOAL := help

