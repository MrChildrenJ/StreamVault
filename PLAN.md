# StreamVault — Implementation Plan

## Project Goal

Build a Twitch-like monetization backend in Go, covering:
- User wallet (fiat balance)
- Subscriptions / donations / gift subscriptions
- Virtual currency (Bits-like: purchase → spend → streamer earns)
- Immutable transaction ledger
- Streamer revenue dashboard

---

## Current State (as of 2026-03-26)

| Layer | Status | Notes |
|---|---|---|
| DB Schema | ✅ Done | `migrations/000001_init_schema.up.sql` |
| Docker Infra | ✅ Done | PostgreSQL 16 + Kafka (KRaft mode) via docker-compose |
| Go module | ✅ Done | `github.com/MrChildrenJ/streamvault`, Go 1.24.5 |
| Server entrypoint | 🟡 Stub | `cmd/server/main.go` — just prints "Hello" |
| Business logic | ❌ Not started | |
| API layer | ❌ Not started | |
| Event publishing | ❌ Not started | |
| Tests | ❌ Not started | |

### Existing Schema Summary

```
users                      — id, username, email, role (viewer|streamer)
wallets                    — user_id (1:1), balance (cents), version (optimistic lock)
virtual_currency_balances  — user_id (1:1), balance (bits), version (optimistic lock)
transactions               — immutable ledger, idempotency_key UNIQUE, type/status enums
subscription_tiers         — streamer-defined tiers with price
subscriptions              — subscriber→streamer, tier, gifted_by, expires_at
streamer_revenue_summary   — materialised totals for dashboard
```

Key design decisions already baked in:
- `idempotency_key` on transactions → prevents duplicate charges
- `version` on wallets/bits balances → optimistic locking for concurrent deductions
- `no_update / no_delete` RULES on transactions → ledger immutability enforced at DB level
- All monetary amounts in smallest unit (cents / bits as integers)

---

## Architecture

```
HTTP API (Gin/Chi)
     │
     ▼
Service Layer  ──────────────────► Kafka Producer
(wallet, sub, donation, bits)             │
     │                                    ▼
     ▼                           Event Consumers
Repository Layer                 (revenue aggregation,
(PostgreSQL via pgx)              notification hooks)
```

---

## Implementation Phases

### Phase 1 — Project Skeleton ✅
- [x] Add dependencies: `pgx/v5`, `kafka-go`, `gin`, `golang-migrate`, `envconfig`
- [x] Wire DB connection pool in `main.go`
- [x] Add config loading (`envconfig`)
- [x] Directory structure: `internal/{config,db,wallet,bits,subscription,donation,dashboard,event}`

### Phase 2 — Wallet & Transaction Core ✅
- [x] `WalletRepository`: `GetByUserID`, `Credit`, `Debit` (optimistic lock)
- [x] `TransactionRepository`: `Create` (idempotency via `ON CONFLICT DO NOTHING`)
- [x] `WalletService.TopUp`, `Debit` (retry loop on version conflict, max 3 attempts)
- [x] `withTx` helper (Begin + defer Rollback + Commit)
- [x] HTTP handlers: `GET /api/v1/users/:id/wallet`, `POST /api/v1/wallets/topup`
- [ ] Unit tests with real Postgres (testcontainers)

### Phase 3 — Virtual Currency (Bits) ✅
- [x] `VirtualCurrencyRepository`: same optimistic lock pattern as wallet
- [x] `BitsService.Purchase` — optimistic-lock retry on wallet debit, credit bits, insert tx
- [x] `BitsService.Cheer` — optimistic-lock retry on bits debit, insert tx
- [x] Kafka producer: publish `bits.purchased` / `bits.cheered` after DB commit (best-effort)
- [x] `withTx` moved to `internal/db` package (shared)

