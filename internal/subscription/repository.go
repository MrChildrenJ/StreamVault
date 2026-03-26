package subscription

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetTierByID(ctx context.Context, tierID string) (*Tier, error) {
	t := &Tier{}
	err := r.db.QueryRow(ctx, `
		SELECT id, streamer_id, name, price, currency, created_at
		FROM subscription_tiers WHERE id = $1
	`, tierID).Scan(&t.ID, &t.StreamerID, &t.Name, &t.Price, &t.Currency, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTierNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("tier get: %w", err)
	}
	return t, nil
}

// Create inserts a new subscription row within an existing DB transaction.
func (r *Repository) Create(ctx context.Context, tx pgx.Tx, sub *Subscription) error {
	err := tx.QueryRow(ctx, `
		INSERT INTO subscriptions
			(subscriber_id, streamer_id, tier_id, status, gifted_by, transaction_id, started_at, expires_at)
		VALUES ($1, $2, $3, 'active', $4, $5, $6, $7)
		RETURNING id, created_at
	`,
		sub.SubscriberID,
		sub.StreamerID,
		sub.TierID,
		sub.GiftedBy,
		sub.TransactionID,
		sub.StartedAt,
		sub.ExpiresAt,
	).Scan(&sub.ID, &sub.CreatedAt)
	if err != nil {
		return fmt.Errorf("subscription create: %w", err)
	}
	return nil
}

// ExpireOld marks all subscriptions past their expires_at as 'expired'.
// Called by the background expiry worker.
func (r *Repository) ExpireOld(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE subscriptions SET status = 'expired'
		WHERE status = 'active' AND expires_at < $1
	`, time.Now())
	if err != nil {
		return 0, fmt.Errorf("expire subscriptions: %w", err)
	}
	return tag.RowsAffected(), nil
}
