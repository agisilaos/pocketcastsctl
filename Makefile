.PHONY: build test test-race-local bench-local bench-local-ci test-scripts test-scripts-cover vet fmt fmt-check check-help check-help-docs docs-check changelog-context release-check release-check-ci release release-dry-run

build:
	go build -o pocketcastsctl ./cmd/pocketcastsctl
	@echo "built: ./pocketcastsctl"

test:
	go test ./...

test-race-local:
	go test -race ./internal/localplayback ./internal/app ./cmd/pocketcastsctl

bench-local:
	go test ./internal/localplayback -run '^$$' -bench 'BenchmarkSnapshot' -benchmem

bench-local-ci:
	go test ./internal/localplayback -run '^$$' -bench 'BenchmarkSnapshot' -benchmem -benchtime=100x

test-scripts:
	go test ./scripts -run 'TestReleaseCheckModes|TestChangelogTraceability|TestCheckHelpDocsDriftScript|TestReleaseUsesConfigurableHTTPSHomebrewTapRemote'

test-scripts-cover:
	go test -cover ./scripts

vet:
	go vet ./...

fmt:
	gofmt -w cmd internal scripts

fmt-check:
	@test -z "$$(gofmt -l cmd internal scripts)"

check-help:
	./scripts/check-help.sh

check-help-docs:
	./scripts/check-help.sh

docs-check:
	./scripts/docs-check.sh

changelog-context:
	@if [ -z "$(VERSION)" ]; then echo "VERSION is required (e.g. make changelog-context VERSION=v0.1.0)"; exit 2; fi
	./scripts/changelog-context.sh "$(VERSION)"

release-check:
	@if [ -z "$(VERSION)" ]; then echo "VERSION is required (e.g. make release-check VERSION=v0.1.0)"; exit 2; fi
	./scripts/release-check.sh "$(VERSION)"

release-check-ci:
	./scripts/release-check.sh --ci

release:
	@if [ -z "$(VERSION)" ]; then echo "VERSION is required (e.g. make release VERSION=v0.1.0)"; exit 2; fi
	./scripts/release.sh "$(VERSION)"

release-dry-run:
	@if [ -z "$(VERSION)" ]; then echo "VERSION is required (e.g. make release-dry-run VERSION=v0.1.0)"; exit 2; fi
	./scripts/release.sh "$(VERSION)" --dry-run
