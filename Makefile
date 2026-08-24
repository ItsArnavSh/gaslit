GO ?= go
GOLANGCI_LINT ?= golangci-lint

.PHONY: fmt vet build test tidy lint check clean

fmt:
	@test -z "$$(gofmt -l -s .)" || { echo "The following files need formatting (run: make fmt-fix):"; gofmt -l -s .; exit 1; }

fmt-fix:
	gofmt -s -w .

vet:
	$(GO) vet ./...

build:
	$(GO) build ./...

test:
	$(GO) test -race -cover ./...

tidy:
	$(GO) mod tidy

lint:
	$(GOLANGCI_LINT) run ./...

check: fmt vet build test

clean:
	$(GO) clean ./...
