# go-payment-gateway

Prototype payment gateway microservice built with Go, gRPC, PostgreSQL and Docker Compose.

The project demonstrates practical backend concepts:

- gRPC API described in `.proto`
- PostgreSQL schema migrations
- transaction handling with `pgx`
- idempotent write requests
- refund workflow
- clean project structure
- structured JSON logging
- Docker Compose environment

## API

The service exposes four gRPC methods:

- `CreatePayment` creates a `PENDING` payment.
- `ConfirmPayment` moves a payment to `SUCCEEDED`.
- `RefundPayment` creates a refund transaction and moves a payment to `REFUNDED`.
- `GetPayment` returns a payment with its transactions.

Write requests require an `idempotency_key`. If the same key is sent with the same request payload, the service returns the saved response. If the same key is reused with different payload, the service returns an error.

## Requirements

- Go 1.26+
- Git
- Docker Desktop
- Optional: `grpcurl` for manual gRPC calls

The project uses `buf` and Go protobuf plugins for code generation. They can be installed locally into `work/bin` with:

Windows PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\install-tools.ps1
powershell -ExecutionPolicy Bypass -File .\scripts\generate-proto.ps1
```

macOS/Linux:

```bash
make tools
make proto
```

## Run With Docker Compose

```bash
docker compose up --build
```

The app listens on:

```text
localhost:50051
```

## Local Development

Generate gRPC code:

Windows PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\install-tools.ps1
powershell -ExecutionPolicy Bypass -File .\scripts\generate-proto.ps1
```

macOS/Linux:

```bash
make tools
make proto
```

Run tests:

```bash
make test
```

Run the app locally:

```bash
go run ./cmd/payment-gateway
```

Default PostgreSQL DSN:

```text
postgres://postgres:postgres@localhost:5432/payment_gateway?sslmode=disable
```

## Example grpcurl Calls

If `grpcurl` is not installed locally, use the Docker image:

```powershell
'{"amount":2500,"currency":"EUR","idempotency_key":"docker-grpcurl-create-001"}' |
  docker run --rm -i --network host fullstorydev/grpcurl:latest `
  -plaintext -d '@' localhost:50051 payment.v1.PaymentGateway/CreatePayment
```

Create a payment:

```bash
grpcurl -plaintext \
  -d '{"amount": 1999, "currency": "USD", "idempotency_key": "create-001"}' \
  localhost:50051 payment.v1.PaymentGateway/CreatePayment
```

Confirm a payment:

```bash
grpcurl -plaintext \
  -d '{"payment_id": "PASTE_PAYMENT_ID", "idempotency_key": "confirm-001"}' \
  localhost:50051 payment.v1.PaymentGateway/ConfirmPayment
```

Refund a payment:

```bash
grpcurl -plaintext \
  -d '{"payment_id": "PASTE_PAYMENT_ID", "amount": 1999, "idempotency_key": "refund-001"}' \
  localhost:50051 payment.v1.PaymentGateway/RefundPayment
```

Get a payment:

```bash
grpcurl -plaintext \
  -d '{"payment_id": "PASTE_PAYMENT_ID"}' \
  localhost:50051 payment.v1.PaymentGateway/GetPayment
```

## Project Structure

```text
cmd/payment-gateway      application entry point
proto/payment/v1         gRPC contract
gen/payment/v1           generated protobuf code
internal/domain          business entities and errors
internal/usecase         application logic
internal/repository      PostgreSQL persistence
internal/transport/grpc  gRPC handlers
migrations               database migrations
```
