APP_NAME := payment-gateway
PROTOC_GEN_GO := ./work/bin/protoc-gen-go
PROTOC_GEN_GO_GRPC := ./work/bin/protoc-gen-go-grpc
BUF := ./work/bin/buf

.PHONY: tools proto tidy build test run compose-up compose-down

tools:
	GOBIN=$$(pwd)/work/bin go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	GOBIN=$$(pwd)/work/bin go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	GOBIN=$$(pwd)/work/bin go install github.com/bufbuild/buf/cmd/buf@latest

proto:
	PATH=$$(pwd)/work/bin:$$PATH $(BUF) generate

tidy:
	go mod tidy

build:
	go build -o ./bin/$(APP_NAME) ./cmd/payment-gateway

test:
	go test ./...

run:
	go run ./cmd/payment-gateway

compose-up:
	docker compose up --build

compose-down:
	docker compose down -v
