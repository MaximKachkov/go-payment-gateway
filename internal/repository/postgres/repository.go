package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maxotik/go-payment-gateway/internal/domain"
	"github.com/maxotik/go-payment-gateway/internal/usecase"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) RunInTransaction(ctx context.Context, fn func(ctx context.Context, tx usecase.PaymentTx) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}

	repo := &txRepository{tx: tx}
	if err := fn(ctx, repo); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) GetPayment(ctx context.Context, id string) (*domain.Payment, error) {
	return loadPayment(ctx, r.pool, id, false)
}

type txRepository struct {
	tx pgx.Tx
}

func (r *txRepository) GetIdempotency(ctx context.Context, endpoint, key string) (*usecase.IdempotencyRecord, error) {
	const query = `
		SELECT request_hash, response_json
		FROM idempotency_keys
		WHERE endpoint = $1 AND key = $2
		FOR UPDATE`

	var requestHash string
	var responseJSON []byte
	if err := r.tx.QueryRow(ctx, query, endpoint, key).Scan(&requestHash, &responseJSON); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	var payment domain.Payment
	if err := json.Unmarshal(responseJSON, &payment); err != nil {
		return nil, err
	}

	return &usecase.IdempotencyRecord{
		RequestHash: requestHash,
		Response:    &payment,
	}, nil
}

func (r *txRepository) SaveIdempotency(ctx context.Context, endpoint, key, requestHash string, response *domain.Payment) error {
	responseJSON, err := json.Marshal(response)
	if err != nil {
		return err
	}

	const query = `
		INSERT INTO idempotency_keys (key, endpoint, request_hash, response_json)
		VALUES ($1, $2, $3, $4)`

	_, err = r.tx.Exec(ctx, query, key, endpoint, requestHash, responseJSON)
	return err
}

func (r *txRepository) CreatePayment(ctx context.Context, amount int64, currency string) (*domain.Payment, error) {
	const query = `
		INSERT INTO payments (amount, currency, status)
		VALUES ($1, $2, $3)
		RETURNING id::text, amount, currency, status, created_at, updated_at`

	var payment domain.Payment
	var status string
	if err := r.tx.QueryRow(ctx, query, amount, currency, domain.PaymentStatusPending).Scan(
		&payment.ID,
		&payment.Amount,
		&payment.Currency,
		&status,
		&payment.CreatedAt,
		&payment.UpdatedAt,
	); err != nil {
		return nil, err
	}

	payment.Status = domain.PaymentStatus(status)
	return &payment, nil
}

func (r *txRepository) CreateTransaction(ctx context.Context, paymentID string, typ domain.TransactionType, amount int64, status domain.PaymentStatus) (*domain.Transaction, error) {
	const query = `
		INSERT INTO transactions (payment_id, type, amount, status)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, payment_id::text, type, amount, status, created_at`

	var transaction domain.Transaction
	var transactionType string
	var transactionStatus string
	if err := r.tx.QueryRow(ctx, query, paymentID, typ, amount, status).Scan(
		&transaction.ID,
		&transaction.PaymentID,
		&transactionType,
		&transaction.Amount,
		&transactionStatus,
		&transaction.CreatedAt,
	); err != nil {
		return nil, err
	}

	transaction.Type = domain.TransactionType(transactionType)
	transaction.Status = domain.PaymentStatus(transactionStatus)
	return &transaction, nil
}

func (r *txRepository) GetPaymentForUpdate(ctx context.Context, id string) (*domain.Payment, error) {
	return loadPayment(ctx, r.tx, id, true)
}

func (r *txRepository) UpdatePaymentStatus(ctx context.Context, id string, status domain.PaymentStatus) (*domain.Payment, error) {
	const query = `
		UPDATE payments
		SET status = $2, updated_at = now()
		WHERE id = $1
		RETURNING id::text, amount, currency, status, created_at, updated_at`

	var payment domain.Payment
	var paymentStatus string
	if err := r.tx.QueryRow(ctx, query, id, status).Scan(
		&payment.ID,
		&payment.Amount,
		&payment.Currency,
		&paymentStatus,
		&payment.CreatedAt,
		&payment.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrPaymentNotFound
		}
		return nil, err
	}

	payment.Status = domain.PaymentStatus(paymentStatus)
	transactions, err := loadTransactions(ctx, r.tx, id)
	if err != nil {
		return nil, err
	}
	payment.Transactions = transactions
	return &payment, nil
}

func (r *txRepository) UpdateTransactionStatus(ctx context.Context, paymentID string, typ domain.TransactionType, status domain.PaymentStatus) error {
	const query = `
		UPDATE transactions
		SET status = $3
		WHERE payment_id = $1 AND type = $2`

	result, err := r.tx.Exec(ctx, query, paymentID, typ, status)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return domain.ErrPaymentNotFound
	}
	return nil
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func loadPayment(ctx context.Context, q querier, id string, forUpdate bool) (*domain.Payment, error) {
	query := `
		SELECT id::text, amount, currency, status, created_at, updated_at
		FROM payments
		WHERE id = $1`
	if forUpdate {
		query += " FOR UPDATE"
	}

	var payment domain.Payment
	var status string
	if err := q.QueryRow(ctx, query, id).Scan(
		&payment.ID,
		&payment.Amount,
		&payment.Currency,
		&status,
		&payment.CreatedAt,
		&payment.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrPaymentNotFound
		}
		return nil, err
	}

	payment.Status = domain.PaymentStatus(status)
	transactions, err := loadTransactions(ctx, q, id)
	if err != nil {
		return nil, err
	}
	payment.Transactions = transactions
	return &payment, nil
}

func loadTransactions(ctx context.Context, q querier, paymentID string) ([]domain.Transaction, error) {
	const query = `
		SELECT id::text, payment_id::text, type, amount, status, created_at
		FROM transactions
		WHERE payment_id = $1
		ORDER BY created_at ASC`

	rows, err := q.Query(ctx, query, paymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []domain.Transaction
	for rows.Next() {
		var transaction domain.Transaction
		var typ string
		var status string
		if err := rows.Scan(
			&transaction.ID,
			&transaction.PaymentID,
			&typ,
			&transaction.Amount,
			&status,
			&transaction.CreatedAt,
		); err != nil {
			return nil, err
		}

		transaction.Type = domain.TransactionType(typ)
		transaction.Status = domain.PaymentStatus(status)
		transactions = append(transactions, transaction)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read transactions: %w", err)
	}
	return transactions, nil
}
