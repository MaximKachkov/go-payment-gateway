package usecase

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/maxotik/go-payment-gateway/internal/domain"
)

func TestCreatePaymentIsIdempotent(t *testing.T) {
	repo := newFakeRepo()
	service := NewPaymentService(repo)

	first, err := service.CreatePayment(context.Background(), 1999, "usd", "create-001")
	if err != nil {
		t.Fatalf("create first payment: %v", err)
	}

	second, err := service.CreatePayment(context.Background(), 1999, "USD", "create-001")
	if err != nil {
		t.Fatalf("create second payment: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("expected same payment id, got %q and %q", first.ID, second.ID)
	}
	if len(repo.payments) != 1 {
		t.Fatalf("expected one stored payment, got %d", len(repo.payments))
	}
}

func TestCreatePaymentRejectsIdempotencyConflict(t *testing.T) {
	service := NewPaymentService(newFakeRepo())

	if _, err := service.CreatePayment(context.Background(), 1999, "USD", "create-001"); err != nil {
		t.Fatalf("create first payment: %v", err)
	}

	_, err := service.CreatePayment(context.Background(), 2999, "USD", "create-001")
	if !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
}

func TestRefundRequiresSucceededPayment(t *testing.T) {
	service := NewPaymentService(newFakeRepo())

	payment, err := service.CreatePayment(context.Background(), 1999, "USD", "create-001")
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}

	_, err = service.RefundPayment(context.Background(), payment.ID, 1999, "refund-001")
	if !errors.Is(err, domain.ErrPaymentNotSucceeded) {
		t.Fatalf("expected payment not succeeded error, got %v", err)
	}
}

func TestConfirmPaymentUpdatesPaymentTransaction(t *testing.T) {
	service := NewPaymentService(newFakeRepo())

	payment, err := service.CreatePayment(context.Background(), 1999, "USD", "create-001")
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}

	confirmed, err := service.ConfirmPayment(context.Background(), payment.ID, "confirm-001")
	if err != nil {
		t.Fatalf("confirm payment: %v", err)
	}

	if confirmed.Status != domain.PaymentStatusSucceeded {
		t.Fatalf("expected payment status %q, got %q", domain.PaymentStatusSucceeded, confirmed.Status)
	}
	if len(confirmed.Transactions) != 1 {
		t.Fatalf("expected one transaction, got %d", len(confirmed.Transactions))
	}
	if confirmed.Transactions[0].Status != domain.PaymentStatusSucceeded {
		t.Fatalf("expected transaction status %q, got %q", domain.PaymentStatusSucceeded, confirmed.Transactions[0].Status)
	}
}

type fakeRepo struct {
	payments    map[string]*domain.Payment
	idempotency map[string]*IdempotencyRecord
	nextID      int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		payments:    map[string]*domain.Payment{},
		idempotency: map[string]*IdempotencyRecord{},
	}
}

func (r *fakeRepo) RunInTransaction(ctx context.Context, fn func(ctx context.Context, tx PaymentTx) error) error {
	return fn(ctx, &fakeTx{repo: r})
}

func (r *fakeRepo) GetPayment(_ context.Context, id string) (*domain.Payment, error) {
	payment, ok := r.payments[id]
	if !ok {
		return nil, domain.ErrPaymentNotFound
	}
	return clonePayment(payment), nil
}

type fakeTx struct {
	repo *fakeRepo
}

func (tx *fakeTx) GetIdempotency(_ context.Context, endpoint, key string) (*IdempotencyRecord, error) {
	record, ok := tx.repo.idempotency[idempotencyMapKey(endpoint, key)]
	if !ok {
		return nil, nil
	}
	return &IdempotencyRecord{
		RequestHash: record.RequestHash,
		Response:    clonePayment(record.Response),
	}, nil
}

func (tx *fakeTx) SaveIdempotency(_ context.Context, endpoint, key, requestHash string, response *domain.Payment) error {
	tx.repo.idempotency[idempotencyMapKey(endpoint, key)] = &IdempotencyRecord{
		RequestHash: requestHash,
		Response:    clonePayment(response),
	}
	return nil
}

func (tx *fakeTx) CreatePayment(_ context.Context, amount int64, currency string) (*domain.Payment, error) {
	tx.repo.nextID++
	now := time.Now().UTC()
	payment := &domain.Payment{
		ID:        fmt.Sprintf("payment-%d", tx.repo.nextID),
		Amount:    amount,
		Currency:  currency,
		Status:    domain.PaymentStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	tx.repo.payments[payment.ID] = clonePayment(payment)
	return clonePayment(payment), nil
}

func (tx *fakeTx) CreateTransaction(_ context.Context, paymentID string, typ domain.TransactionType, amount int64, status domain.PaymentStatus) (*domain.Transaction, error) {
	payment, ok := tx.repo.payments[paymentID]
	if !ok {
		return nil, domain.ErrPaymentNotFound
	}

	transaction := domain.Transaction{
		ID:        fmt.Sprintf("transaction-%d", len(payment.Transactions)+1),
		PaymentID: paymentID,
		Type:      typ,
		Amount:    amount,
		Status:    status,
		CreatedAt: time.Now().UTC(),
	}
	payment.Transactions = append(payment.Transactions, transaction)
	return &transaction, nil
}

func (tx *fakeTx) GetPaymentForUpdate(_ context.Context, id string) (*domain.Payment, error) {
	payment, ok := tx.repo.payments[id]
	if !ok {
		return nil, domain.ErrPaymentNotFound
	}
	return clonePayment(payment), nil
}

func (tx *fakeTx) UpdatePaymentStatus(_ context.Context, id string, status domain.PaymentStatus) (*domain.Payment, error) {
	payment, ok := tx.repo.payments[id]
	if !ok {
		return nil, domain.ErrPaymentNotFound
	}

	payment.Status = status
	payment.UpdatedAt = time.Now().UTC()
	return clonePayment(payment), nil
}

func (tx *fakeTx) UpdateTransactionStatus(_ context.Context, paymentID string, typ domain.TransactionType, status domain.PaymentStatus) error {
	payment, ok := tx.repo.payments[paymentID]
	if !ok {
		return domain.ErrPaymentNotFound
	}

	updated := false
	for i := range payment.Transactions {
		if payment.Transactions[i].Type == typ {
			payment.Transactions[i].Status = status
			updated = true
		}
	}
	if !updated {
		return domain.ErrPaymentNotFound
	}
	return nil
}

func idempotencyMapKey(endpoint, key string) string {
	return endpoint + ":" + key
}

func clonePayment(payment *domain.Payment) *domain.Payment {
	if payment == nil {
		return nil
	}

	clone := *payment
	clone.Transactions = append([]domain.Transaction(nil), payment.Transactions...)
	return &clone
}
