package wallet

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MrChildrenJ/streamvault/internal/transaction"
)

const maxDebitRetries = 3

// withTx runs fn inside a database transaction, committing on success and rolling back on error.
func withTx(ctx context.Context, db *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck — rollback error is irrelevant after a real error
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type Service struct {
	db      *pgxpool.Pool
	wallets *Repository
	txs     *transaction.Repository
}

func NewService(db *pgxpool.Pool, wallets *Repository, txs *transaction.Repository) *Service {
	return &Service{db: db, wallets: wallets, txs: txs}
}

// TopUp credits the user's wallet and records the transaction atomically.
// Idempotent: repeated calls with the same idempotencyKey are safe.
func (s *Service) TopUp(ctx context.Context, userID string, amount int64, idempotencyKey string) error {
	return withTx(ctx, s.db, func(tx pgx.Tx) error {
		if err := s.wallets.Credit(ctx, tx, userID, amount); err != nil {
			return fmt.Errorf("TopUp credit: %w", err)
		}
		return s.txs.Create(ctx, tx, &transaction.Transaction{
			IdempotencyKey: idempotencyKey,
			Type:           transaction.TypeTopUp,
			Status:         transaction.StatusCompleted,
			Amount:         amount,
			Currency:       "USD",
			ToUserID:       &userID,
		})
	})
}

// Debit subtracts amount from the wallet using an optimistic-lock retry loop.
// It records a transaction of txType atomically with the balance change.
// Idempotent: repeated calls with the same idempotencyKey are safe.
//
// Concurrency model:
//  1. Read wallet (gets current balance + version).
//  2. Check balance >= amount in application code.
//  3. Open DB transaction: UPDATE with WHERE version = <old>; insert ledger row.
//  4. If UPDATE affects 0 rows → version changed by a concurrent writer → retry.
func (s *Service) Debit(
	ctx context.Context,
	userID string,
	amount int64,
	idempotencyKey string,
	txType transaction.Type,
	streamerID *string,
) error {
	for attempt := 0; attempt < maxDebitRetries; attempt++ {
		// Fresh read every attempt to get the latest balance + version.
		w, err := s.wallets.GetByUserID(ctx, userID)
		if err != nil {
			return err
		}
		if w.Balance < amount {
			return ErrInsufficientFunds
		}

		err = withTx(ctx, s.db, func(tx pgx.Tx) error {
			if err := s.wallets.Debit(ctx, tx, userID, amount, w.Version); err != nil {
				return err
			}
			return s.txs.Create(ctx, tx, &transaction.Transaction{
				IdempotencyKey: idempotencyKey,
				Type:           txType,
				Status:         transaction.StatusCompleted,
				Amount:         amount,
				Currency:       "USD",
				FromUserID:     &userID,
				StreamerID:     streamerID,
			})
		})

		if err == nil {
			return nil
		}
		if err != ErrVersionConflict {
			return err
		}
		// Version conflict: loop and retry with fresh read.
	}
	return ErrMaxRetriesExceeded
}

// GetBalance returns the current wallet for a user.
func (s *Service) GetBalance(ctx context.Context, userID string) (*Wallet, error) {
	return s.wallets.GetByUserID(ctx, userID)
}
