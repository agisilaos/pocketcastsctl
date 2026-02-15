.PHONY: build test test-scripts vet fmt fmt-check check-help-docs release-preflight release

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

release-preflight:
	@if [ -z "$(VERSION)" ]; then echo "VERSION is required (e.g. make release-preflight VERSION=v0.1.0)"; exit 2; fi
	./scripts/release_preflight.sh "$(VERSION)"

release:
	@if [ -z "$(VERSION)" ]; then echo "VERSION is required (e.g. make release VERSION=v0.1.0)"; exit 2; fi
	./scripts/release.sh "$(VERSION)"
