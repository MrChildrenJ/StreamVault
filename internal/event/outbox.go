package event

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OutboxRepository writes pending events into the event_outbox table.
// Always call Enqueue inside the same DB transaction as your business operation —
// that atomicity is the whole point of the outbox pattern.
type OutboxRepository struct {
	db *pgxpool.Pool
}

func NewOutboxRepository(db *pgxpool.Pool) *OutboxRepository {
	return &OutboxRepository{db: db}
}

// Enqueue serialises payload as JSON and inserts it into event_outbox within tx.
func (r *OutboxRepository) Enqueue(ctx context.Context, tx pgx.Tx, topic, key string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("outbox marshal: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO event_outbox (topic, key, payload)
		VALUES ($1, $2, $3)
	`, topic, key, body)
	if err != nil {
		return fmt.Errorf("outbox enqueue: %w", err)
	}
	return nil
}

// outboxRow is a single unpublished row from event_outbox.
type outboxRow struct {
	ID      string
	Topic   string
	Key     string
	Payload []byte
}

// OutboxRelay polls the outbox table and forwards pending events to Kafka.
// Using FOR UPDATE SKIP LOCKED makes it safe to run multiple relay instances.
type OutboxRelay struct {
	db       *pgxpool.Pool
	producer *Producer
	interval time.Duration
}

func NewOutboxRelay(db *pgxpool.Pool, producer *Producer, interval time.Duration) *OutboxRelay {
	return &OutboxRelay{db: db, producer: producer, interval: interval}
}

// Start runs the relay loop in a background goroutine until ctx is cancelled.
func (r *OutboxRelay) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := r.flush(ctx); err != nil {
					log.Printf("outbox relay flush: %v", err)
				}
			}
		}
	}()
}

// flush reads up to 100 unpublished rows, publishes them, and marks them done.
func (r *OutboxRelay) flush(ctx context.Context) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// FOR UPDATE SKIP LOCKED: concurrent relay instances skip rows locked by others.
	rows, err := tx.Query(ctx, `
		SELECT id, topic, key, payload
		FROM event_outbox
		WHERE published_at IS NULL
		ORDER BY created_at
		LIMIT 100
		FOR UPDATE SKIP LOCKED
	`)
	if err != nil {
		return fmt.Errorf("outbox select: %w", err)
	}

	var pending []outboxRow
	for rows.Next() {
		var row outboxRow
		if err := rows.Scan(&row.ID, &row.Topic, &row.Key, &row.Payload); err != nil {
			rows.Close()
			return fmt.Errorf("outbox scan: %w", err)
		}
		pending = append(pending, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(pending) == 0 {
		return tx.Commit(ctx)
	}

	// Publish each row to Kafka. On failure we abort — rows stay unpublished and will retry.
	for _, row := range pending {
		if err := r.producer.publishRaw(ctx, row.Topic, row.Key, row.Payload); err != nil {
			return fmt.Errorf("outbox publish topic=%s id=%s: %w", row.Topic, row.ID, err)
		}
	}

	// Mark all published in one UPDATE.
	ids := make([]string, len(pending))
	for i, row := range pending {
		ids[i] = row.ID
	}
	if _, err := tx.Exec(ctx, `
		UPDATE event_outbox SET published_at = NOW() WHERE id = ANY($1::uuid[])
	`, ids); err != nil {
		return fmt.Errorf("outbox mark published: %w", err)
	}

	if n := len(pending); n > 0 {
		log.Printf("outbox relay: published %d events", n)
	}
	return tx.Commit(ctx)
}
