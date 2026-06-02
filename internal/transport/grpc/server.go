package grpc

import (
	"context"
	"errors"

	paymentv1 "github.com/maxotik/go-payment-gateway/gen/payment/v1"
	"github.com/maxotik/go-payment-gateway/internal/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type PaymentUseCase interface {
	CreatePayment(ctx context.Context, amount int64, currency, idempotencyKey string) (*domain.Payment, error)
	ConfirmPayment(ctx context.Context, paymentID, idempotencyKey string) (*domain.Payment, error)
	RefundPayment(ctx context.Context, paymentID string, amount int64, idempotencyKey string) (*domain.Payment, error)
	GetPayment(ctx context.Context, paymentID string) (*domain.Payment, error)
}

type Server struct {
	paymentv1.UnimplementedPaymentGatewayServer
	usecase PaymentUseCase
}

func NewServer(usecase PaymentUseCase) *Server {
	return &Server{usecase: usecase}
}

func (s *Server) CreatePayment(ctx context.Context, req *paymentv1.CreatePaymentRequest) (*paymentv1.PaymentResponse, error) {
	payment, err := s.usecase.CreatePayment(ctx, req.GetAmount(), req.GetCurrency(), req.GetIdempotencyKey())
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &paymentv1.PaymentResponse{Payment: toProtoPayment(payment)}, nil
}

func (s *Server) ConfirmPayment(ctx context.Context, req *paymentv1.ConfirmPaymentRequest) (*paymentv1.PaymentResponse, error) {
	payment, err := s.usecase.ConfirmPayment(ctx, req.GetPaymentId(), req.GetIdempotencyKey())
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &paymentv1.PaymentResponse{Payment: toProtoPayment(payment)}, nil
}

func (s *Server) RefundPayment(ctx context.Context, req *paymentv1.RefundPaymentRequest) (*paymentv1.PaymentResponse, error) {
	payment, err := s.usecase.RefundPayment(ctx, req.GetPaymentId(), req.GetAmount(), req.GetIdempotencyKey())
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &paymentv1.PaymentResponse{Payment: toProtoPayment(payment)}, nil
}

func (s *Server) GetPayment(ctx context.Context, req *paymentv1.GetPaymentRequest) (*paymentv1.PaymentResponse, error) {
	payment, err := s.usecase.GetPayment(ctx, req.GetPaymentId())
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &paymentv1.PaymentResponse{Payment: toProtoPayment(payment)}, nil
}

func toGRPCError(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrPaymentNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrIdempotencyConflict):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, domain.ErrPaymentAlreadyFinal), errors.Is(err, domain.ErrPaymentNotSucceeded):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}

func toProtoPayment(payment *domain.Payment) *paymentv1.Payment {
	if payment == nil {
		return nil
	}

	transactions := make([]*paymentv1.Transaction, 0, len(payment.Transactions))
	for _, transaction := range payment.Transactions {
		transactions = append(transactions, toProtoTransaction(transaction))
	}

	return &paymentv1.Payment{
		Id:           payment.ID,
		Amount:       payment.Amount,
		Currency:     payment.Currency,
		Status:       toProtoPaymentStatus(payment.Status),
		CreatedAt:    timestamppb.New(payment.CreatedAt),
		UpdatedAt:    timestamppb.New(payment.UpdatedAt),
		Transactions: transactions,
	}
}

func toProtoTransaction(transaction domain.Transaction) *paymentv1.Transaction {
	return &paymentv1.Transaction{
		Id:        transaction.ID,
		PaymentId: transaction.PaymentID,
		Type:      toProtoTransactionType(transaction.Type),
		Amount:    transaction.Amount,
		Status:    toProtoPaymentStatus(transaction.Status),
		CreatedAt: timestamppb.New(transaction.CreatedAt),
	}
}

func toProtoPaymentStatus(status domain.PaymentStatus) paymentv1.PaymentStatus {
	switch status {
	case domain.PaymentStatusPending:
		return paymentv1.PaymentStatus_PAYMENT_STATUS_PENDING
	case domain.PaymentStatusSucceeded:
		return paymentv1.PaymentStatus_PAYMENT_STATUS_SUCCEEDED
	case domain.PaymentStatusRefunded:
		return paymentv1.PaymentStatus_PAYMENT_STATUS_REFUNDED
	case domain.PaymentStatusFailed:
		return paymentv1.PaymentStatus_PAYMENT_STATUS_FAILED
	default:
		return paymentv1.PaymentStatus_PAYMENT_STATUS_UNSPECIFIED
	}
}

func toProtoTransactionType(typ domain.TransactionType) paymentv1.TransactionType {
	switch typ {
	case domain.TransactionTypePayment:
		return paymentv1.TransactionType_TRANSACTION_TYPE_PAYMENT
	case domain.TransactionTypeRefund:
		return paymentv1.TransactionType_TRANSACTION_TYPE_REFUND
	default:
		return paymentv1.TransactionType_TRANSACTION_TYPE_UNSPECIFIED
	}
}
