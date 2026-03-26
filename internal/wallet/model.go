package wallet

import (
	"errors"
	"time"
)

var (
	ErrNotFound           = errors.New("wallet not found")
	ErrInsufficientFunds  = errors.New("insufficient funds")
	ErrVersionConflict    = errors.New("optimistic lock version conflict")
	ErrMaxRetriesExceeded = errors.New("max retries exceeded on concurrent debit")
)

type Wallet struct {
	ID        string
	UserID    string
	Balance   int64  // in cents
	Currency  string
	Version   int64  // incremented on every write; used for optimistic locking
	CreatedAt time.Time
	UpdatedAt time.Time
}
