REGISTRY  := registry.adoah.dev/projects
PLATFORM  := linux/amd64
BUILDER   ?= homelab-remote-builder
IMAGES    := api ui

GIT_SHA    := $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
VERSION_ARGS = --build-arg VERSION=$(VERSION) --build-arg GIT_SHA=$(GIT_SHA) --build-arg BUILD_TIME=$(BUILD_TIME)
# Same registry cache CI reads and writes, so local and CI builds reuse each other's layers
CACHE_ARGS = --cache-from type=registry,ref=$(1):buildcache --cache-to type=registry,ref=$(1):buildcache,mode=max

GALLERY_DB ?= postgres://postgres:postgres@localhost:5433/gallery?sslmode=disable

.PHONY: db dev web test bin $(addprefix build-,$(IMAGES)) push deploy

db:                ## a local Postgres on :5433 (skipped when the container exists)
	@docker inspect photo-gallery-pg >/dev/null 2>&1 || \
		docker run -d --name photo-gallery-pg -p 5433:5432 -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=gallery postgres:16-alpine

dev:               ## public API on :8080 and admin API on :8081 against $(GALLERY_DB)
	GALLERY_DB=$(GALLERY_DB) go run ./cmd/photo-gallery serve

web:               ## the Vite dev server (proxies /api to :8080 and /admin-api to :8081)
	cd web && pnpm install && pnpm run dev

bin:               ## bin/photo-gallery
	CGO_ENABLED=0 go build -ldflags "-X main.version=$(VERSION)" -o bin/photo-gallery ./cmd/photo-gallery

test:              ## set GALLERY_TEST_PG to run the database tests (make db first)
	go vet ./... && go test -race ./...
	cd web && pnpm run build && pnpm run lint

build-%:
	docker buildx build --builder $(BUILDER) --platform $(PLATFORM) --file docker/$*.Dockerfile --target production \
		$(VERSION_ARGS) $(call CACHE_ARGS,$(REGISTRY)/photo-gallery-$*) \
		--tag $(REGISTRY)/photo-gallery-$*:$(VERSION) --tag $(REGISTRY)/photo-gallery-$*:latest --push .

push: $(addprefix build-,$(IMAGES))

deploy:            ## helm upgrade into production with the current VERSION tag
	helm repo add mononen-charts https://mononen.github.io/charts/ >/dev/null 2>&1 || true
	helm repo update mononen-charts >/dev/null
	helm dependency update .ci/chart
	helm upgrade --install photo-gallery .ci/chart --namespace production \
		--set mononen-library-chart.global.imageDefaults.tag=$(VERSION)
