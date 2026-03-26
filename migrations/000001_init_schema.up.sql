-- StreamVault Schema
-- All monetary amounts are stored in the smallest unit (e.g., USD cents)
-- Virtual currency (bits) stored as integer units

-- Enable UUID generation
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ============================================================
-- USERS
-- ============================================================
CREATE TABLE users (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username    VARCHAR(64) NOT NULL UNIQUE,
    email       VARCHAR(255) NOT NULL UNIQUE,
    role        VARCHAR(16) NOT NULL DEFAULT 'viewer' CHECK (role IN ('viewer', 'streamer')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- WALLETS (fiat / real money)
-- ============================================================
CREATE TABLE wallets (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID NOT NULL UNIQUE REFERENCES users(id),
    balance     BIGINT NOT NULL DEFAULT 0 CHECK (balance >= 0),  -- in cents
    currency    CHAR(3) NOT NULL DEFAULT 'USD',
    version     BIGINT NOT NULL DEFAULT 0,  -- optimistic locking
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- VIRTUAL CURRENCY BALANCES (Bits-like)
-- ============================================================
CREATE TABLE virtual_currency_balances (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID NOT NULL UNIQUE REFERENCES users(id),
    balance     BIGINT NOT NULL DEFAULT 0 CHECK (balance >= 0),  -- in bits
    version     BIGINT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- TRANSACTION LEDGER (immutable append-only)
-- ============================================================
CREATE TYPE transaction_type AS ENUM (
    'top_up',               -- user adds money to wallet
    'subscription',         -- subscription payment
    'gift_subscription',    -- gift sub payment
    'donation',             -- fiat donation
    'bits_purchase',        -- buy virtual currency
    'bits_cheer',           -- spend bits on a streamer
    'payout',               -- streamer withdraws revenue
    'refund'                -- refund
);

CREATE TYPE transaction_status AS ENUM (
    'pending',
    'completed',
    'failed',
    'reversed'
);

CREATE TABLE transactions (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    idempotency_key     VARCHAR(128) NOT NULL UNIQUE,  -- prevent duplicate charges
    type                transaction_type NOT NULL,
    status              transaction_status NOT NULL DEFAULT 'pending',
    amount              BIGINT NOT NULL CHECK (amount > 0),  -- always positive; direction via from/to
    currency            CHAR(3) NOT NULL DEFAULT 'USD',
    from_user_id        UUID REFERENCES users(id),           -- NULL for external top-up
    to_user_id          UUID REFERENCES users(id),           -- NULL for payouts to bank
    streamer_id         UUID REFERENCES users(id),           -- relevant streamer (if any)
    metadata            JSONB,                               -- flexible: sub tier, message, etc.
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
    -- NO updated_at: this table is append-only
);

-- ============================================================
-- SUBSCRIPTION TIERS
-- ============================================================
CREATE TABLE subscription_tiers (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    streamer_id UUID NOT NULL REFERENCES users(id),
    name        VARCHAR(64) NOT NULL,           -- e.g. "Tier 1", "Tier 2", "Tier 3"
    price       BIGINT NOT NULL CHECK (price > 0),  -- in cents
    currency    CHAR(3) NOT NULL DEFAULT 'USD',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (streamer_id, name)
);

-- ============================================================
-- SUBSCRIPTIONS
-- ============================================================
CREATE TYPE subscription_status AS ENUM (
    'active',
    'cancelled',
    'expired'
);

CREATE TABLE subscriptions (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    subscriber_id   UUID NOT NULL REFERENCES users(id),
    streamer_id     UUID NOT NULL REFERENCES users(id),
    tier_id         UUID NOT NULL REFERENCES subscription_tiers(id),
    status          subscription_status NOT NULL DEFAULT 'active',
    gifted_by       UUID REFERENCES users(id),   -- NULL if self-subscribed
    transaction_id  UUID NOT NULL REFERENCES transactions(id),
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (subscriber_id, streamer_id, started_at)
);

-- ============================================================
-- STREAMER REVENUE SUMMARY (materialised for dashboard)
-- ============================================================
CREATE TABLE streamer_revenue_summary (
    streamer_id         UUID PRIMARY KEY REFERENCES users(id),
    total_revenue       BIGINT NOT NULL DEFAULT 0,   -- lifetime, in cents
    subscription_revenue BIGINT NOT NULL DEFAULT 0,
    donation_revenue    BIGINT NOT NULL DEFAULT 0,
    bits_revenue        BIGINT NOT NULL DEFAULT 0,   -- bits cheered (converted)
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- INDEXES
-- ============================================================
CREATE INDEX idx_transactions_from_user    ON transactions(from_user_id);
CREATE INDEX idx_transactions_to_user      ON transactions(to_user_id);
CREATE INDEX idx_transactions_streamer     ON transactions(streamer_id);
CREATE INDEX idx_transactions_created_at   ON transactions(created_at DESC);
CREATE INDEX idx_transactions_type_status  ON transactions(type, status);
CREATE INDEX idx_subscriptions_subscriber  ON subscriptions(subscriber_id);
CREATE INDEX idx_subscriptions_streamer    ON subscriptions(streamer_id);
CREATE INDEX idx_subscriptions_status      ON subscriptions(status, expires_at);

-- ============================================================
-- PREVENT UPDATE/DELETE ON TRANSACTIONS (ledger integrity)
-- ============================================================
CREATE RULE no_update_transactions AS ON UPDATE TO transactions DO INSTEAD NOTHING;
CREATE RULE no_delete_transactions AS ON DELETE TO transactions DO INSTEAD NOTHING;
