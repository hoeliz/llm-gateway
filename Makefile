.PHONY: run test build fmt ui ui-dev
run:
	go run ./cmd/gateway
test:
	go test -race ./...
	go vet ./...
	cd web && npm test
build:
	go build -o bin/llm-gateway ./cmd/gateway
ui:
	cd web && npm ci && npm run build
ui-dev:
	cd web && npm run dev
fmt:
	gofmt -w cmd internal