### Phase 4 — Subscriptions & Donations ✅
- [x] `SubscriptionService.Subscribe` — optimistic-lock retry, debit wallet + insert tx + insert sub (single DB tx)
- [x] `SubscriptionService.GiftSubscription` — same flow, gifterID pays, recipientID gets sub
- [x] `DonationService.Donate` — optimistic-lock retry, debit wallet + insert tx
- [x] Publish `subscription.created`, `donation.received` to Kafka after commit
- [x] Background expiry worker (`StartExpiryWorker`) — tickers every 5 min, updates expired subs
- [x] `transaction.Create` upgraded to `RETURNING id` so callers get the generated ID

### Phase 5 — Revenue Dashboard
- [ ] Kafka consumer: `revenue-aggregator` service
  - On each `subscription.created` / `donation.received` / `bits.cheered` → update `streamer_revenue_summary`
- [ ] `DashboardHandler GET /streamers/:id/revenue` — reads summary table
- [ ] `DashboardHandler GET /streamers/:id/transactions` — paginated ledger query

### Phase 6 — HTTP API Surface

| Method | Path | Description |
|---|---|---|
| POST | `/wallets/topup` | Add fiat to wallet |
| POST | `/bits/purchase` | Buy bits with wallet balance |
| POST | `/bits/cheer` | Cheer bits at a streamer |
| POST | `/subscriptions` | Subscribe to a streamer |
| POST | `/subscriptions/gift` | Gift a sub |
| POST | `/donations` | Send a fiat donation |
| GET | `/streamers/:id/revenue` | Revenue dashboard |
| GET | `/streamers/:id/transactions` | Transaction history |
| GET | `/users/:id/wallet` | Wallet balance |
| GET | `/users/:id/bits` | Bits balance |

### Phase 7 — Hardening
- [ ] Idempotency middleware: cache `idempotency_key` responses (Redis or Postgres)
- [ ] Distributed lock consideration for high-concurrency debit (optimistic lock + retry is baseline; add advisory lock if contention is high)
- [ ] Payout flow: `streamer_revenue_summary` → `transactions (type=payout)` → mark pending until external payment processor confirms
- [ ] Circuit breaker on Kafka publish (don't fail the write path if Kafka is down — use outbox pattern)

---

## Key Design Principles

### Idempotency
Every mutating operation takes an `idempotency_key`. The `transactions` table has a `UNIQUE` constraint on it — a duplicate request hits `ON CONFLICT DO NOTHING` and returns the original result.

### High-concurrency Debit (Race Condition Prevention)
```
1. SELECT balance, version FROM wallets WHERE user_id = $1
2. Check balance >= amount
3. UPDATE wallets SET balance = balance - $amount, version = version + 1
   WHERE user_id = $1 AND version = $old_version
4. If 0 rows updated → version conflict → retry from step 1 (max 3 retries)
```

### Eventual Consistency
The write path (debit + insert transaction) is synchronous and strongly consistent within PostgreSQL. Revenue aggregation is async via Kafka — the dashboard may lag by seconds, which is acceptable.

### Immutable Ledger
PostgreSQL `RULE` prevents UPDATE/DELETE on `transactions`. The only way to "reverse" is to insert a new `refund` transaction row.

---

## Directory Layout (Target)

```
StreamVault/
├── cmd/server/main.go
├── internal/
│   ├── config/
│   ├── db/                  # connection pool, migrations
│   ├── wallet/
│   │   ├── repository.go
│   │   ├── service.go
│   │   └── handler.go
│   ├── bits/
│   │   ├── repository.go
│   │   ├── service.go
│   │   └── handler.go
│   ├── subscription/
│   │   ├── repository.go
│   │   ├── service.go
│   │   └── handler.go
│   ├── donation/
│   │   ├── repository.go
│   │   ├── service.go
│   │   └── handler.go
│   ├── dashboard/
│   │   └── handler.go
│   └── event/               # Kafka producer + consumers
│       ├── producer.go
│       └── consumer/
│           └── revenue_aggregator.go
├── migrations/
├── docker-compose.yml
├── go.mod
└── PLAN.md
```
