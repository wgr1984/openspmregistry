TAILWIND = npx tailwindcss -i ./static/input.css -o ./static/output.css

.PHONY: build

build: tailwind
	go build -o openspmregistry main.go

build-docker: tailwind
	docker build -t openspmregistry -f .docker/Dockerfile .

run: tailwind
	go run main.go -tls=true -v

tailwind:
	$(TAILWIND)

tailwind-watch:
	$(TAILWIND) --watch