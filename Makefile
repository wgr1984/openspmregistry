TAILWIND = npx tailwindcss -i ./static/input.css -o ./static/output.css

.PHONY: build

build: tailwind
	docker build -t openspmregistry -f .docker/Dockerfile .

tailwind:
	$(TAILWIND)

tailwind-watch:
	$(TAILWIND) --watch