SHELL := /bin/bash

# Container and Cloud Run destination. Override any value per environment.
GCP_PROJECT ?= nail-lab-449417
GCP_REGION ?= europe-west1
AR_REPOSITORY ?= aestheticlab
IMAGE_NAME ?= web
IMAGE ?= $(GCP_REGION)-docker.pkg.dev/$(GCP_PROJECT)/$(AR_REPOSITORY)/$(IMAGE_NAME)
SERVICE ?= aestheticlab-web

define check-tag
	@if [ -z "$(TAG)" ]; then \
		echo "TAG is required (for example: make $@ TAG=$$(git rev-parse --short HEAD))"; \
		exit 1; \
	fi
endef

define check-digest
	@case "$(DIGEST)" in \
		sha256:[0-9a-f][0-9a-f]*) ;; \
		*) echo "DIGEST must be an immutable sha256:<digest> value"; exit 1 ;; \
	esac
endef

define check-revision
	@if [ -z "$(REVISION)" ]; then \
		echo "REVISION is required"; \
		exit 1; \
	fi
endef

# Help target (default)
.PHONY: help
help: ## Show available targets and usage
	@echo "Usage: make <target> [TAG=your-tag]"
	@echo
	@echo "Targets:"
	@grep -E '^[a-zA-Z1-9._-]+:.*?##' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?##"}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: dev
dev: ## Run the development server
	bun run dev

.PHONY: build
build: ## Build the project for production
	bun run build

.PHONY: lint
lint: ## Run linting and formatting (biome)
	bun run biome

.PHONY: clean
clean: ## Remove build artifacts and node_modules
	rm -rf dist server node_modules
	bun install

.PHONY: docker-build-push
docker-build-push: ## Build, attest, and push image to Artifact Registry (requires TAG)
	$(call check-tag)
	docker buildx build \
		--platform linux/amd64 \
		--provenance=mode=max \
		--sbom=true \
		-t $(IMAGE):$(TAG) \
		--push .

.PHONY: gcloud-deploy-candidate
gcloud-deploy-candidate: ## Deploy an immutable Cloud Run candidate without traffic (requires DIGEST)
	$(call check-digest)
	gcloud run deploy $(SERVICE) \
		--image=$(IMAGE)@$(DIGEST) \
		--no-traffic \
		--region=$(GCP_REGION) \
		--project=$(GCP_PROJECT)

.PHONY: gcloud-promote
gcloud-promote: ## Promote a verified Cloud Run revision (requires REVISION)
	$(call check-revision)
	gcloud run services update-traffic $(SERVICE) \
		--to-revisions=$(REVISION)=100 \
		--region=$(GCP_REGION) \
		--project=$(GCP_PROJECT)

# Go site (cmd/web) ------------------------------------------------------------
# The Go + templ + Datastar rewrite (plans/go-datastar-scale-to-zero.md) lives
# beside the Qwik site until Phase 10. Until then dev, lint and build stay
# Qwik's, and the Go equivalents are go-dev, go-lint and go-build.
GO ?= go
AIR ?= air
GOLANGCI_LINT ?= $(GO) tool golangci-lint
GO_DIRS := cmd internal static

.PHONY: templ templ-check go-lint test security go-build ci go-dev assets assets-check frontend-test firestore snapshot

templ: ## Go: generate Go code from templ templates
	$(GO) tool templ generate

templ-check: ## Go: fail when generated templ code is out of date
	$(GO) tool templ generate -check

go-lint: templ-check ## Go: go vet, staticcheck, golangci-lint and gofmt
	$(GO) vet ./...
	$(GO) tool staticcheck ./...
	$(GOLANGCI_LINT) run ./...
	@test -z "$$(gofmt -l $(GO_DIRS))" || (echo "Unformatted Go files:" && gofmt -l $(GO_DIRS) && exit 1)

test: ## Go: race-enabled test suite
	$(GO) test -race -count=1 ./...

security: ## Go: govulncheck and gosec
	$(GO) tool govulncheck ./...
	$(GO) tool gosec -quiet ./...

go-build: ## Go: build the static site binary at bin/web
	mkdir -p bin
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "-s -w" -o bin/web ./cmd/web

ci: assets-check frontend-test templ-check go-lint test security go-build ## Go: the complete quality gate (CI runs it)

go-dev: assets ## Go: run the site with Air on http://localhost:3000 (PORT changes it)
	$(AIR)

# Generated Go code the build writes and the repository keeps; the built
# files themselves (static/dist/) are not committed.
ASSETS_GENERATED := internal/assets/manifest_gen.go internal/views/icons_gen.go

assets: ## Go: build CSS, fonts, Datastar, icons and image variants into static/dist/
	bun run assets

assets-check: assets ## Go: fail when the manifest or icon code differs from what git has
	@git diff --quiet -- $(ASSETS_GENERATED) && test -z "$$(git ls-files --others --exclude-standard -- $(ASSETS_GENERATED))" || \
		(echo "Out of date (run make assets, then git add):" && git status --short -- $(ASSETS_GENERATED) && exit 1)

frontend-test: ## Go: unit tests for the asset and icon generators
	bun run test.frontend

firestore: ## Go: start the Firestore emulator (TODO: Phase 5)
	@echo "make $@ is not implemented yet: plans/go-datastar-scale-to-zero.md, Phase 5" >&2; exit 1

snapshot: ## Go: refresh the embedded content snapshot (TODO: Phase 5)
	@echo "make $@ is not implemented yet: plans/go-datastar-scale-to-zero.md, Phase 5" >&2; exit 1
