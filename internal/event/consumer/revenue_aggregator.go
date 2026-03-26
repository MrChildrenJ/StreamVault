package consumer

import (
	"context"
	"encoding/json"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/segmentio/kafka-go"

	"github.com/MrChildrenJ/streamvault/internal/event"
)

// RevenueAggregator consumes domain events and updates streamer_revenue_summary.
// This is the async half of the eventual-consistency model:
// the write path (debit + transaction insert) is strongly consistent in PostgreSQL,
// while the revenue summary is updated here, after Kafka delivery.
type RevenueAggregator struct {
	db      *pgxpool.Pool
	readers []*kafka.Reader
}

func NewRevenueAggregator(db *pgxpool.Pool, broker string) *RevenueAggregator {
	topics := []string{
		event.TopicSubscriptionCreated,
		event.TopicDonationReceived,
		event.TopicBitsCheered,
	}
	readers := make([]*kafka.Reader, len(topics))
	for i, topic := range topics {
		readers[i] = kafka.NewReader(kafka.ReaderConfig{
			Brokers:  []string{broker},
			GroupID:  "revenue-aggregator",
			Topic:    topic,
			MinBytes: 1,
			MaxBytes: 1e6,
		})
	}
	return &RevenueAggregator{db: db, readers: readers}
}

// Start launches one goroutine per topic. Blocks until ctx is cancelled.
func (a *RevenueAggregator) Start(ctx context.Context) {
	for _, r := range a.readers {
		go a.consume(ctx, r)
	}
}

func (a *RevenueAggregator) Close() {
	for _, r := range a.readers {
		r.Close()
	}
}

func (a *RevenueAggregator) consume(ctx context.Context, r *kafka.Reader) {
	for {
		msg, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return // shutdown
			}
			log.Printf("revenue-aggregator fetch [%s]: %v", r.Config().Topic, err)
			continue
		}

		if err := a.handle(ctx, msg); err != nil {
			log.Printf("revenue-aggregator handle [%s]: %v", r.Config().Topic, err)
			// Do not commit — message will be redelivered (at-least-once processing).
			continue
		}

		if err := r.CommitMessages(ctx, msg); err != nil {
			log.Printf("revenue-aggregator commit [%s]: %v", r.Config().Topic, err)
		}
	}
}

func (a *RevenueAggregator) handle(ctx context.Context, msg kafka.Message) error {
	switch msg.Topic {
	case event.TopicSubscriptionCreated:
		var e event.SubscriptionCreatedEvent
		if err := json.Unmarshal(msg.Value, &e); err != nil {
			return err
		}
		return a.upsertRevenue(ctx, e.StreamerID, e.AmountCents, "subscription_revenue")

	case event.TopicDonationReceived:
		var e event.DonationReceivedEvent
		if err := json.Unmarshal(msg.Value, &e); err != nil {
			return err
		}
		return a.upsertRevenue(ctx, e.StreamerID, e.AmountCents, "donation_revenue")

	case event.TopicBitsCheered:
		var e event.BitsCheeredEvent
		if err := json.Unmarshal(msg.Value, &e); err != nil {
			return err
		}
		return a.upsertRevenue(ctx, e.StreamerID, e.Bits, "bits_revenue")
	}
	return nil
}

// upsertRevenue atomically increments the specific revenue column and total_revenue.
// Uses INSERT ... ON CONFLICT DO UPDATE so the first event for a streamer creates the row.
func (a *RevenueAggregator) upsertRevenue(ctx context.Context, streamerID string, amount int64, column string) error {
	// column is an internal constant, not user input — safe to interpolate.
	query := `
		INSERT INTO streamer_revenue_summary (streamer_id, total_revenue, ` + column + `, updated_at)
		VALUES ($1, $2, $2, NOW())
		ON CONFLICT (streamer_id) DO UPDATE SET
			total_revenue  = streamer_revenue_summary.total_revenue + $2,
			` + column + `  = streamer_revenue_summary.` + column + ` + $2,
			updated_at     = NOW()
	`
	_, err := a.db.Exec(ctx, query, streamerID, amount)
	return err
}
