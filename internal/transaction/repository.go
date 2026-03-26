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
	db *pgxpool.Pool // Connection pool which maintains a set of connections, whenever there is a request, lend one conn to that request
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Create inserts a new transaction row.
// If idempotency_key already exists (duplicate request), the INSERT is silently skipped
// because the DB has ON CONFLICT DO NOTHING via a UNIQUE constraint.
// The caller should treat a no-op insert as success (idempotent behaviour).
// Create inserts a transaction and populates t.ID and t.CreatedAt from the DB.
// On idempotency_key conflict the INSERT is skipped (DO NOTHING), so t.ID will
// remain empty — the caller can treat this as a success (duplicate request).
func (r *Repository) Create(ctx context.Context, tx pgx.Tx, t *Transaction) error {
	err := tx.QueryRow(ctx, `
		INSERT INTO transactions
			(idempotency_key, type, status, amount, currency,
			 from_user_id, to_user_id, streamer_id, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id, created_at
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
	).Scan(&t.ID, &t.CreatedAt)

	// pgx.ErrNoRows means ON CONFLICT DO NOTHING fired (duplicate idempotency_key).
	// Treat as success — the original transaction already exists.
	if err == pgx.ErrNoRows {
		return nil
	}
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
