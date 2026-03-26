package bits

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetByUserID(ctx context.Context, userID string) (*Balance, error) {
	b := &Balance{}
	err := r.db.QueryRow(ctx, `
		SELECT id, user_id, balance, version, created_at, updated_at
		FROM virtual_currency_balances WHERE user_id = $1
	`, userID).Scan(&b.ID, &b.UserID, &b.Balance, &b.Version, &b.CreatedAt, &b.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("bits get: %w", err)
	}
	return b, nil
}

// Credit adds bits within an existing DB transaction. No version check needed.
func (r *Repository) Credit(ctx context.Context, tx pgx.Tx, userID string, amount int64) error {
	tag, err := tx.Exec(ctx, `
		UPDATE virtual_currency_balances
		SET balance = balance + $1, version = version + 1, updated_at = NOW()
		WHERE user_id = $2
	`, amount, userID)
	if err != nil {
		return fmt.Errorf("bits credit: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Debit subtracts bits using optimistic locking.
// Returns ErrVersionConflict when a concurrent writer modified the row.
// The caller must verify balance >= amount before calling.
func (r *Repository) Debit(ctx context.Context, tx pgx.Tx, userID string, amount int64, version int64) error {
	tag, err := tx.Exec(ctx, `
		UPDATE virtual_currency_balances
		SET balance = balance - $1, version = version + 1, updated_at = NOW()
		WHERE user_id = $2 AND version = $3
	`, amount, userID, version)
	if err != nil {
		return fmt.Errorf("bits debit: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}
