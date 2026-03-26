-- Transactional outbox table.
-- Events are written here in the SAME DB transaction as the business operation,
-- guaranteeing that an event is never lost even if Kafka is temporarily down.
-- The OutboxRelay worker reads unpublished rows and forwards them to Kafka.
CREATE TABLE event_outbox (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    topic        VARCHAR(128) NOT NULL,
    key          TEXT         NOT NULL,  -- Kafka message key (e.g. streamer_id for ordering)
    payload      JSONB        NOT NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ            -- NULL = pending; set by OutboxRelay after Kafka ACK
);

-- Partial index: only unpublished rows are ever scanned by the relay.
CREATE INDEX idx_event_outbox_unpublished ON event_outbox(created_at)
    WHERE published_at IS NULL;
