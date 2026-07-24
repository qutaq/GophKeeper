CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    login         TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT users_login_unique UNIQUE (login)
);

CREATE TABLE IF NOT EXISTS items (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id       UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    type           SMALLINT NOT NULL,
    encrypted_data BYTEA NOT NULL,
    metadata       JSONB NOT NULL DEFAULT '{}'::jsonb,
    version        BIGINT NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted        BOOLEAN NOT NULL DEFAULT FALSE,
    CONSTRAINT items_type_check CHECK (type BETWEEN 1 AND 5),
    CONSTRAINT items_version_positive CHECK (version > 0)
);

-- Fast sync / listing by owner and recency.
CREATE INDEX IF NOT EXISTS idx_items_owner_updated_at ON items (owner_id, updated_at);
CREATE INDEX IF NOT EXISTS idx_items_owner_version ON items (owner_id, version);
