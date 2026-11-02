.PHONY: build run test test-go test-web vet fmt clean

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

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.git/*')

clean:
	rm -rf bin
