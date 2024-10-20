.PHONY: build

build:
	docker build -t openspmregistry -f .docker/Dockerfile .