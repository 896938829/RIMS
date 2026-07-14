-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Copyright (c) 2026 ShangBin Wang
-- Reserve one object-key namespace for disposable M9 fixtures.

CREATE TABLE IF NOT EXISTS rims_dev_fixture_attachment_cleanup (
    object_key VARCHAR(512) PRIMARY KEY,
    source_document_id BIGINT NOT NULL,
    source_doc_no VARCHAR(32) NOT NULL,
    source_remark TEXT NOT NULL,
    claim_token VARCHAR(128),
    claim_version BIGINT NOT NULL DEFAULT 0,
    claimed_at TIMESTAMPTZ,
    queued_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT rims_dev_fixture_attachment_cleanup_source_check CHECK (
      source_doc_no LIKE 'M9DOC%'
      OR source_remark LIKE 'M9-E2E:%'
    )
);

CREATE OR REPLACE FUNCTION rims_guard_fixture_attachment_object_key()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    fixture_binding BOOLEAN := FALSE;
BEGIN
    IF EXISTS (
        SELECT 1
        FROM rims_dev_fixture_attachment_cleanup
        WHERE object_key = NEW.object_key
    ) THEN
        RAISE EXCEPTION 'pending fixture cleanup owns object key; attachment create must retry later';
    END IF;

    IF NEW.business_type = 'doc_attachment' AND NEW.business_id IS NOT NULL THEN
        SELECT EXISTS (
            SELECT 1
            FROM documents
            WHERE id = NEW.business_id
              AND (doc_no LIKE 'M9DOC%' OR remark LIKE 'M9-E2E:%')
        ) INTO fixture_binding;
    END IF;

    IF NEW.object_key LIKE 'm9-e2e/%' AND NOT fixture_binding THEN
        RAISE EXCEPTION 'reserved M9 fixture object key cannot be used by an ordinary attachment';
    END IF;
    IF fixture_binding AND NEW.object_key NOT LIKE 'm9-e2e/%' THEN
        RAISE EXCEPTION 'M9 fixture attachments must use the reserved object-key namespace';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_guard_fixture_attachment_object_key ON file_attachments;
CREATE TRIGGER trg_guard_fixture_attachment_object_key
BEFORE INSERT OR UPDATE OF object_key, business_type, business_id
ON file_attachments
FOR EACH ROW
EXECUTE FUNCTION rims_guard_fixture_attachment_object_key();

CREATE OR REPLACE FUNCTION rims_guard_fixture_cleanup_object_key()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.object_key LIKE 'm9-e2e/%' THEN
        RETURN NEW;
    END IF;
    IF NEW.source_doc_no NOT LIKE 'M9DOC%' AND NEW.source_remark NOT LIKE 'M9-E2E:%' THEN
        RAISE EXCEPTION 'legacy cleanup keys require an M9 fixture source';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_guard_fixture_cleanup_object_key ON rims_dev_fixture_attachment_cleanup;
CREATE TRIGGER trg_guard_fixture_cleanup_object_key
BEFORE INSERT OR UPDATE OF object_key
ON rims_dev_fixture_attachment_cleanup
FOR EACH ROW
EXECUTE FUNCTION rims_guard_fixture_cleanup_object_key();
