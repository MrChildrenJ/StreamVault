# StreamVault

A Twitch-like monetization backend built in Go, featuring wallet management, subscriptions, virtual currency (Bits), donations, and a streamer revenue dashboard.

## Features

| Domain | What it does |
|---|---|
| **Wallet** | Fiat balance per user, top-up, debit |
| **Bits** | Virtual currency — purchase with fiat, cheer at streamers |
| **Subscriptions** | Tiered subs, gift subs, 30-day expiry |
| **Donations** | Fiat donation from viewer to streamer |
| **Revenue Dashboard** | Per-streamer earnings breakdown + paginated transaction ledger |
| **Payouts** | Streamer withdraws earned revenue (status: pending) |

## Key Design Decisions

**Idempotency** — every mutating endpoint requires an `idempotency_key`. The `transactions` table has a `UNIQUE` constraint on it; duplicate requests hit `ON CONFLICT DO NOTHING` and are silently skipped.

**High-concurrency debit** — wallets and bits balances use optimistic locking (`version` column). On conflict, the service re-reads the row and retries up to 3 times before returning `409 Conflict`.

**Immutable ledger** — PostgreSQL `RULE` prevents `UPDATE`/`DELETE` on the `transactions` table. Reversals are new rows of type `refund`.

**Outbox pattern** — Kafka events are written to an `event_outbox` table in the *same* DB transaction as the business operation. A background `OutboxRelay` polls every 2 seconds, publishes to Kafka with `FOR UPDATE SKIP LOCKED` (safe for multiple instances), then marks rows as published. The write path never fails due to Kafka being down.

**Eventual consistency** — the `streamer_revenue_summary` table is updated asynchronously by a Kafka consumer (`RevenueAggregator`). Dashboard reads may lag by a few seconds.

## Tech Stack

- **Language**: Go 1.25
- **Database**: PostgreSQL 16 (pgx/v5, golang-migrate)
- **Message broker**: Kafka (KRaft mode, segmentio/kafka-go)
- **HTTP framework**: Gin
- **Infrastructure**: Docker Compose

## Project Structure

```
cmd/server/          — entrypoint (wires all dependencies, starts HTTP server)
internal/
  config/            — env-based config (envconfig)
  db/                — connection pool, migrations, WithTx helper
  wallet/            — fiat wallet: model, repo, service, handler
  bits/              — virtual currency: model, repo, service, handler
  subscription/      — subs & gift subs: model, repo, service, handler
  donation/          — fiat donations: service, handler
  payout/            — streamer payouts: service, handler
  dashboard/         — revenue summary + tx ledger: repo, handler
  transaction/       — immutable ledger: model, repo
  event/
    producer.go      — Kafka producer
    outbox.go        — OutboxRepository + OutboxRelay
    consumer/        — RevenueAggregator (Kafka consumer)
migrations/          — SQL migration files (golang-migrate)
```

## Getting Started

### Prerequisites

- Go 1.25+
- Docker & Docker Compose

### Run

```bash
# 1. Start PostgreSQL and Kafka
docker-compose up -d

# 2. Copy and configure environment
cp .env.example .env

# 3. Start the server (migrations run automatically on startup)
go run ./cmd/server/main.go
```

```
migrations applied
database connected
StreamVault listening on :8080
```

### Health check

```bash
curl http://localhost:8080/healthz
```

### Seed test data

```bash
docker exec -it streamvault-postgres psql -U streamvault -d streamvault
```

```sql
INSERT INTO users (id, username, email, role) VALUES
  ('aaaaaaaa-0000-0000-0000-000000000001', 'viewer1',   'viewer1@test.com',   'viewer'),
  ('bbbbbbbb-0000-0000-0000-000000000002', 'streamer1', 'streamer1@test.com', 'streamer');

INSERT INTO wallets (user_id, balance) VALUES
  ('aaaaaaaa-0000-0000-0000-000000000001', 0),
  ('bbbbbbbb-0000-0000-0000-000000000002', 0);

INSERT INTO virtual_currency_balances (user_id, balance) VALUES
  ('aaaaaaaa-0000-0000-0000-000000000001', 0),
  ('bbbbbbbb-0000-0000-0000-000000000002', 0);

INSERT INTO subscription_tiers (streamer_id, name, price) VALUES
  ('bbbbbbbb-0000-0000-0000-000000000002', 'Tier 1', 499);
```

## API Reference

All endpoints are prefixed with `/api/v1`.

### Wallet

| Method | Path | Body fields |
|---|---|---|
| `GET` | `/users/:id/wallet` | — |
| `POST` | `/wallets/topup` | `user_id`, `amount` (cents), `idempotency_key` |

### Bits (Virtual Currency)

| Method | Path | Body fields |
|---|---|---|
| `GET` | `/users/:id/bits` | — |
| `POST` | `/bits/purchase` | `user_id`, `bits`, `paid_cents`, `idempotency_key` |
| `POST` | `/bits/cheer` | `user_id`, `streamer_id`, `bits`, `idempotency_key` |

### Subscriptions

| Method | Path | Body fields |
|---|---|---|
| `POST` | `/subscriptions` | `subscriber_id`, `tier_id`, `idempotency_key` |
| `POST` | `/subscriptions/gift` | `gifter_id`, `recipient_id`, `tier_id`, `idempotency_key` |

### Donations & Payouts

| Method | Path | Body fields |
|---|---|---|
| `POST` | `/donations` | `from_user_id`, `streamer_id`, `amount_cents`, `message`, `idempotency_key` |
| `POST` | `/payouts` | `streamer_id`, `amount_cents`, `idempotency_key` |

### Dashboard

| Method | Path | Query params |
|---|---|---|
| `GET` | `/streamers/:id/revenue` | — |
| `GET` | `/streamers/:id/transactions` | `limit` (max 100), `before` (unix ms cursor) |

### Example flow

```bash
VIEWER=aaaaaaaa-0000-0000-0000-000000000001
STREAMER=bbbbbbbb-0000-0000-0000-000000000002

# Top up wallet ($100)
curl -s -X POST http://localhost:8080/api/v1/wallets/topup \
  -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"$VIEWER\",\"amount\":10000,\"idempotency_key\":\"topup-001\"}"

# Buy 100 bits
curl -s -X POST http://localhost:8080/api/v1/bits/purchase \
  -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"$VIEWER\",\"bits\":100,\"paid_cents\":140,\"idempotency_key\":\"purchase-001\"}"

# Cheer 50 bits at streamer
curl -s -X POST http://localhost:8080/api/v1/bits/cheer \
  -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"$VIEWER\",\"streamer_id\":\"$STREAMER\",\"bits\":50,\"idempotency_key\":\"cheer-001\"}"

# Check streamer revenue (allow ~2s for outbox relay)
sleep 2
curl -s http://localhost:8080/api/v1/streamers/$STREAMER/revenue
```

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | *(required)* | PostgreSQL connection string |
| `KAFKA_BROKER` | `localhost:9092` | Kafka broker address |
| `SERVER_PORT` | `8080` | HTTP listen port |
| `MIGRATIONS_PATH` | `migrations` | Path to SQL migration files |
