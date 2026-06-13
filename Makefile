VERSION ?= 0.1.0
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    := $(shell date +%Y-%m-%d)
PKG     := calcassist/internal/version
MAIN    := ./cmd/calcassist
LDFLAGS := -s -w -X '$(PKG).Version=$(VERSION)' -X '$(PKG).Commit=$(COMMIT)' -X '$(PKG).Date=$(DATE)'

export CGO_ENABLED=0

.PHONY: build all test vet fmt fmtcheck tidy clean run

build: ## Build for the host OS into ./bin
	@mkdir -p bin
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/calcassist $(MAIN)

all: ## Cross-compile windows/linux/darwin (amd64+arm64) into ./dist
	@mkdir -p dist
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/calcassist-windows-amd64.exe $(MAIN)
	GOOS=windows GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/calcassist-windows-arm64.exe $(MAIN)
	GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/calcassist-linux-amd64 $(MAIN)
	GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/calcassist-linux-arm64 $(MAIN)
	GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/calcassist-darwin-amd64 $(MAIN)
	GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/calcassist-darwin-arm64 $(MAIN)

test: ## Run all tests
	go test ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Format the code
	gofmt -w .

fmtcheck: ## Fail if any file is unformatted
	@test -z "$$(gofmt -l .)" || (echo "unformatted files:"; gofmt -l .; exit 1)

tidy: ## Tidy go.mod/go.sum
	go mod tidy

clean: ## Remove build artifacts
	rm -rf bin dist

run: build ## Build and run
	./bin/calcassist
