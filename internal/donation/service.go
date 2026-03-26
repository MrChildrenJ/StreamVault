package donation

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MrChildrenJ/streamvault/internal/db"
	"github.com/MrChildrenJ/streamvault/internal/event"
	"github.com/MrChildrenJ/streamvault/internal/transaction"
	"github.com/MrChildrenJ/streamvault/internal/wallet"
)

const maxDebitRetries = 3

type Service struct {
	db       *pgxpool.Pool
	wallets  *wallet.Repository
	txs      *transaction.Repository
	producer *event.Producer
}

func NewService(
	db *pgxpool.Pool,
	wallets *wallet.Repository,
	txs *transaction.Repository,
	producer *event.Producer,
) *Service {
	return &Service{db: db, wallets: wallets, txs: txs, producer: producer}
}

// Donate sends a fiat donation from a user to a streamer.
// Atomically: debit wallet + insert transaction.
// Then publishes donation.received for async revenue aggregation.
func (s *Service) Donate(ctx context.Context, fromUserID, streamerID string, amountCents int64, message, idempotencyKey string) error {
	var txID string

	for attempt := 0; attempt < maxDebitRetries; attempt++ {
		w, err := s.wallets.GetByUserID(ctx, fromUserID)
		if err != nil {
			return err
		}
		if w.Balance < amountCents {
			return wallet.ErrInsufficientFunds
		}

		err = db.WithTx(ctx, s.db, func(tx pgx.Tx) error {
			if err := s.wallets.Debit(ctx, tx, fromUserID, amountCents, w.Version); err != nil {
				return err
			}
			t := &transaction.Transaction{
				IdempotencyKey: idempotencyKey,
				Type:           transaction.TypeDonation,
				Status:         transaction.StatusCompleted,
				Amount:         amountCents,
				Currency:       "USD",
				FromUserID:     &fromUserID,
				StreamerID:     &streamerID,
			}
			if err := s.txs.Create(ctx, tx, t); err != nil {
				return err
			}
			txID = t.ID
			return nil
		})

		if err == nil {
			break
		}
		if err != wallet.ErrVersionConflict {
			return err
		}
	}
	if txID == "" {
		return wallet.ErrMaxRetriesExceeded
	}

	_ = s.producer.Publish(ctx, event.TopicDonationReceived, streamerID, event.DonationReceivedEvent{
		FromUserID:  fromUserID,
		StreamerID:  streamerID,
		AmountCents: amountCents,
		Message:     message,
		TxID:        txID,
		OccuredAt:   time.Now(),
	})
	return nil
}
