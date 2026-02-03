.PHONY: build run test clean

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GORUN=$(GOCMD) run
GOTEST=$(GOCMD) test
GOCLEAN=$(GOCMD) clean
GOMOD=$(GOCMD) mod

# Binary name
BINARY_NAME=airbridge
BINARY_PATH=bin/$(BINARY_NAME)

# Build directories
BUILD_DIR=bin

all: build

build:
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BINARY_PATH) ./cmd/airbridge

run:
	$(GORUN) ./cmd/airbridge

test:
	$(GOTEST) -v ./...

clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)

deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Development helpers
fmt:
	gofmt -w .

lint:
	golangci-lint run

# Docker
DOCKER_IMAGE=ghcr.io/kenyonj/airbridge

docker-build:
	docker build -t $(DOCKER_IMAGE):latest .

docker-run:
	docker run --rm --network=host -v airbridge-data:/data $(DOCKER_IMAGE):latest

docker-push:
	docker push $(DOCKER_IMAGE):latest

# Install dependencies for CGO (libraop)
setup-libraop:
	@echo "Setting up libraop..."
	@mkdir -p vendor/libraop
	git clone --depth 1 https://github.com/philippe44/libraop.git vendor/libraop
	cd vendor/libraop && git submodule update --init
	@echo "libraop cloned to vendor/libraop"
