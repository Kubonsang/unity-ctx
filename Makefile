BINARY := unity-ctx
CMD := ./cmd/unity-ctx
SMOKE_SCENE := testdata/scenes/simple_scene.unity

.PHONY: build test vet lint smoke clean

build:
	go build -o $(BINARY) $(CMD)

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
