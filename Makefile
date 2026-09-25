.PHONY: build test check

build:
	go build -o bin/twentycrm ./cmd/twentycrm

test:
	go test ./...

check:
	go vet ./...
	go test ./...
