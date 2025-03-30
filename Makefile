TAILWIND = npx tailwindcss -i ./static/input.css -o ./static/output.css
VERSION ?= 0.0.0
RELEASE_TYPE ?= patch

.PHONY: help build clean build-docker run tailwind tailwind-watch release changelog-unreleased

# Default target when no arguments are given to make
help:
	@echo "OpenSPM Registry Makefile Help"
	@echo "=============================="
	@echo ""
	@echo "Available targets:"
	@echo "  help          - Show this help message"
	@echo "  build         - Build the project (includes tests and dependencies)"
	@echo "  clean         - Clean Go build cache"
	@echo "  build-docker  - Build Docker image"
	@echo "  run           - Run the server locally"
	@echo "  tailwind      - Build Tailwind CSS"
	@echo "  tailwind-watch- Watch and rebuild Tailwind CSS on changes"
	@echo "  release       - Create a new release (Usage: make release VERSION=1.2.3)"
	@echo "  changelog-unreleased - Show unreleased changes"
	@echo ""
	@echo "Example usage:"
	@echo "  make build              - Build the project"
	@echo "  make run               - Run the server locally"
	@echo "  make release VERSION=1.2.3 - Create release version 1.2.3"

build: build tailwind
	go mod tidy && go mod verify && go mod download && go test ./... && go build -o openspmregistry main.go

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
	sed -i.bak -e "/## \[Unreleased\]/i\\\n## [$(VERSION)] - $$DATE\n" CHANGELOG.md; \
	rm -f CHANGELOG.md.bak
	@# Commit CHANGELOG.md
	git add CHANGELOG.md
	git commit -m "chore: update CHANGELOG for v$(VERSION)"
	@# Create and push tag
	git tag -a v$(VERSION) -m "Release v$(VERSION)"
	git push origin main v$(VERSION)
	@echo "Release v$(VERSION) created and pushed. GitHub Actions will handle the Docker build and publish."

changelog-unreleased:
	@echo "Unreleased changes:"
	@sed -n '/## \[Unreleased\]/,/## \[[0-9]\+\.[0-9]\+\.[0-9]\+\]/p' CHANGELOG.md