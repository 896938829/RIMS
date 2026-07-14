-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Copyright (c) 2026 ShangBin Wang

ALTER TABLE file_storage_cleanup_queue
  ADD COLUMN IF NOT EXISTS prepare_token VARCHAR(128),
  ADD COLUMN IF NOT EXISTS state VARCHAR(16),
  ADD COLUMN IF NOT EXISTS claim_token VARCHAR(128),
  ADD COLUMN IF NOT EXISTS claim_version BIGINT,
  ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;

UPDATE file_storage_cleanup_queue
SET prepare_token = COALESCE(prepare_token, 'legacy-' || md5(object_key || queued_at::text)),
    state = COALESCE(state, CASE WHEN ready_at IS NULL THEN 'prepared' ELSE 'ready' END),
    claim_version = COALESCE(claim_version, 0)
WHERE prepare_token IS NULL OR state IS NULL OR claim_version IS NULL;

ALTER TABLE file_storage_cleanup_queue
  ALTER COLUMN prepare_token SET NOT NULL,
  ALTER COLUMN state SET DEFAULT 'prepared',
  ALTER COLUMN state SET NOT NULL,
  ALTER COLUMN claim_version SET DEFAULT 0,
  ALTER COLUMN claim_version SET NOT NULL;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'file_storage_cleanup_queue'::regclass
      AND conname = 'file_storage_cleanup_queue_state_check'
  ) THEN
    ALTER TABLE file_storage_cleanup_queue
      ADD CONSTRAINT file_storage_cleanup_queue_state_check
      CHECK (state IN ('prepared', 'ready', 'claimed', 'completed'));
  END IF;
END;
$$;

CREATE INDEX IF NOT EXISTS idx_file_storage_cleanup_queue_claim_lease
ON file_storage_cleanup_queue (claimed_at, object_key)
WHERE state = 'claimed' AND completed_at IS NULL;

CREATE OR REPLACE FUNCTION rims_guard_storage_cleanup_object_key()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  cleanup_state VARCHAR(16);
  cleanup_prepare_token VARCHAR(128);
  transaction_prepare_token TEXT;
BEGIN
  SELECT state, prepare_token
  INTO cleanup_state, cleanup_prepare_token
  FROM file_storage_cleanup_queue
  WHERE object_key = NEW.object_key;

  IF NOT FOUND THEN
    RETURN NEW;
  END IF;

  transaction_prepare_token := current_setting('rims.storage_prepare_token', true);
  IF cleanup_state = 'prepared'
     AND transaction_prepare_token IS NOT NULL
     AND transaction_prepare_token = cleanup_prepare_token THEN
    RETURN NEW;
  END IF;

  RAISE EXCEPTION 'storage cleanup ownership blocks attachment object key';
END;
$$;

DROP TRIGGER IF EXISTS trg_guard_storage_cleanup_object_key ON file_attachments;
CREATE TRIGGER trg_guard_storage_cleanup_object_key
BEFORE INSERT OR UPDATE OF object_key
ON file_attachments
FOR EACH ROW
EXECUTE FUNCTION rims_guard_storage_cleanup_object_key();
