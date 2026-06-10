-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Copyright (c) 2026 ShangBin Wang

CREATE TABLE IF NOT EXISTS idempotency_keys (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    user_id BIGINT NOT NULL,
    scope VARCHAR(255) NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL,
    request_hash VARCHAR(64) NOT NULL,
    state VARCHAR(32) NOT NULL,
    status_code INTEGER,
    response_body JSONB,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_idempotency_user_scope_key
    ON idempotency_keys (user_id, scope, idempotency_key)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_idempotency_expires_at
    ON idempotency_keys (expires_at)
    WHERE deleted_at IS NULL;
