BINARY := testkube-ai-metrics-parser
PKG := ./cmd/testkube-ai-metrics-parser
VERSION ?= dev

.PHONY: build test lint fmt vet docker run clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/$(BINARY) $(PKG)

test:
	go test -race ./...

# Regenerate golden fixtures after intentional parser output changes.
golden:
	go test ./internal/parser/all -run TestGolden -update

lint: vet
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi

fmt:
	gofmt -w .

vet:
	go vet ./...

docker:
	docker build --build-arg VERSION=$(VERSION) -t testkube-ai-metrics-parser:$(VERSION) .

run: build
	./bin/$(BINARY) $(ARGS)

clean:
	rm -rf bin dist
