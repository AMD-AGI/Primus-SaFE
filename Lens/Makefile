# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.

LICENSE_YEAR := 2025-2026
LICENSE_HOLDER := Advanced Micro Devices, Inc. All rights reserved.
GIT_ROOT := $(shell git rev-parse --show-toplevel)

# Check if addlicense is installed
ADDLICENSE := $(shell which addlicense 2>/dev/null)

.PHONY: help license license-check license-install install-hooks

help:
	@echo "Available targets:"
	@echo "  license         - Add license headers to all Go and Python files"
	@echo "  license-check   - Check if all files have license headers (dry-run)"
	@echo "  license-install - Install addlicense tool"
	@echo "  install-hooks   - Install git pre-commit hook for auto license headers"

license-install:
	@echo "Installing addlicense..."
	go install github.com/google/addlicense@latest

install-hooks: license-install
	@echo "Installing pre-commit hook..."
	@cp scripts/pre-commit $(GIT_ROOT)/.git/hooks/pre-commit
	@chmod +x $(GIT_ROOT)/.git/hooks/pre-commit
	@echo "Pre-commit hook installed successfully!"
	@echo "License headers will be automatically added when you commit Go or Python files."

license: check-addlicense
	@echo "Adding license headers to Go files..."
	@addlicense -f license-go.tpl -y "$(LICENSE_YEAR)" -c "$(LICENSE_HOLDER)" \
		$$(find . -name "*.go" -type f -not -path "*/.venv/*" -not -path "*/vendor/*")
	@echo "Adding license headers to Python files..."
	@addlicense -f license-py.tpl -y "$(LICENSE_YEAR)" -c "$(LICENSE_HOLDER)" \
		$$(find . -name "*.py" -type f -not -path "*/.venv/*" -not -path "*/site-packages/*")
	@echo "Done!"

license-check: check-addlicense
	@echo "Checking license headers in Go files..."
	@addlicense -check -f license-go.tpl -y "$(LICENSE_YEAR)" -c "$(LICENSE_HOLDER)" \
		$$(find . -name "*.go" -type f -not -path "*/.venv/*" -not -path "*/vendor/*") || (echo "Some Go files are missing license headers" && exit 1)
	@echo "Checking license headers in Python files..."
	@addlicense -check -f license-py.tpl -y "$(LICENSE_YEAR)" -c "$(LICENSE_HOLDER)" \
		$$(find . -name "*.py" -type f -not -path "*/.venv/*" -not -path "*/site-packages/*") || (echo "Some Python files are missing license headers" && exit 1)
	@echo "All files have license headers!"

check-addlicense:
ifndef ADDLICENSE
	$(error addlicense is not installed. Run 'make license-install' first)
endif
