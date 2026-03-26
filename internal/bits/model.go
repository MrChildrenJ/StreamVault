package bits

import (
	"errors"
	"time"
)

var (
	ErrNotFound           = errors.New("bits balance not found")
	ErrInsufficientBits   = errors.New("insufficient bits")
	ErrVersionConflict    = errors.New("optimistic lock version conflict")
	ErrMaxRetriesExceeded = errors.New("max retries exceeded on concurrent debit")
)

type Balance struct {
	ID        string
	UserID    string
	Balance   int64 // in bits
	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}
