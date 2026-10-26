CONTAINER_RUNTIME ?= podman
IMAGE ?= tenttiarkisto
REGISTRY ?= rg.fr-par.scw.cloud
REGISTRY_NAMESPACE ?= tenttiarkisto
PLATFORM ?= linux/amd64
VERSION ?= $(shell git rev-parse --short HEAD)
REGION ?= fr-par

.PHONY: help
help:
	@echo "targets: geo-update test vet fmt build image push tf-init tf-plan tf-apply run-migrate run-seed"

# Regenerate Finnish IP prefixes from RIPE delegated stats
# (downloads ~10 MB, needs network)
.PHONY: geo-update
geo-update:
	go run ./scripts/genfi
	gofmt -w internal/geogate/fi_prefixes.go

.PHONY: test
test:
	go test ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: fmt
fmt:
	gofmt -w internal cmd scripts

.PHONY: fmt-check
fmt-check:
	@files=$$(gofmt -l internal cmd scripts); \
	if [ -n "$$files" ]; then echo "unformatted:"; echo "$$files"; exit 1; fi

.PHONY: build
build:
	go build ./...

# Build the container image (amd64 for Scaleway).
.PHONY: image
image:
	$(CONTAINER_RUNTIME) build --platform $(PLATFORM) \
		-t $(IMAGE):$(VERSION) -f Dockerfile .
	$(CONTAINER_RUNTIME) tag $(IMAGE):$(VERSION) $(IMAGE):latest

# Push to the Scaleway registry (deploy machine only).
.PHONY: push
push:
	$(CONTAINER_RUNTIME) push $(REGISTRY)/$(REGISTRY_NAMESPACE)/$(IMAGE):$(VERSION)

# --- terraform (deploy machine only; see docs/DEPLOY.md) ---
.PHONY: tf-init
tf-init:
	terraform -chdir=infra init

.PHONY: tf-plan
tf-plan:
	terraform -chdir=infra plan -var image_tag=$(VERSION) -out tfplan

.PHONY: tf-apply
tf-apply:
	terraform -chdir=infra apply tfplan

# --- one-off jobs against the database (deploy machine only) ---
# Run migrations from the local machine through the DB endpoint:
#   make run-migrate DATABASE_URL=postgres://...
.PHONY: run-migrate
run-migrate:
	go run ./cmd/migrate

.PHONY: run-seed
run-seed:
	go run ./cmd/migrate -seed