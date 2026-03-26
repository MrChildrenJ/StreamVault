package transaction

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("transaction not found")

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Create inserts a new transaction row.
// If idempotency_key already exists (duplicate request), the INSERT is silently skipped
// because the DB has ON CONFLICT DO NOTHING via a UNIQUE constraint.
// The caller should treat a no-op insert as success (idempotent behaviour).
func (r *Repository) Create(ctx context.Context, tx pgx.Tx, t *Transaction) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO transactions
			(idempotency_key, type, status, amount, currency,
			 from_user_id, to_user_id, streamer_id, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (idempotency_key) DO NOTHING
	`,
		t.IdempotencyKey,
		t.Type,
		t.Status,
		t.Amount,
		t.Currency,
		t.FromUserID,
		t.ToUserID,
		t.StreamerID,
		t.Metadata,
	)
	if err != nil {
		return fmt.Errorf("transaction insert: %w", err)
	}
	return nil
}

// GetByIDempotencyKey fetches a transaction by its idempotency key.
func (r *Repository) GetByIdempotencyKey(ctx context.Context, key string) (*Transaction, error) {
	t := &Transaction{}
	err := r.db.QueryRow(ctx, `
		SELECT id, idempotency_key, type, status, amount, currency,
		       from_user_id, to_user_id, streamer_id, metadata, created_at
		FROM transactions WHERE idempotency_key = $1
	`, key).Scan(
		&t.ID, &t.IdempotencyKey, &t.Type, &t.Status,
		&t.Amount, &t.Currency,
		&t.FromUserID, &t.ToUserID, &t.StreamerID,
		&t.Metadata, &t.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("transaction get: %w", err)
	}
	return t, nil
}
