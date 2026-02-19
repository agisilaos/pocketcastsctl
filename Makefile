.PHONY: build test test-scripts vet fmt fmt-check check-help-docs docs-check release-preflight release-check release release-dry-run

build:
	go build -o pocketcastsctl ./cmd/pocketcastsctl

test:
	go test ./...

test-scripts:
	go test ./scripts -run 'TestReleasePreflightFailurePaths|TestCheckHelpDocsDriftScript'

vet:
	go vet ./...

fmt:
	gofmt -w cmd internal scripts

fmt-check:
	@test -z "$$(gofmt -l cmd internal scripts)"

check-help-docs:
	./scripts/check-help-docs-drift.sh

docs-check:
	./scripts/docs-check.sh

release-preflight:
	@if [ -z "$(VERSION)" ]; then echo "VERSION is required (e.g. make release-preflight VERSION=v0.1.0)"; exit 2; fi
	./scripts/release_preflight.sh "$(VERSION)"

release-check:
	@if [ -z "$(VERSION)" ]; then echo "VERSION is required (e.g. make release-check VERSION=v0.1.0)"; exit 2; fi
	./scripts/release-check.sh "$(VERSION)"

release:
	@if [ -z "$(VERSION)" ]; then echo "VERSION is required (e.g. make release VERSION=v0.1.0)"; exit 2; fi
	./scripts/release.sh "$(VERSION)"

release-dry-run:
	@if [ -z "$(VERSION)" ]; then echo "VERSION is required (e.g. make release-dry-run VERSION=v0.1.0)"; exit 2; fi
	./scripts/release.sh "$(VERSION)" --dry-run
