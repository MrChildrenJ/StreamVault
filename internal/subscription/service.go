package subscription

import (
	"context"
	"fmt"
	"log"
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
	subs    *Repository
	wallets *wallet.Repository
	txs     *transaction.Repository
	outbox  *event.OutboxRepository
}

func NewService(
	db *pgxpool.Pool,
	subs *Repository,
	wallets *wallet.Repository,
	txs *transaction.Repository,
	outbox *event.OutboxRepository,
) *Service {
	return &Service{db: db, subs: subs, wallets: wallets, txs: txs, outbox: outbox}
}

// Subscribe creates a self-subscription for subscriberID to a streamer's tier.
// Atomically: debit wallet + insert transaction + insert subscription.
func (s *Service) Subscribe(ctx context.Context, subscriberID, tierID, idempotencyKey string) (*Subscription, error) {
	return s.createSubscription(ctx, subscriberID, tierID, nil, idempotencyKey)
}

// GiftSubscription creates a subscription paid by gifterID but credited to recipientID.
func (s *Service) GiftSubscription(ctx context.Context, gifterID, recipientID, tierID, idempotencyKey string) (*Subscription, error) {
	return s.createSubscription(ctx, recipientID, tierID, &gifterID, idempotencyKey)
}

// createSubscription is the shared core for Subscribe and GiftSubscription.
// payerID is gifterID for gifts, subscriberID for self-subs.
func (s *Service) createSubscription(ctx context.Context, subscriberID, tierID string, gifterID *string, idempotencyKey string) (*Subscription, error) {
	tier, err := s.subs.GetTierByID(ctx, tierID)
	if err != nil {
		return nil, err
	}

	payerID := subscriberID
	if gifterID != nil {
		payerID = *gifterID
	}

	var sub *Subscription

	for attempt := 0; attempt < maxDebitRetries; attempt++ {
		w, err := s.wallets.GetByUserID(ctx, payerID)
		if err != nil {
			return nil, err
		}
		if w.Balance < tier.Price {
			return nil, wallet.ErrInsufficientFunds
		}

		now := time.Now()
		sub = &Subscription{
			SubscriberID: subscriberID,
			StreamerID:   tier.StreamerID,
			TierID:       tierID,
			GiftedBy:     gifterID,
			StartedAt:    now,
			ExpiresAt:    now.Add(SubDuration),
		}

		err = db.WithTx(ctx, s.db, func(tx pgx.Tx) error {
			if err := s.wallets.Debit(ctx, tx, payerID, tier.Price, w.Version); err != nil {
				return err
			}

			t := &transaction.Transaction{
				IdempotencyKey: idempotencyKey,
				Type:           txType(gifterID),
				Status:         transaction.StatusCompleted,
				Amount:         tier.Price,
				Currency:       tier.Currency,
				FromUserID:     &payerID,
				StreamerID:     &tier.StreamerID,
			}
			if err := s.txs.Create(ctx, tx, t); err != nil {
				return fmt.Errorf("subscription tx: %w", err)
			}
			sub.TransactionID = t.ID

			if err := s.subs.Create(ctx, tx, sub); err != nil {
				return err
			}
			return s.outbox.Enqueue(ctx, tx, event.TopicSubscriptionCreated, tier.StreamerID, event.SubscriptionCreatedEvent{
				SubscriptionID: sub.ID,
				SubscriberID:   subscriberID,
				StreamerID:     tier.StreamerID,
				TierID:         tierID,
				AmountCents:    tier.Price,
				GiftedBy:       gifterID,
				TxID:           sub.TransactionID,
				OccuredAt:      time.Now(),
			})
		})

		if err == nil {
			break
		}
		if err != wallet.ErrVersionConflict {
			return nil, err
		}
	}
	if sub == nil || sub.ID == "" {
		return nil, wallet.ErrMaxRetriesExceeded
	}
	return sub, nil
}

func txType(gifterID *string) transaction.Type {
	if gifterID != nil {
		return transaction.TypeGiftSubscription
	}
	return transaction.TypeSubscription
}

// StartExpiryWorker runs a background goroutine that periodically expires old subscriptions.
func (s *Service) StartExpiryWorker(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := s.subs.ExpireOld(ctx)
				if err != nil {
					log.Printf("expiry worker: %v", err)
				} else if n > 0 {
					log.Printf("expiry worker: expired %d subscriptions", n)
				}
			}
		}
	}()
}
