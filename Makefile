.PHONY: run fmt build

run:
	go run .

fmt:
	gofmt -w main.go

build:
	go build -o bin/devtunnel-tui .
