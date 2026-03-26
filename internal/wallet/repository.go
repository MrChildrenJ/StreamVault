package wallet

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

func (r *Repository) GetByUserID(ctx context.Context, userID string) (*Wallet, error) {
	w := &Wallet{}
	err := r.db.QueryRow(ctx, `
		SELECT id, user_id, balance, currency, version, created_at, updated_at
		FROM wallets WHERE user_id = $1
	`, userID).Scan(
		&w.ID, &w.UserID, &w.Balance, &w.Currency,
		&w.Version, &w.CreatedAt, &w.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("wallet get: %w", err)
	}
	return w, nil
}

// Credit adds amount to the wallet within an existing DB transaction.
// No version check needed — credits cannot cause negative balance.
func (r *Repository) Credit(ctx context.Context, tx pgx.Tx, userID string, amount int64) error {
	tag, err := tx.Exec(ctx, `
		UPDATE wallets
		SET balance = balance + $1, version = version + 1, updated_at = NOW()
		WHERE user_id = $2
	`, amount, userID)
	if err != nil {
		return fmt.Errorf("wallet credit: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Debit subtracts amount using optimistic locking.
// Returns ErrVersionConflict when another writer modified the wallet concurrently
// (the caller should re-read the wallet and retry).
// The balance check (balance >= amount) must be done by the caller before invoking.
func (r *Repository) Debit(ctx context.Context, tx pgx.Tx, userID string, amount int64, version int64) error {
	tag, err := tx.Exec(ctx, `
		UPDATE wallets
		SET balance = balance - $1, version = version + 1, updated_at = NOW()
		WHERE user_id = $2 AND version = $3
	`, amount, userID, version)
	if err != nil {
		return fmt.Errorf("wallet debit: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// version mismatch: a concurrent writer updated the row between our read and this write
		return ErrVersionConflict
	}
	return nil
}
