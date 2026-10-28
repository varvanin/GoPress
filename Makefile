.PHONY: generate assets css release build test check run
TEMPL ?= templ

generate:
	@test "$$($(TEMPL) version)" = "v0.3.1070" || (echo 'Install templ v0.3.1070 (see README)'; exit 1)
	$(TEMPL) generate

assets:
	npm run build

css:
	npm run build:css

release: assets generate
	$(MAKE) build

build:
	go build -trimpath -o bin/gopress ./cmd/gopress

test:
	go test ./...

check:
	@test -z "$$(gofmt -l cmd internal)"
	npm test
	go vet ./...
	go test -race ./...

run: build
	./bin/gopress serve
