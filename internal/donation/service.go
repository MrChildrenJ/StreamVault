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
	db      *pgxpool.Pool
	wallets *wallet.Repository
	txs     *transaction.Repository
	outbox  *event.OutboxRepository
}

func NewService(
	db *pgxpool.Pool,
	wallets *wallet.Repository,
	txs *transaction.Repository,
	outbox *event.OutboxRepository,
) *Service {
	return &Service{db: db, wallets: wallets, txs: txs, outbox: outbox}
}

// Donate sends a fiat donation from a user to a streamer.
// All side-effects happen in one DB transaction via the outbox pattern.
func (s *Service) Donate(ctx context.Context, fromUserID, streamerID string, amountCents int64, message, idempotencyKey string) error {
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
			return s.outbox.Enqueue(ctx, tx, event.TopicDonationReceived, streamerID, event.DonationReceivedEvent{
				FromUserID:  fromUserID,
				StreamerID:  streamerID,
				AmountCents: amountCents,
				Message:     message,
				TxID:        t.ID,
				OccuredAt:   time.Now(),
			})
		})

		if err == nil {
			return nil
		}
		if err != wallet.ErrVersionConflict {
			return err
		}
	}
	return wallet.ErrMaxRetriesExceeded
}
