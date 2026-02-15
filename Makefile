TAILWIND = npx tailwindcss -i ./static/input.css -o ./static/output.css
VERSION ?= 0.0.0
RELEASE_TYPE ?= patch

.PHONY: help build clean build-docker run tailwind tailwind-watch lint staticcheck golangci-lint errcheck release changelog-unreleased test-integration test-integration-up test-integration-down test-e2e-generate-certs test-e2e-swift test-e2e-full

# Default target when no arguments are given to make
help:
	@echo "OpenSPM Registry Makefile Help"
	@echo "=============================="
	@echo ""
	@echo "Available targets:"
	@echo "  help          - Show this help message"
	@echo "  build         - Build the project (includes lint, tests and dependencies)"
	@echo "  lint         - Run staticcheck, golangci-lint and errcheck"
	@echo "  staticcheck   - Run staticcheck linter"
	@echo "  golangci-lint - Run golangci-lint"
	@echo "  errcheck     - Run errcheck with -blank (includes unchecked defer Close)"
	@echo "  clean         - Clean Go build cache"
	@echo "  build-docker  - Build Docker image"
	@echo "  run           - Run the server locally"
	@echo "  tailwind      - Build Tailwind CSS"
	@echo "  tailwind-watch- Watch and rebuild Tailwind CSS on changes"
	@echo "  release       - Create a new release (Usage: make release VERSION=1.2.3)"
	@echo "  changelog-unreleased - Show unreleased changes"
	@echo "  test-integration - Run integration tests (requires Docker)"
	@echo "  test-integration-up - Start Nexus test server"
	@echo "  test-integration-down - Stop Nexus test server"
	@echo "  test-e2e-generate-certs - Generate E2E HTTPS certs (for optional HTTPS testing)"
	@echo "  test-e2e-swift - E2E: Swift publish + resolve (Nexus must be up; requires Swift)"
	@echo "  test-e2e-full - Start Nexus, run E2E Swift test, then stop Nexus"
	@echo ""
	@echo "Example usage:"
	@echo "  make build              - Build the project"
	@echo "  make run               - Run the server locally"
	@echo "  make release VERSION=1.2.3 - Create release version 1.2.3"

build: tailwind lint
	go mod tidy && go mod verify && go mod download && go test ./... && go build -o openspmregistry main.go

lint: staticcheck golangci-lint errcheck

staticcheck:
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...

golangci-lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run ./...

# Catches unchecked errors including defer x.Close() (no -blank: explicit _ discards not reported).
errcheck:
	go run github.com/kisielk/errcheck@latest ./...

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
	@echo "Starting Nexus test server..."
	docker-compose -f docker-compose.test.yml up -d
	@echo "Waiting for Nexus to be ready (this may take 2-3 minutes)..."
	@timeout=180; \
	while [ $$timeout -gt 0 ]; do \
		ready=0; \
		if command -v curl > /dev/null 2>&1; then \
			if curl -sf http://localhost:8081/service/rest/v1/status > /dev/null 2>&1; then \
				ready=1; \
			fi; \
		fi; \
		if [ $$ready -eq 1 ]; then \
			echo "Nexus is ready!"; \
			break; \
		fi; \
		sleep 5; \
		timeout=$$((timeout - 5)); \
	done; \
	if [ $$timeout -le 0 ]; then \
		echo "Warning: Nexus may not be fully ready yet. Check with: docker ps"; \
		exit 1; \
	fi
	@echo "Bootstrapping Nexus (create repo, set admin password)..."
	@NEXUS_TEST_PASSWORD_FILE=.nexus-test-password bash scripts/nexus-bootstrap.sh

test-integration-down:
	@echo "Stopping Nexus test server..."
	docker-compose -f docker-compose.test.yml down

test-integration: test-integration-up
	@echo "Running integration tests..."
	@passfile=".nexus-test-password"; \
	if [ -f "$$passfile" ]; then \
	  MAVEN_REPO_PASSWORD=$$(cat "$$passfile"); \
	else \
	  MAVEN_REPO_PASSWORD=admin123; \
	fi; \
	INTEGRATION_TESTS=1 MAVEN_REPO_URL=http://localhost:8081/repository MAVEN_REPO_NAME=private MAVEN_REPO_USERNAME=admin MAVEN_REPO_PASSWORD="$$MAVEN_REPO_PASSWORD" go test -tags=integration -v ./repo/maven/... -run TestIntegration
	@$(MAKE) test-integration-down

# Generate E2E certs for optional HTTPS testing (testdata/e2e/certs/).
test-e2e-generate-certs:
	@bash scripts/e2e-generate-certs.sh

# E2E Swift: publish package to OpenSPMRegistry (Maven-backed) and resolve from consumer.
# test-e2e-swift: run E2E script only (start Nexus first with make test-integration-up).
test-e2e-swift:
	@bash scripts/e2e-swift-publish-resolve.sh

# test-e2e-full: start Nexus, bootstrap, run E2E Swift test, then tear down.
test-e2e-full: test-integration-up
	@$(MAKE) test-e2e-swift
	@$(MAKE) test-integration-down