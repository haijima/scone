
.PHONY: build

build:
	go build -ldflags="-s -w" -trimpath -o scone ./cmd/scone/...

run:
	go run ./cmd/scone/...

licenses:
	go-licenses report github.com/haijima/scone/cmd/scone > licenses.csv

lint:
	golangci-lint run ./...

test:
	go test ./...

check: lint test

