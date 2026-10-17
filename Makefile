.PHONY: build test race race-integration vet lint vuln mod-verify docs-hygiene bench-routing coverage verify verify-race docker-verify run docker-build clean web-build migrate-build

# Version injected into the binary at build time; "dev" when not on a tag.
VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || echo dev)
# Build provenance injected alongside VERSION and surfaced by GET /api/about.
# COMMIT stays empty outside a git checkout; the About page then renders an
# em-dash instead of a fabricated SHA.
COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
VERSION_PKG := github.com/deliciousbuding/metapi-go/internal/version
LDFLAGS := -s -w -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).BuildTime=$(BUILD_TIME)

# Build the server binary (requires web/dist/ to exist for go:embed)
build:
	go build -trimpath -ldflags="$(LDFLAGS)" -o metapi ./cmd/server

# Run tests
test:
	go test ./... -count=1 -timeout=60s

# Run tests with the Go race detector
race:
	bash ./scripts/go-race.sh

# Run integration tests with the Go race detector (requires PG_TEST_DSN)
race-integration:
	go test ./... -count=1 -race -tags=integration -timeout=180s

# Run go vet
vet:
	go vet ./...

# Run linter (requires golangci-lint)
lint:
	golangci-lint run --timeout=3m ./...

# Run dependency vulnerability scan
vuln:
	go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...

# Verify downloaded modules match go.sum checksums
mod-verify:
	go mod verify

# Check the published tree for local paths, credential examples, AI citation artifacts,
# unsupported runtime claims, and internal work-programme codes
docs-hygiene:
	go test ./docs -run 'Hygiene|ProgrammeCode' -count=1

# Run routing benchmark smoke set with allocation reporting
bench-routing:
	go test ./routing -run '^$$' -bench '^BenchmarkCalculateWeightedSelection' -benchmem -count=5

# Generate aggregate coverage profile
coverage:
	go test ./... -count=1 -coverprofile=coverage.out

# Full local gate, followed by release-specific static checks and binaries.
verify:
	bash .githooks/pre-push-project </dev/null
	$(MAKE) mod-verify lint vuln build migrate-build

# Compatibility alias: verify already includes the full race suite.
verify-race: verify

# Local container release gate (requires Docker)
docker-verify: docker-build

# Run the server locally
run:
	go run ./cmd/server

# Build Docker image (multi-stage: frontend + Go)
docker-build:
	docker build -t metapi-go:dev .

# Build the React frontend (requires Bun)
web-build:
	cd web && sh ../scripts/bun-install.sh && bun run build:web

# Build the standalone migration tool
migrate-build:
	go build -trimpath -ldflags="-s -w" -o metapi-migrate ./cmd/migrate

# Clean build artifacts
clean:
	rm -f metapi metapi.exe metapi-migrate metapi-migrate.exe
	rm -rf web/dist

cascade-e2e:
	go test ./e2e -count=1 -run 'CascadeIsolation' -timeout 60s
