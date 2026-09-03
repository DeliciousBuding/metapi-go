.PHONY: build test race race-integration vet lint vuln mod-verify docs-hygiene bench-routing coverage verify verify-race docker-verify run docker-build clean web-build migrate-build electron-build

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

# Check public Markdown for local paths, credential examples, AI citation artifacts, and unsupported runtime claims
docs-hygiene:
	go test ./docs -run TestPublicMarkdownHygiene -count=1

# Run routing benchmark smoke set with allocation reporting
bench-routing:
	go test ./routing -run '^$$' -bench '^BenchmarkCalculateWeightedSelection' -benchmem -count=5

# Generate aggregate coverage profile
coverage:
	go test ./... -count=1 -coverprofile=coverage.out

# Local release gate
verify: docs-hygiene mod-verify test vet lint vuln build migrate-build

# Local release gate plus race detector (requires a working CGO/C toolchain)
verify-race: verify race

# Local container release gate (requires Docker)
docker-verify: docker-build

# Run the server locally
run:
	go run ./cmd/server

# Build Docker image (multi-stage: frontend + Go)
docker-build:
	docker build -t metapi-go:latest .

# Build the React frontend (requires Bun)
web-build:
	cd web && bun install --frozen-lockfile && bun run build:web

# Build the standalone migration tool
migrate-build:
	go build -trimpath -ldflags="-s -w" -o metapi-migrate ./cmd/migrate

# Build the Electron desktop shell (Go binary + electron-packager output).
# Requires Node.js >= 18 and npm. Runs scripts/build-electron.sh / .ps1.
electron-build:
ifeq ($(OS),Windows_NT)
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build-electron.ps1
else
	bash ./scripts/build-electron.sh
endif

# Clean build artifacts
clean:
	rm -f metapi metapi.exe metapi-migrate metapi-migrate.exe
	rm -rf web/dist
	rm -rf electron/metapi electron/metapi.exe electron/dist electron/node_modules

cascade-e2e:
	go test ./e2e -count=1 -run 'CascadeIsolation' -timeout 60s
