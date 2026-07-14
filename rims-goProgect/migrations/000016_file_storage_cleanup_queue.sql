-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Copyright (c) 2026 ShangBin Wang

CREATE TABLE IF NOT EXISTS file_storage_cleanup_queue (
    object_key VARCHAR(512) PRIMARY KEY,
    source_operation VARCHAR(32) NOT NULL,
    primary_error TEXT NOT NULL DEFAULT '',
    cleanup_error TEXT NOT NULL DEFAULT '',
    attempt_count BIGINT NOT NULL DEFAULT 0,
    ready_at TIMESTAMPTZ,
    queued_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT file_storage_cleanup_queue_object_key_check CHECK (object_key <> '')
);

ALTER TABLE file_storage_cleanup_queue
  ADD COLUMN IF NOT EXISTS ready_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_file_storage_cleanup_queue_updated_at
ON file_storage_cleanup_queue (updated_at, object_key);
