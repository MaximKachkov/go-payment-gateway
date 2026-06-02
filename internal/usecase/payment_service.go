package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/maxotik/go-payment-gateway/internal/domain"
)

const (
	endpointCreatePayment  = "CreatePayment"
	endpointConfirmPayment = "ConfirmPayment"
	endpointRefundPayment  = "RefundPayment"
)

type PaymentRepository interface {
	RunInTransaction(ctx context.Context, fn func(ctx context.Context, tx PaymentTx) error) error
	GetPayment(ctx context.Context, id string) (*domain.Payment, error)
}

type PaymentTx interface {
	GetIdempotency(ctx context.Context, endpoint, key string) (*IdempotencyRecord, error)
	SaveIdempotency(ctx context.Context, endpoint, key, requestHash string, response *domain.Payment) error
	CreatePayment(ctx context.Context, amount int64, currency string) (*domain.Payment, error)
	CreateTransaction(ctx context.Context, paymentID string, typ domain.TransactionType, amount int64, status domain.PaymentStatus) (*domain.Transaction, error)
	GetPaymentForUpdate(ctx context.Context, id string) (*domain.Payment, error)
	UpdatePaymentStatus(ctx context.Context, id string, status domain.PaymentStatus) (*domain.Payment, error)
	UpdateTransactionStatus(ctx context.Context, paymentID string, typ domain.TransactionType, status domain.PaymentStatus) error
}

type IdempotencyRecord struct {
	RequestHash string
	Response    *domain.Payment
}

type PaymentService struct {
	repo PaymentRepository
}

func NewPaymentService(repo PaymentRepository) *PaymentService {
	return &PaymentService{repo: repo}
}

func (s *PaymentService) CreatePayment(ctx context.Context, amount int64, currency, idempotencyKey string) (*domain.Payment, error) {
	currency = domain.NormalizeCurrency(currency)
	if err := domain.ValidateAmount(amount); err != nil {
		return nil, err
	}
	if err := domain.ValidateCurrency(currency); err != nil {
		return nil, err
	}
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return nil, err
	}

	requestHash, err := hashRequest(map[string]any{
		"amount":   amount,
		"currency": currency,
	})
	if err != nil {
		return nil, err
	}

	var payment *domain.Payment
	err = s.repo.RunInTransaction(ctx, func(ctx context.Context, tx PaymentTx) error {
		existing, err := tx.GetIdempotency(ctx, endpointCreatePayment, idempotencyKey)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.RequestHash != requestHash {
				return domain.ErrIdempotencyConflict
			}
			payment = existing.Response
			return nil
		}

		created, err := tx.CreatePayment(ctx, amount, currency)
		if err != nil {
			return err
		}
		transaction, err := tx.CreateTransaction(ctx, created.ID, domain.TransactionTypePayment, amount, domain.PaymentStatusPending)
		if err != nil {
			return err
		}
		created.Transactions = []domain.Transaction{*transaction}

		if err := tx.SaveIdempotency(ctx, endpointCreatePayment, idempotencyKey, requestHash, created); err != nil {
			return err
		}

		payment = created
		return nil
	})
	if err != nil {
		return nil, err
	}
	return payment, nil
}

func (s *PaymentService) ConfirmPayment(ctx context.Context, paymentID, idempotencyKey string) (*domain.Payment, error) {
	if err := domain.ValidateID(paymentID); err != nil {
		return nil, err
	}
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return nil, err
	}

	requestHash, err := hashRequest(map[string]any{"payment_id": paymentID})
	if err != nil {
		return nil, err
	}

	var payment *domain.Payment
	err = s.repo.RunInTransaction(ctx, func(ctx context.Context, tx PaymentTx) error {
		existing, err := tx.GetIdempotency(ctx, endpointConfirmPayment, idempotencyKey)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.RequestHash != requestHash {
				return domain.ErrIdempotencyConflict
			}
			payment = existing.Response
			return nil
		}

		current, err := tx.GetPaymentForUpdate(ctx, paymentID)
		if err != nil {
			return err
		}
		if current.Status == domain.PaymentStatusRefunded || current.Status == domain.PaymentStatusFailed {
			return domain.ErrPaymentAlreadyFinal
		}
		if current.Status == domain.PaymentStatusSucceeded {
			payment = current
		} else {
			if err := tx.UpdateTransactionStatus(ctx, paymentID, domain.TransactionTypePayment, domain.PaymentStatusSucceeded); err != nil {
				return err
			}
			payment, err = tx.UpdatePaymentStatus(ctx, paymentID, domain.PaymentStatusSucceeded)
			if err != nil {
				return err
			}
		}

		if err := tx.SaveIdempotency(ctx, endpointConfirmPayment, idempotencyKey, requestHash, payment); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return payment, nil
}

func (s *PaymentService) RefundPayment(ctx context.Context, paymentID string, amount int64, idempotencyKey string) (*domain.Payment, error) {
	if err := domain.ValidateID(paymentID); err != nil {
		return nil, err
	}
	if err := domain.ValidateAmount(amount); err != nil {
		return nil, err
	}
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return nil, err
	}

	requestHash, err := hashRequest(map[string]any{
		"amount":     amount,
		"payment_id": paymentID,
	})
	if err != nil {
		return nil, err
	}

	var payment *domain.Payment
	err = s.repo.RunInTransaction(ctx, func(ctx context.Context, tx PaymentTx) error {
		existing, err := tx.GetIdempotency(ctx, endpointRefundPayment, idempotencyKey)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.RequestHash != requestHash {
				return domain.ErrIdempotencyConflict
			}
			payment = existing.Response
			return nil
		}

		current, err := tx.GetPaymentForUpdate(ctx, paymentID)
		if err != nil {
			return err
		}
		if current.Status != domain.PaymentStatusSucceeded {
			return domain.ErrPaymentNotSucceeded
		}
		if amount > current.Amount {
			return fmt.Errorf("%w: refund amount is greater than payment amount", domain.ErrInvalidInput)
		}

		if _, err := tx.CreateTransaction(ctx, paymentID, domain.TransactionTypeRefund, amount, domain.PaymentStatusRefunded); err != nil {
			return err
		}
		payment, err = tx.UpdatePaymentStatus(ctx, paymentID, domain.PaymentStatusRefunded)
		if err != nil {
			return err
		}

		if err := tx.SaveIdempotency(ctx, endpointRefundPayment, idempotencyKey, requestHash, payment); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return payment, nil
}

func (s *PaymentService) GetPayment(ctx context.Context, paymentID string) (*domain.Payment, error) {
	if err := domain.ValidateID(paymentID); err != nil {
		return nil, err
	}
	return s.repo.GetPayment(ctx, paymentID)
}

func validateIdempotencyKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return domain.ErrInvalidInput
	}
	return nil
}

func hashRequest(value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func IsDomainError(err error, target error) bool {
	return errors.Is(err, target)
}
