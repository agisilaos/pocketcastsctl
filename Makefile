.PHONY: build test vet fmt release

build:
	go build -o pocketcastsctl ./cmd/pocketcastsctl

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w cmd/pocketcastsctl/main.go internal

release:
	@if [ -z "$(VERSION)" ]; then echo "VERSION is required (e.g. make release VERSION=v0.1.0)"; exit 2; fi
	./scripts/release.sh "$(VERSION)"

