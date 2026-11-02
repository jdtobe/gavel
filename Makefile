.PHONY: build run test test-go test-web vet fmt clean e2e

build:
	go build -o bin/gavel ./cmd/gavel

run:
	go run ./cmd/gavel

test: test-go test-web

test-go:
	go test ./...

test-web:
	@if [ -d web ]; then node --test web/; else echo "web/ not present yet; skipping browser tests"; fi

vet:
	go vet ./...

# Explicitly opts into a multi-GB download in a temporary GAVEL_HOME.
e2e:
	GAVEL_E2E=1 go test ./e2e -run TestRealKev -v -timeout 50m

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.git/*')

clean:
	rm -rf bin
