TAILWIND = npx tailwindcss -i ./static/input.css -o ./static/output.css
VERSION ?= 0.0.0
RELEASE_TYPE ?= patch

# E2E with HTTPS: set E2E_HTTPS=1 or use test-e2e-*-https targets. Requires certs (make test-e2e-generate-certs).
ifdef E2E_HTTPS
E2E_REGISTRY_URL := https://127.0.0.1:8082
endif

.PHONY: help build clean build-docker run tailwind tailwind-watch lint staticcheck golangci-lint errcheck release changelog-unreleased test-integration test-integration-up test-integration-down test-e2e-generate-certs test-e2e-swift test-e2e-registry test-e2e-spm test-e2e-full test-e2e-swift-https test-e2e-registry-https test-e2e-full-https

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
	@echo "  test-integration - Run integration tests (requires Docker; default Nexus, or MAVEN_PROVIDER=reposilite)"
	@echo "  test-integration-up - Start Maven test server (Nexus or Reposilite per MAVEN_PROVIDER)"
	@echo "  test-integration-down - Stop Maven test server(s)"
	@echo "  test-e2e-generate-certs - Generate E2E HTTPS certs (for optional HTTPS testing)"
	@echo "  test-e2e-spm - E2E: SPM proxy backend (no external services required; self-contained mock upstream)"
	@echo "  test-e2e-swift - E2E: Swift publish + resolve (Maven server must be up; requires Swift)"
	@echo "  test-e2e-registry - E2E: Registry HTTP API against file and Maven backends"
	@echo "  test-e2e-full - Start Maven server, run E2E Swift and registry tests, then stop"
	@echo "  test-e2e-swift-https - E2E Swift over HTTPS (requires certs: make test-e2e-generate-certs)"
	@echo "  test-e2e-registry-https - E2E registry API over HTTPS (requires certs)"
	@echo "  test-e2e-full-https - Full E2E over HTTPS (Maven server + certs required)"
	@echo ""
	@echo "Maven provider: MAVEN_PROVIDER=nexus (default) or reposilite. E2E HTTPS: E2E_HTTPS=1 or -https targets."
	@echo ""
	@echo "Example usage:"
	@echo "  make build              - Build the project"
	@echo "  make run               - Run the server locally"
	@echo "  make release VERSION=1.2.3 - Create release version 1.2.3"

build: tailwind lint
	go mod tidy && go mod verify && go mod download && go test ./... && go build -o openspmregistry main.go

lint: staticcheck golangci-lint errcheck

# Pinned versions (no @latest) to avoid supply-chain risk. Bump periodically for latest.
# staticcheck: v0.7.0 requires Go 1.25; v0.6.0 is latest for Go 1.23.
STATICCHECK_VER ?= v0.6.0
GOLANGCI_LINT_VER ?= v1.64.8
ERRCHECK_VER ?= v1.10.0

staticcheck:
	go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VER) ./...

golangci-lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VER) run ./...

# Catches unchecked errors including defer x.Close() (no -blank: explicit _ discards not reported).
errcheck:
	go run github.com/kisielk/errcheck@$(ERRCHECK_VER) ./...

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

# Maven test provider: nexus (default) or reposilite. Set MAVEN_PROVIDER=reposilite to use Reposilite.
MAVEN_PROVIDER ?= nexus

