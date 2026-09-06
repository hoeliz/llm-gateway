.PHONY: run test build fmt
run:
	go run ./cmd/gateway
test:
	go test -race ./...
	go vet ./...
build:
	go build -o bin/llm-gateway ./cmd/gateway
fmt:
	gofmt -w cmd internal
