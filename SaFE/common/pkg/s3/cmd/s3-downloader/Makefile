.PHONY: build clean docker-build docker-push docker-images docker-verify deps help

BINARY_NAME=s3-downloader
DOCKER_IMAGE=primussafe/s3-downloader
# Use := to evaluate once at the beginning of Makefile execution
DOCKER_TAG:=$(shell date +%Y%m%d%H%M)

# Build the binary
build:
	go build -o $(BINARY_NAME) .

# Build with version info
build-release:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BINARY_NAME) .

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	go clean

# Build Docker image (builds from SaFE/ root directory to include all modules)
# Tags with timestamp (e.g., 202501241430) and latest
docker-build:
	@echo "Building Docker image: $(DOCKER_IMAGE):$(DOCKER_TAG)"
	@echo "Build context: ../../../../.."
	docker build -f ./Dockerfile -t $(DOCKER_IMAGE):$(DOCKER_TAG) -t $(DOCKER_IMAGE):latest ../../../../.. || (echo "Docker build failed!" && exit 1)
	@echo ""
	@echo "✓ Successfully built: $(DOCKER_IMAGE):$(DOCKER_TAG)"
	@echo "✓                    $(DOCKER_IMAGE):latest"
	@echo ""
	@docker images | grep "$(DOCKER_IMAGE)" | grep -E "$(DOCKER_TAG)|latest" || echo "Warning: Images not found in docker images list"

# Push Docker image (both timestamp and latest tags)
docker-push: docker-build
	@echo ""
	@echo "Verifying images before push..."
	@docker images $(DOCKER_IMAGE):$(DOCKER_TAG) --format "{{.Repository}}:{{.Tag}}" | grep -q "$(DOCKER_TAG)" || (echo "Error: Image $(DOCKER_IMAGE):$(DOCKER_TAG) not found!" && exit 1)
	@docker images $(DOCKER_IMAGE):latest --format "{{.Repository}}:{{.Tag}}" | grep -q "latest" || (echo "Error: Image $(DOCKER_IMAGE):latest not found!" && exit 1)
	@echo "✓ Images verified"
	@echo ""
	@echo "Pushing to Docker Hub (primussafe)..."
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG) || (echo "Failed to push $(DOCKER_TAG)" && exit 1)
	docker push $(DOCKER_IMAGE):latest || (echo "Failed to push latest" && exit 1)
	@echo ""
	@echo "✓ Successfully pushed: $(DOCKER_IMAGE):$(DOCKER_TAG)"
	@echo "✓                      $(DOCKER_IMAGE):latest"

# Install dependencies
deps:
	go mod download
	go mod tidy

# List local Docker images for this project
docker-images:
	@echo "Local Docker images for s3-downloader:"
	@docker images primussafe/s3-downloader

# Verify Docker build without pushing
docker-verify: docker-build
	@echo ""
	@echo "Verifying built images..."
	@docker images primussafe/s3-downloader
	@echo ""
	@echo "Image details:"
	@docker inspect $(DOCKER_IMAGE):$(DOCKER_TAG) --format='Image ID: {{.Id}}' 2>/dev/null || echo "Error: $(DOCKER_TAG) not found"
	@docker inspect $(DOCKER_IMAGE):latest --format='Image ID: {{.Id}}' 2>/dev/null || echo "Error: latest not found"

# Show help
help:
	@echo "S3 Downloader - Build and Push Tool"
	@echo ""
	@echo "Available targets:"
	@echo "  build           - Build the Go binary locally"
	@echo "  build-release   - Build optimized binary for release"
	@echo "  clean           - Remove build artifacts"
	@echo "  docker-build    - Build Docker image (timestamp + latest tags)"
	@echo "  docker-verify   - Build and verify images without pushing"
	@echo "  docker-images   - List local Docker images"
	@echo "  docker-push     - Build and push to Docker Hub (requires login)"
	@echo "  deps            - Download and tidy dependencies"
	@echo "  help            - Show this help message"
	@echo ""
	@echo "Docker Configuration:"
	@echo "  Image: $(DOCKER_IMAGE)"
	@echo "  Tags:  $(DOCKER_TAG), latest"
	@echo ""
	@echo "Workflow:"
	@echo "  1. docker login                    # Login to Docker Hub"
	@echo "  2. make docker-verify              # Build and verify (optional)"
	@echo "  3. make docker-push                # Build and push"

