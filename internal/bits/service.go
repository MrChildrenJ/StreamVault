package bits

import (
	"context"
	"fmt"
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
	bits    *Repository
	wallets *wallet.Repository
	txs     *transaction.Repository
	outbox  *event.OutboxRepository
}

func NewService(
	db *pgxpool.Pool,
	bits *Repository,
	wallets *wallet.Repository,
	txs *transaction.Repository,
	outbox *event.OutboxRepository,
) *Service {
	return &Service{db: db, bits: bits, wallets: wallets, txs: txs, outbox: outbox}
}

// Purchase buys bits using fiat wallet balance.
// All side-effects (wallet debit, bits credit, ledger entry, outbox event)
// happen in a single DB transaction — nothing is lost if the process crashes.
func (s *Service) Purchase(ctx context.Context, userID string, bitsAmount int64, paidCents int64, idempotencyKey string) error {
	for attempt := 0; attempt < maxDebitRetries; attempt++ {
		w, err := s.wallets.GetByUserID(ctx, userID)
		if err != nil {
			return err
		}
		if w.Balance < paidCents {
			return wallet.ErrInsufficientFunds
		}

		err = db.WithTx(ctx, s.db, func(tx pgx.Tx) error {
			if err := s.wallets.Debit(ctx, tx, userID, paidCents, w.Version); err != nil {
				return err
			}
			if err := s.bits.Credit(ctx, tx, userID, bitsAmount); err != nil {
				return fmt.Errorf("purchase bits credit: %w", err)
			}
			t := &transaction.Transaction{
				IdempotencyKey: idempotencyKey,
				Type:           transaction.TypeBitsPurchase,
				Status:         transaction.StatusCompleted,
				Amount:         paidCents,
				Currency:       "USD",
				FromUserID:     &userID,
			}
			if err := s.txs.Create(ctx, tx, t); err != nil {
				return err
			}
			return s.outbox.Enqueue(ctx, tx, event.TopicBitsPurchased, userID, event.BitsPurchasedEvent{
				UserID:    userID,
				Bits:      bitsAmount,
				PaidCents: paidCents,
				TxID:      t.ID,
				OccuredAt: time.Now(),
			})
		})

		if err == nil {
			return nil
		}
		if err != wallet.ErrVersionConflict {
			return err
		}
	}
	return ErrMaxRetriesExceeded
}

// Cheer spends bits on a streamer.
// Outbox event drives async revenue aggregation — safe even if Kafka is down.
func (s *Service) Cheer(ctx context.Context, userID, streamerID string, bitsAmount int64, idempotencyKey string) error {
	for attempt := 0; attempt < maxDebitRetries; attempt++ {
		b, err := s.bits.GetByUserID(ctx, userID)
		if err != nil {
			return err
		}
		if b.Balance < bitsAmount {
			return ErrInsufficientBits
		}

		err = db.WithTx(ctx, s.db, func(tx pgx.Tx) error {
			if err := s.bits.Debit(ctx, tx, userID, bitsAmount, b.Version); err != nil {
				return err
			}
			t := &transaction.Transaction{
				IdempotencyKey: idempotencyKey,
				Type:           transaction.TypeBitsCheer,
				Status:         transaction.StatusCompleted,
				Amount:         bitsAmount,
				Currency:       "USD",
				FromUserID:     &userID,
				StreamerID:     &streamerID,
			}
			if err := s.txs.Create(ctx, tx, t); err != nil {
				return err
			}
			return s.outbox.Enqueue(ctx, tx, event.TopicBitsCheered, streamerID, event.BitsCheeredEvent{
				UserID:     userID,
				StreamerID: streamerID,
				Bits:       bitsAmount,
				TxID:       t.ID,
				OccuredAt:  time.Now(),
			})
		})

		if err == nil {
			return nil
		}
		if err != ErrVersionConflict {
			return err
		}
	}
	return ErrMaxRetriesExceeded
}

// GetBalance returns the bits balance for a user.
func (s *Service) GetBalance(ctx context.Context, userID string) (*Balance, error) {
	return s.bits.GetByUserID(ctx, userID)
}
