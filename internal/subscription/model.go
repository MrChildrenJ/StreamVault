package subscription

import (
	"errors"
	"time"
)

var (
	ErrTierNotFound = errors.New("subscription tier not found")
	ErrNotFound     = errors.New("subscription not found")
)

const SubDuration = 30 * 24 * time.Hour // one subscription period

type Tier struct {
	ID         string
	StreamerID string
	Name       string
	Price      int64 // in cents
	Currency   string
	CreatedAt  time.Time
}

type Subscription struct {
	ID            string
	SubscriberID  string
	StreamerID    string
	TierID        string
	Status        string
	GiftedBy      *string
	TransactionID string
	StartedAt     time.Time
	ExpiresAt     time.Time
	CreatedAt     time.Time
}
