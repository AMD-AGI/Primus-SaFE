.PHONY: install uninstall check test example clean help

help:
	@echo "Primus Lens WandB Exporter - Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  install    - Install the package and wandb hook"
	@echo "  uninstall  - Uninstall the wandb hook"
	@echo "  check      - Check installation status"
	@echo "  test       - Run tests"
	@echo "  example    - Run example usage"
	@echo "  clean      - Clean build artifacts"
	@echo "  help       - Show this help message"

install:
	@echo "Installing Primus Lens WandB Exporter..."
	pip install -e .
	@echo ""
	@echo "Verifying installation..."
	python install_hook.py check

uninstall:
	@echo "Uninstalling wandb hook..."
	python install_hook.py uninstall

check:
	@echo "Checking installation..."
	python install_hook.py check

test:
	@echo "Running tests..."
	python test_wandb_hook.py

example:
	@echo "Running example..."
	python example_usage.py

clean:
	@echo "Cleaning build artifacts..."
	rm -rf build/
	rm -rf dist/
	rm -rf *.egg-info
	rm -rf src/*.egg-info
	find . -type d -name __pycache__ -exec rm -rf {} + 2>/dev/null || true
	find . -type f -name "*.pyc" -delete
	@echo "Done!"

