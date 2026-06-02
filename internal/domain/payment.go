package domain

import (
	"errors"
	"strings"
	"time"
)

type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "PENDING"
	PaymentStatusSucceeded PaymentStatus = "SUCCEEDED"
	PaymentStatusRefunded  PaymentStatus = "REFUNDED"
	PaymentStatusFailed    PaymentStatus = "FAILED"
)

type TransactionType string

const (
	TransactionTypePayment TransactionType = "PAYMENT"
	TransactionTypeRefund  TransactionType = "REFUND"
)

var (
	ErrInvalidInput        = errors.New("invalid input")
	ErrPaymentNotFound     = errors.New("payment not found")
	ErrPaymentAlreadyFinal = errors.New("payment is already in a final state")
	ErrPaymentNotSucceeded = errors.New("payment is not succeeded")
	ErrIdempotencyConflict = errors.New("idempotency key was reused with a different request")
)

type Payment struct {
	ID           string
	Amount       int64
	Currency     string
	Status       PaymentStatus
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Transactions []Transaction
}

type Transaction struct {
	ID        string
	PaymentID string
	Type      TransactionType
	Amount    int64
	Status    PaymentStatus
	CreatedAt time.Time
}

func NormalizeCurrency(currency string) string {
	return strings.ToUpper(strings.TrimSpace(currency))
}

func ValidateAmount(amount int64) error {
	if amount <= 0 {
		return ErrInvalidInput
	}
	return nil
}

func ValidateCurrency(currency string) error {
	if len(NormalizeCurrency(currency)) != 3 {
		return ErrInvalidInput
	}
	return nil
}

func ValidateID(value string) error {
	if strings.TrimSpace(value) == "" {
		return ErrInvalidInput
	}
	return nil
}
