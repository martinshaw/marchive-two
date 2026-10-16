.PHONY: build test

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o marchive ./cmd/marchive

test:
	CGO_ENABLED=0 go test ./...
