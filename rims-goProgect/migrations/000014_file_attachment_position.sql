-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Copyright (c) 2026 ShangBin Wang
-- Migration: stable ordering for file attachments

ALTER TABLE file_attachments
    ADD COLUMN IF NOT EXISTS position INTEGER NOT NULL DEFAULT 0;

WITH ranked AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY business_type, business_id
            ORDER BY created_at ASC, id ASC
        ) - 1 AS position
    FROM file_attachments
)
UPDATE file_attachments AS attachment
SET position = ranked.position
FROM ranked
WHERE attachment.id = ranked.id;

CREATE INDEX IF NOT EXISTS idx_file_business_position
    ON file_attachments (business_type, business_id, position, id)
    WHERE deleted_at IS NULL;
