package dashboard

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RevenueSummary struct {
	StreamerID          string
	TotalRevenue        int64
	SubscriptionRevenue int64
	DonationRevenue     int64
	BitsRevenue         int64 // in bits
	UpdatedAt           time.Time
}

type TxRow struct {
	ID         string
	Type       string
	Status     string
	Amount     int64
	Currency   string
	FromUserID *string
	CreatedAt  time.Time
}

var ErrNotFound = errors.New("revenue summary not found")

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetRevenueSummary(ctx context.Context, streamerID string) (*RevenueSummary, error) {
	s := &RevenueSummary{}
	err := r.db.QueryRow(ctx, `
		SELECT streamer_id, total_revenue, subscription_revenue, donation_revenue, bits_revenue, updated_at
		FROM streamer_revenue_summary WHERE streamer_id = $1
	`, streamerID).Scan(
		&s.StreamerID, &s.TotalRevenue,
		&s.SubscriptionRevenue, &s.DonationRevenue,
		&s.BitsRevenue, &s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("revenue summary get: %w", err)
	}
	return s, nil
}

// ListTransactions returns up to `limit` transactions for a streamer, ordered newest-first.
// Cursor-based pagination: pass the created_at of the last item as `before` for the next page.
func (r *Repository) ListTransactions(ctx context.Context, streamerID string, limit int, before time.Time) ([]TxRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, type, status, amount, currency, from_user_id, created_at
		FROM transactions
		WHERE streamer_id = $1 AND created_at < $2
		ORDER BY created_at DESC
		LIMIT $3
	`, streamerID, before, limit)
	if err != nil {
		return nil, fmt.Errorf("transactions list: %w", err)
	}
	defer rows.Close()

	var result []TxRow
	for rows.Next() {
		var t TxRow
		if err := rows.Scan(&t.ID, &t.Type, &t.Status, &t.Amount, &t.Currency, &t.FromUserID, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("transactions scan: %w", err)
		}
		result = append(result, t)
	}
	return result, rows.Err()
}
