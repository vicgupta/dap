.PHONY: build test run e2e release

build:
	go build -o da ./cmd/da

test:
	go test ./...

e2e:
	cd e2e && npm test

run: build
	./da run

# TAG=v0.1.0-alpha.2 make release
release:
	packaging/release.sh $(TAG)
