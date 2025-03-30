TAILWIND = npx tailwindcss -i ./static/input.css -o ./static/output.css

.PHONY: help build clean build-docker run tailwind tailwind-watch

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
	@echo ""
	@echo "Example usage:"
	@echo "  make build    - Build the project"
	@echo "  make run      - Run the server locally"

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