# Integration test targets
test-integration-up:
	@provider=$${MAVEN_PROVIDER:-nexus}; \
	echo "Starting Maven test server ($$provider)..."; \
	if [ "$$provider" = "reposilite" ]; then \
		docker-compose -f docker-compose.test.yml up -d reposilite; \
	else \
		docker-compose -f docker-compose.test.yml up -d nexus; \
	fi
	@provider=$${MAVEN_PROVIDER:-nexus}; \
	wait_timeout=180; [ -n "$${NEXUS_WAIT_TIMEOUT}" ] && wait_timeout=$${NEXUS_WAIT_TIMEOUT}; [ -n "$${GITHUB_ACTIONS}" ] && [ -z "$${NEXUS_WAIT_TIMEOUT}" ] && wait_timeout=360; \
	if [ "$$provider" = "reposilite" ]; then \
		echo "Waiting for Reposilite to be ready..."; \
		timeout=$$wait_timeout; \
		while [ $$timeout -gt 0 ]; do \
			ready=0; \
			if command -v curl > /dev/null 2>&1; then \
				if curl -sf http://localhost:8080/ > /dev/null 2>&1; then ready=1; fi; \
			fi; \
			if [ $$ready -eq 1 ]; then echo "Reposilite is ready!"; break; fi; \
			sleep 5; timeout=$$((timeout - 5)); \
		done; \
		if [ $$timeout -le 0 ]; then echo "Warning: Reposilite may not be ready. Check: docker ps"; exit 1; fi; \
		echo "Bootstrapping Reposilite (default private repo)..."; \
		go run ./cmd/reposilite-bootstrap; \
	else \
		echo "Waiting for Nexus to be ready (this may take 2-5 minutes on CI)..."; \
		timeout=$$wait_timeout; \
		while [ $$timeout -gt 0 ]; do \
			ready=0; \
			if command -v curl > /dev/null 2>&1; then \
				if curl -sf http://localhost:8081/service/rest/v1/status > /dev/null 2>&1; then ready=1; fi; \
			fi; \
			if [ $$ready -eq 1 ]; then echo "Nexus is ready!"; break; fi; \
			sleep 5; timeout=$$((timeout - 5)); \
		done; \
		if [ $$timeout -le 0 ]; then echo "Warning: Nexus may not be fully ready yet. Check with: docker ps"; exit 1; fi; \
		echo "Bootstrapping Nexus (create repo, set admin password)..."; \
		NEXUS_TEST_PASSWORD_FILE=.nexus-test-password go run ./cmd/nexus-bootstrap; \
	fi

test-integration-down:
	@echo "Stopping Maven test server(s)..."
	docker-compose -f docker-compose.test.yml down

test-integration: test-integration-up
	@provider=$${MAVEN_PROVIDER:-nexus}; \
	if [ "$$provider" = "reposilite" ]; then \
		passfile=".reposilite-test-token"; [ -f "$$passfile" ] && MAVEN_REPO_PASSWORD=$$(cat "$$passfile") || MAVEN_REPO_PASSWORD="test-secret"; \
		INTEGRATION_TESTS=1 MAVEN_REPO_URL="http://localhost:8080" MAVEN_REPO_NAME=private MAVEN_PROVIDER=reposilite MAVEN_REPO_USERNAME=e2e MAVEN_REPO_PASSWORD="$$MAVEN_REPO_PASSWORD" go test -tags=integration -v ./repo/maven/... -run TestIntegration; \
	else \
		passfile=".nexus-test-password"; [ -f "$$passfile" ] && MAVEN_REPO_PASSWORD=$$(cat "$$passfile") || MAVEN_REPO_PASSWORD=admin123; \
		INTEGRATION_TESTS=1 MAVEN_REPO_URL="http://localhost:8081/repository" MAVEN_REPO_NAME=private MAVEN_PROVIDER=nexus MAVEN_REPO_USERNAME=admin MAVEN_REPO_PASSWORD="$$MAVEN_REPO_PASSWORD" go test -tags=integration -v ./repo/maven/... -run TestIntegration; \
	fi; \
	r=$$?; $(MAKE) test-integration-down; exit $$r

# Generate E2E certs for optional HTTPS testing (testdata/e2e/certs/).
test-e2e-generate-certs:
	@go run ./cmd/e2e-generate-certs

# E2E SPM proxy: exercise the SPM proxy backend and split mode against a mock upstream.
# No external services required — the mock upstream is started in-process by the test.
test-e2e-spm:
	E2E_TESTS=1 go test -tags=e2e -v -count=1 ./e2e/... -run TestRegistryE2ESPM

