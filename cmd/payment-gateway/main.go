package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	paymentv1 "github.com/maxotik/go-payment-gateway/gen/payment/v1"
	"github.com/maxotik/go-payment-gateway/internal/repository/postgres"
	grpcserver "github.com/maxotik/go-payment-gateway/internal/transport/grpc"
	"github.com/maxotik/go-payment-gateway/internal/usecase"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("service stopped", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := loadConfig()

	pool, err := connectPostgres(ctx, cfg.PostgresDSN)
	if err != nil {
		return err
	}
	defer pool.Close()

	repo := postgres.New(pool)
	service := usecase.NewPaymentService(repo)

	listener, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return err
	}

	server := grpc.NewServer()
	paymentv1.RegisterPaymentGatewayServer(server, grpcserver.NewServer(service))
	reflection.Register(server)

	go func() {
		<-ctx.Done()
		logger.Info("shutdown signal received")
		server.GracefulStop()
	}()

	logger.Info("payment gateway started", slog.String("grpc_addr", cfg.GRPCAddr))
	if err := server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return err
	}
	return nil
}

type config struct {
	GRPCAddr    string
	PostgresDSN string
}

func loadConfig() config {
	return config{
		GRPCAddr:    envOrDefault("GRPC_ADDR", ":50051"),
		PostgresDSN: envOrDefault("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/payment_gateway?sslmode=disable"),
	}
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func connectPostgres(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}
