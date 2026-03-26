package event

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
)

const (
	TopicBitsPurchased      = "bits.purchased"
	TopicBitsCheered        = "bits.cheered"
	TopicSubscriptionCreated = "subscription.created"
	TopicDonationReceived   = "donation.received"
)

// BitsPurchasedEvent is published when a user buys bits with fiat.
type BitsPurchasedEvent struct {
	UserID    string    `json:"user_id"`
	Bits      int64     `json:"bits"`
	PaidCents int64     `json:"paid_cents"`
	TxID      string    `json:"transaction_id"`
	OccuredAt time.Time `json:"occured_at"`
}

// BitsCheeredEvent is published when a user cheers bits at a streamer.
type BitsCheeredEvent struct {
	UserID     string    `json:"user_id"`
	StreamerID string    `json:"streamer_id"`
	Bits       int64     `json:"bits"`
	TxID       string    `json:"transaction_id"`
	OccuredAt  time.Time `json:"occured_at"`
}

// SubscriptionCreatedEvent is published when a subscription is created (self or gift).
type SubscriptionCreatedEvent struct {
	SubscriptionID string    `json:"subscription_id"`
	SubscriberID   string    `json:"subscriber_id"`
	StreamerID     string    `json:"streamer_id"`
	TierID         string    `json:"tier_id"`
	AmountCents    int64     `json:"amount_cents"`
	GiftedBy       *string   `json:"gifted_by,omitempty"`
	TxID           string    `json:"transaction_id"`
	OccuredAt      time.Time `json:"occured_at"`
}

// DonationReceivedEvent is published when a fiat donation is made.
type DonationReceivedEvent struct {
	FromUserID  string    `json:"from_user_id"`
	StreamerID  string    `json:"streamer_id"`
	AmountCents int64     `json:"amount_cents"`
	Message     string    `json:"message,omitempty"`
	TxID        string    `json:"transaction_id"`
	OccuredAt   time.Time `json:"occured_at"`
}

// Producer wraps a kafka-go writer for publishing domain events.
type Producer struct {
	writer *kafka.Writer
}

func NewProducer(broker string) *Producer {
	return &Producer{
		writer: &kafka.Writer{
			Addr:         kafka.TCP(broker),
			Balancer:     &kafka.LeastBytes{},
			RequiredAcks: kafka.RequireOne,
		},
	}
}

func (p *Producer) Close() error {
	return p.writer.Close()
}

// Publish sends an event to the given topic.
// The userID is used as the message key to preserve per-user ordering.
func (p *Producer) Publish(ctx context.Context, topic, key string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("event marshal: %w", err)
	}
	return p.writer.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: body,
	})
}
