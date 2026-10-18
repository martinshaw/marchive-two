.PHONY: build test ui

NPM ?= npm
ifeq ($(CI),true)
NPM_INSTALL = $(NPM) ci --prefix web
else
NPM_INSTALL = $(NPM) install --prefix web
endif

# ui installs the web dependencies and writes the page embedded by the Go build.
ui:
	$(NPM_INSTALL)
	$(NPM) run build --prefix web

build: ui
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o marchive ./cmd/marchive

test:
	CGO_ENABLED=0 go test ./...