# E2E Swift: publish package to OpenSPMRegistry (Maven-backed) and resolve from consumer.
# test-e2e-swift: run E2E test only (start Maven server first with make test-integration-up).
test-e2e-swift:
	@provider=$${MAVEN_PROVIDER:-nexus}; \
	if [ "$$provider" = "reposilite" ]; then \
	  passfile=".reposilite-test-token"; [ -f "$$passfile" ] && MAVEN_REPO_PASSWORD=$$(cat "$$passfile") || MAVEN_REPO_PASSWORD="test-secret"; \
	  MAVEN_REPO_URL="http://localhost:8080" MAVEN_REPO_NAME=private MAVEN_PROVIDER=reposilite MAVEN_REPO_USERNAME=e2e MAVEN_REPO_PASSWORD="$$MAVEN_REPO_PASSWORD" E2E_TESTS=1 E2E_REGISTRY_URL="$(E2E_REGISTRY_URL)" go test -tags=e2e -v -count=1 ./e2e/... -run TestSwiftPublishResolve; \
	else \
	  passfile=".nexus-test-password"; [ -f "$$passfile" ] && MAVEN_REPO_PASSWORD=$$(cat "$$passfile") || MAVEN_REPO_PASSWORD=admin123; \
	  MAVEN_REPO_URL="http://localhost:8081/repository" MAVEN_REPO_NAME=private MAVEN_PROVIDER=nexus MAVEN_REPO_USERNAME=admin MAVEN_REPO_PASSWORD="$$MAVEN_REPO_PASSWORD" E2E_TESTS=1 E2E_REGISTRY_URL="$(E2E_REGISTRY_URL)" go test -tags=e2e -v -count=1 ./e2e/... -run TestSwiftPublishResolve; \
	fi

# E2E registry: exercise registry HTTP API against file and Maven backends (no Swift required).
# test-e2e-registry: run E2E test only (start Maven server first with make test-integration-up for Maven backend).
test-e2e-registry:
	@provider=$${MAVEN_PROVIDER:-nexus}; \
	if [ "$$provider" = "reposilite" ]; then \
	  passfile=".reposilite-test-token"; [ -f "$$passfile" ] && MAVEN_REPO_PASSWORD=$$(cat "$$passfile") || MAVEN_REPO_PASSWORD="test-secret"; \
	  MAVEN_REPO_URL="http://localhost:8080" MAVEN_REPO_NAME=private MAVEN_PROVIDER=reposilite MAVEN_REPO_USERNAME=e2e MAVEN_REPO_PASSWORD="$$MAVEN_REPO_PASSWORD" E2E_TESTS=1 E2E_REGISTRY_URL="$(E2E_REGISTRY_URL)" go test -tags=e2e -v -count=1 ./e2e/... -run TestRegistryE2E; \
	else \
	  passfile=".nexus-test-password"; [ -f "$$passfile" ] && MAVEN_REPO_PASSWORD=$$(cat "$$passfile") || MAVEN_REPO_PASSWORD=admin123; \
	  MAVEN_REPO_URL="http://localhost:8081/repository" MAVEN_REPO_NAME=private MAVEN_PROVIDER=nexus MAVEN_REPO_USERNAME=admin MAVEN_REPO_PASSWORD="$$MAVEN_REPO_PASSWORD" E2E_TESTS=1 E2E_REGISTRY_URL="$(E2E_REGISTRY_URL)" go test -tags=e2e -v -count=1 ./e2e/... -run TestRegistryE2E; \
	fi

# test-e2e-full: start Maven server, bootstrap, run all E2E tests (Swift, registry, SPM proxy), then tear down.
# Always run test-integration-down so containers are stopped even when tests fail.
test-e2e-full: test-integration-up
	@$(MAKE) test-e2e-swift; r=$$?; $(MAKE) test-e2e-registry; r2=$$?; $(MAKE) test-e2e-spm; r3=$$?; $(MAKE) test-integration-down; \
	if [ $$r -ne 0 ]; then exit $$r; fi; if [ $$r2 -ne 0 ]; then exit $$r2; fi; exit $$r3

test-e2e-swift-https:
	@$(MAKE) test-e2e-swift E2E_HTTPS=1

test-e2e-registry-https:
	@$(MAKE) test-e2e-registry E2E_HTTPS=1

test-e2e-full-https: test-integration-up
	@$(MAKE) test-e2e-swift E2E_HTTPS=1; r=$$?; $(MAKE) test-e2e-registry E2E_HTTPS=1; r2=$$?; $(MAKE) test-integration-down; \
	if [ $$r -ne 0 ]; then exit $$r; fi; exit $$r2