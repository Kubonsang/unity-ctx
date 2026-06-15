BINARY := unity-ctx
CMD := ./cmd/unity-ctx
SMOKE_SCENE := testdata/scenes/simple_scene.unity
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/Kubonsang/unity-ctx/internal/version.Version=$(VERSION)

.PHONY: build test vet lint smoke clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(CMD)

test:
	go test ./...

vet:
	go vet ./...

# Requires golangci-lint to be installed (https://golangci-lint.run/usage/install/).
lint:
	golangci-lint run

smoke: build
	./$(BINARY) scene summarize $(SMOKE_SCENE)

clean:
	rm -f $(BINARY)
