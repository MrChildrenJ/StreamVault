package payout

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MrChildrenJ/streamvault/internal/db"
	"github.com/MrChildrenJ/streamvault/internal/transaction"
)

var ErrInsufficientRevenue = errors.New("insufficient streamer revenue")

type Service struct {
	db  *pgxpool.Pool
	txs *transaction.Repository
}

func NewService(db *pgxpool.Pool, txs *transaction.Repository) *Service {
	return &Service{db: db, txs: txs}
}

// RequestPayout lets a streamer withdraw earned revenue.
// Atomically: decrement streamer_revenue_summary + insert pending payout transaction.
// Status stays 'pending' until an external payment processor confirms delivery.
func (s *Service) RequestPayout(ctx context.Context, streamerID string, amountCents int64, idempotencyKey string) error {
	return db.WithTx(ctx, s.db, func(tx pgx.Tx) error {
		// Decrement total_revenue with a balance check in the same UPDATE.
		tag, err := tx.Exec(ctx, `
			UPDATE streamer_revenue_summary
			SET total_revenue = total_revenue - $1, updated_at = NOW()
			WHERE streamer_id = $2 AND total_revenue >= $1
		`, amountCents, streamerID)
		if err != nil {
			return fmt.Errorf("payout deduct: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrInsufficientRevenue
		}

		return s.txs.Create(ctx, tx, &transaction.Transaction{
			IdempotencyKey: idempotencyKey,
			Type:           transaction.TypePayout,
			Status:         transaction.StatusPending, // confirmed by payment processor later
			Amount:         amountCents,
			Currency:       "USD",
			FromUserID:     &streamerID, // revenue leaving the streamer's account
		})
	})
}
