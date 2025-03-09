TAILWIND = npx tailwindcss -i ./static/input.css -o ./static/output.css

.PHONY: build

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