.PHONY: build test run

build:
	go build -o da ./cmd/da

test:
	go test ./...

run: build
	./da run
