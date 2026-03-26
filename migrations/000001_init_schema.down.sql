DROP RULE IF EXISTS no_delete_transactions ON transactions;
DROP RULE IF EXISTS no_update_transactions ON transactions;

DROP INDEX IF EXISTS idx_subscriptions_status;
DROP INDEX IF EXISTS idx_subscriptions_streamer;
DROP INDEX IF EXISTS idx_subscriptions_subscriber;
DROP INDEX IF EXISTS idx_transactions_type_status;
DROP INDEX IF EXISTS idx_transactions_created_at;
DROP INDEX IF EXISTS idx_transactions_streamer;
DROP INDEX IF EXISTS idx_transactions_to_user;
DROP INDEX IF EXISTS idx_transactions_from_user;

DROP TABLE IF EXISTS streamer_revenue_summary;
DROP TABLE IF EXISTS subscriptions;
DROP TABLE IF EXISTS subscription_tiers;
DROP TABLE IF EXISTS transactions;
DROP TABLE IF EXISTS virtual_currency_balances;
DROP TABLE IF EXISTS wallets;
DROP TABLE IF EXISTS users;

DROP TYPE IF EXISTS subscription_status;
DROP TYPE IF EXISTS transaction_status;
DROP TYPE IF EXISTS transaction_type;

DROP EXTENSION IF EXISTS "uuid-ossp";
