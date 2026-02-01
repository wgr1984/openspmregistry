TAILWIND = npx tailwindcss -i ./static/input.css -o ./static/output.css
VERSION ?= 0.0.0
RELEASE_TYPE ?= patch

.PHONY: help build clean build-docker run tailwind tailwind-watch staticcheck release changelog-unreleased test-integration test-integration-up test-integration-down

# Default target when no arguments are given to make
help:
	@echo "OpenSPM Registry Makefile Help"
	@echo "=============================="
	@echo ""
	@echo "Available targets:"
	@echo "  help          - Show this help message"
	@echo "  build         - Build the project (includes staticcheck, tests and dependencies)"
	@echo "  staticcheck   - Run staticcheck linter"
	@echo "  clean         - Clean Go build cache"
	@echo "  build-docker  - Build Docker image"
	@echo "  run           - Run the server locally"
	@echo "  tailwind      - Build Tailwind CSS"
	@echo "  tailwind-watch- Watch and rebuild Tailwind CSS on changes"
	@echo "  release       - Create a new release (Usage: make release VERSION=1.2.3)"
	@echo "  changelog-unreleased - Show unreleased changes"
	@echo "  test-integration - Run integration tests (requires Docker)"
	@echo "  test-integration-up - Start Reposilite test server"
	@echo "  test-integration-down - Stop Reposilite test server"
	@echo ""
	@echo "Example usage:"
	@echo "  make build              - Build the project"
	@echo "  make run               - Run the server locally"
	@echo "  make release VERSION=1.2.3 - Create release version 1.2.3"

build: tailwind staticcheck
	go mod tidy && go mod verify && go mod download && go test ./... && go build -o openspmregistry main.go

staticcheck:
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...

clean:
	go clean -cache

build-docker: tailwind
	docker build -t wgr1984/openspmregistry .

run: tailwind
	go run main.go -v

tailwind:
	$(TAILWIND)

tailwind-watch:
	$(TAILWIND) --watch

release:
	@if [ "$(VERSION)" = "0.0.0" ]; then \
		echo "Please specify a version number: make release VERSION=1.2.3"; \
		exit 1; \
	fi
	@echo "Creating release v$(VERSION)"
	@if [ ! -f CHANGELOG.md ]; then \
		echo "CHANGELOG.md not found!"; \
		exit 1; \
	fi
	@# Update CHANGELOG.md
	@DATE=$$(date +%Y-%m-%d); \
	NEW_VERSION="\n## [$(VERSION)] - $$DATE"; \
	awk -v ver="$$NEW_VERSION" '/## \[Latest\]/ { print; print ver; next } { print }' CHANGELOG.md > CHANGELOG.md.tmp
	@echo "\nReview the latest changes to CHANGELOG.md:"
	@echo "=================================="
	@cat CHANGELOG.md.tmp
	@echo "\n=================================="
	@read -p "Do you want to proceed with these changes? (y/N) " confirm; \
	if [ "$$confirm" != "y" ] && [ "$$confirm" != "Y" ]; then \
		rm CHANGELOG.md.tmp; \
		echo "Release cancelled."; \
		exit 1; \
	fi
	@mv CHANGELOG.md.tmp CHANGELOG.md
	@# Commit CHANGELOG.md
	git add CHANGELOG.md
	git commit -m "chore: update CHANGELOG for v$(VERSION)"
	@# Create and push tag
	git tag -a v$(VERSION) -m "Release v$(VERSION)"
	git push origin main v$(VERSION)
	@echo "Release v$(VERSION) created and pushed. GitHub Actions will handle the Docker build and publish."

changelog-unreleased:
	@echo "Latest changes:"
	@awk '/## \[Latest\]/{p=1;print;next} /## \[[0-9]+\.[0-9]+\.[0-9]+\]/{p=0}p' CHANGELOG.md

# Integration test targets
test-integration-up:
	@echo "Starting Reposilite test server..."
	docker-compose -f docker-compose.test.yml up -d
	@echo "Waiting for Reposilite to be ready (this may take up to 30 seconds)..."
	@timeout=60; \
	while [ $$timeout -gt 0 ]; do \
		ready=0; \
		if command -v curl > /dev/null 2>&1; then \
			if curl -f http://localhost:8080/ > /dev/null 2>&1; then \
				ready=1; \
			fi; \
		fi; \
		if [ $$ready -eq 0 ]; then \
			if docker inspect reposilite-test 2>/dev/null | grep -q '"Status": "running"'; then \
				ready=1; \
			fi; \
		fi; \
		if [ $$ready -eq 1 ]; then \
			echo "Reposilite is ready!"; \
			break; \
		fi; \
		sleep 2; \
		timeout=$$((timeout - 2)); \
	done; \
	if [ $$timeout -le 0 ]; then \
		echo "Warning: Reposilite may not be fully ready yet. Check with: docker ps"; \
	fi

test-integration-down:
	@echo "Stopping Reposilite test server..."
	docker-compose -f docker-compose.test.yml down

test-integration: test-integration-up
	@echo "Running integration tests..."
	@INTEGRATION_TESTS=1 MAVEN_REPO_URL=http://localhost:8080 go test -tags=integration -v ./repo/maven/... -run TestIntegration
	@$(MAKE) test-integration-down