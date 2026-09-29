VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
# Where the update check reads the latest release, for example
# https://api.github.com/repos/<org>/plumb/releases/latest. Empty turns it off.
UPDATE_URL ?=
INSTALL_HINT ?=
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.updateURL=$(UPDATE_URL) -X 'main.installHint=$(INSTALL_HINT)'

PLATFORMS := linux/amd64 linux/arm64 darwin/arm64

.PHONY: build test vet demo golden dist clean

build:
	go build -ldflags "$(LDFLAGS)" -o bin/plumb ./cmd/plumb

test:
	go test ./...

vet:
	gofmt -l . | tee /dev/stderr | (! read)
	go vet ./...

# The built-in lab: every layer, no cloud, no network.
demo:
	go run ./cmd/plumb demo

golden:
	go test ./cmd/plumb -update

# Static binaries for servers: dist/plumb-<os>-<arch>.
dist:
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath \
			-ldflags "$(LDFLAGS)" -o dist/plumb-$$os-$$arch ./cmd/plumb \
			&& echo "dist/plumb-$$os-$$arch"; \
	done

clean:
	rm -rf bin dist
