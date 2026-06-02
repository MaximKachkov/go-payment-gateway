FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/payment-gateway ./cmd/payment-gateway

FROM alpine:3.22

RUN adduser -D -H appuser
USER appuser

COPY --from=build /bin/payment-gateway /bin/payment-gateway

EXPOSE 50051
ENTRYPOINT ["/bin/payment-gateway"]
