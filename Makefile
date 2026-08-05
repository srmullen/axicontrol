BINARY := axicontrold
IMAGE := axicontrol

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "%-14s %s\n", $$1, $$2}'

.PHONY: build
build: ## Compile the axicontrold binary into ./bin
	go build -o bin/$(BINARY) ./cmd/axicontrold

.PHONY: run
run: ## Run the service locally
	go run ./cmd/axicontrold

.PHONY: test
test: ## Run the Go test suite
	go test ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

.PHONY: check
check: vet lint test ## Run vet, lint, and test together (the CI baseline)

.PHONY: tidy
tidy: ## Run go mod tidy
	go mod tidy

.PHONY: docker-build
docker-build: ## Build the container image
	docker build -t $(IMAGE) .

.PHONY: ci
ci: ## Run the full Dagger pipeline locally (lint -> test -> build)
	dagger call -m ./ci ci --source=.

.PHONY: clean
clean: ## Remove local build artifacts and dev data
	rm -rf bin data

.DEFAULT_GOAL := help
