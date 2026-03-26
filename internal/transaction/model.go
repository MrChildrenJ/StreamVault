package transaction

import (
	"encoding/json"
	"time"
)

type Type string

const (
	TypeTopUp            Type = "top_up"
	TypeSubscription     Type = "subscription"
	TypeGiftSubscription Type = "gift_subscription"
	TypeDonation         Type = "donation"
	TypeBitsPurchase     Type = "bits_purchase"
	TypeBitsCheer        Type = "bits_cheer"
	TypePayout           Type = "payout"
	TypeRefund           Type = "refund"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusReversed  Status = "reversed"
)

type Transaction struct {
	ID             string
	IdempotencyKey string
	Type           Type
	Status         Status
	Amount         int64           // always positive, in cents or bits
	Currency       string
	FromUserID     *string         // nil for external top-up
	ToUserID       *string         // nil for bank payouts
	StreamerID     *string
	Metadata       json.RawMessage // sub tier, donation message, etc.
	CreatedAt      time.Time
}
