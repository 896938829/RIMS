#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (c) 2026 ShangBin Wang

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
WORKSPACE_ROOT="${RIMS_WORKSPACE_ROOT:-$(cd "${REPO_ROOT}/.." && pwd)}"
ENV_FILE="${RIMS_ENV_FILE:-${WORKSPACE_ROOT}/.env}"
SQL_FILE="${SCRIPT_DIR}/m9_dev_seed.sql"
MODE="${1:-seed}"

fail() {
	echo "M9 development seed refused: $*" >&2
	exit 1
}

load_dotenv() {
	local line='' line_number=0 key='' value='' first='' last='' length=0
	while IFS= read -r line || [[ -n "${line}" ]]; do
		line_number=$((line_number + 1))
		line=${line%$'\r'}
		[[ "${line}" =~ ^[[:space:]]*$ ]] && continue
		[[ "${line}" =~ ^[[:space:]]*# ]] && continue
		if [[ ! "${line}" =~ ^([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]]; then
			fail "malformed environment record at line ${line_number}"
		fi
		key=${BASH_REMATCH[1]}
		value=${BASH_REMATCH[2]}
		[[ ! "${value}" =~ [[:cntrl:]] ]] ||
			fail "control character in environment value at line ${line_number}"
		length=${#value}
		if (( length > 0 )); then
			first=${value:0:1}
			last=${value:length-1:1}
			if [[ "${first}" == '"' || "${first}" == "'" ]]; then
				(( length >= 2 )) && [[ "${last}" == "${first}" ]] ||
					fail "unmatched environment quote at line ${line_number}"
				value=${value:1:length-2}
			fi
		fi
		export "${key}=${value}"
	done < "${ENV_FILE}"
}

[[ "${RIMS_ALLOW_DEV_SEED:-}" == "1" ]] || fail "set RIMS_ALLOW_DEV_SEED=1 explicitly"
[[ -f "${ENV_FILE}" ]] || fail "environment file not found: ${ENV_FILE}"
[[ -f "${SQL_FILE}" ]] || fail "SQL file not found: ${SQL_FILE}"
case "${MODE}" in
	seed|--reset) ;;
	*) fail "supported modes are seed and --reset" ;;
esac

load_dotenv

case "${APP_ENV:-}" in
	dev|development|test) ;;
	*) fail "APP_ENV must be dev, development, or test" ;;
esac

case "${DB_HOST:-}" in
	localhost|127.0.0.1|postgres|rims-postgres) ;;
	*) fail "DB_HOST must identify the local PostgreSQL service" ;;
esac

expected_db_name="${RIMS_LOCAL_DB_NAME:-appdb}"
[[ -n "${DB_NAME:-}" ]] || fail "DB_NAME is required"
[[ "${DB_NAME}" == "${expected_db_name}" ]] ||
	fail "DB_NAME must be the configured local RIMS database (${expected_db_name})"

advisory_lock_timeout_ms="${RIMS_M9_ADVISORY_LOCK_TIMEOUT_MS:-5000}"
[[ "${advisory_lock_timeout_ms}" =~ ^[0-9]+$ ]] ||
	fail "RIMS_M9_ADVISORY_LOCK_TIMEOUT_MS must be an integer"
(( advisory_lock_timeout_ms >= 50 && advisory_lock_timeout_ms <= 60000 )) ||
	fail "RIMS_M9_ADVISORY_LOCK_TIMEOUT_MS must be between 50 and 60000"
advisory_lock_timeout="${advisory_lock_timeout_ms}ms"

claim_lease_ms="${RIMS_M9_CLAIM_LEASE_MS:-300000}"
[[ "${claim_lease_ms}" =~ ^[0-9]+$ ]] ||
	fail "RIMS_M9_CLAIM_LEASE_MS must be an integer"
(( claim_lease_ms >= 1000 && claim_lease_ms <= 3600000 )) ||
	fail "RIMS_M9_CLAIM_LEASE_MS must be between 1000 and 3600000"
claim_lease="${claim_lease_ms} milliseconds"

cleanup_timeout_ms="${RIMS_M9_CLEANUP_TIMEOUT_MS:-1000}"
[[ "${cleanup_timeout_ms}" =~ ^[0-9]+$ ]] ||
	fail "RIMS_M9_CLEANUP_TIMEOUT_MS must be an integer"
(( cleanup_timeout_ms >= 50 && cleanup_timeout_ms <= 60000 )) ||
	fail "RIMS_M9_CLEANUP_TIMEOUT_MS must be between 50 and 60000"
cleanup_lock_timeout="${cleanup_timeout_ms}ms"
cleanup_statement_timeout="${cleanup_timeout_ms}ms"

max_tombstones="${RIMS_M9_MAX_TOMBSTONES:-1000}"
[[ "${max_tombstones}" =~ ^[0-9]+$ ]] ||
	fail "RIMS_M9_MAX_TOMBSTONES must be an integer"
(( max_tombstones >= 1 && max_tombstones <= 1000000 )) ||
	fail "RIMS_M9_MAX_TOMBSTONES must be between 1 and 1000000"

max_storage_cleanup_pending="${RIMS_STORAGE_CLEANUP_MAX_PENDING:-100}"
[[ "${max_storage_cleanup_pending}" =~ ^[0-9]+$ ]] ||
	fail "RIMS_STORAGE_CLEANUP_MAX_PENDING must be an integer"
(( max_storage_cleanup_pending >= 1 && max_storage_cleanup_pending <= 1000000 )) ||
	fail "RIMS_STORAGE_CLEANUP_MAX_PENDING must be between 1 and 1000000"

export PGPASSWORD="${DB_PASSWORD:?DB_PASSWORD is required}"
if command -v psql >/dev/null 2>&1; then
	PSQL=(
		psql -X -v ON_ERROR_STOP=1
		-h "${DB_HOST}"
		-p "${DB_PORT:-5432}"
		-U "${DB_USER:-app}"
		-d "${DB_NAME}"
	)
else
	postgres_container="${RIMS_POSTGRES_CONTAINER:-rims-postgres}"
	docker inspect "${postgres_container}" >/dev/null 2>&1 ||
		fail "psql is unavailable and PostgreSQL container ${postgres_container} was not found"
	PSQL=(
		docker exec -i
		-e "PGPASSWORD=${PGPASSWORD}"
		"${postgres_container}"
		psql -X -v ON_ERROR_STOP=1
		-U "${DB_USER:-app}"
		-d "${DB_NAME}"
	)
fi

if [[ "${MODE}" == "--reset" ]]; then
	reset_claim_token="m9-reset-$(date +%s)-$$-${RANDOM}-${RANDOM}"
	reset_claim_active=false
	reset_object_keys=()
	reset_claim_versions=()
	release_reset_claim() {
		local original_status=$?
		local cleanup_failed=false cleanup_output='' encoded_key='' index=0
		trap - EXIT
		if [[ "${reset_claim_active}" == true ]]; then
			for index in "${!reset_object_keys[@]}"; do
				encoded_key="$(printf '%s' "${reset_object_keys[index]}" | base64 | tr -d '\n')"
				if ! cleanup_output="$("${PSQL[@]}" -qAt \
					-v claim_token="${reset_claim_token}" \
					-v claim_version="${reset_claim_versions[index]}" \
					-v object_key_b64="${encoded_key}" \
					-v cleanup_lock_timeout="${cleanup_lock_timeout}" \
					-v cleanup_statement_timeout="${cleanup_statement_timeout}" \
					-f - 2>&1 <<'SQL'
BEGIN;
SELECT set_config('lock_timeout', :'cleanup_lock_timeout', true);
SELECT set_config('statement_timeout', :'cleanup_statement_timeout', true);
SELECT pg_advisory_xact_lock(908130011);
UPDATE rims_dev_fixture_attachment_cleanup
SET claim_token = NULL,
	claimed_at = NULL
WHERE object_key = convert_from(decode(:'object_key_b64', 'base64'), 'UTF8')
  AND claim_token = :'claim_token'
  AND claim_version = :'claim_version'::bigint
  AND completed_at IS NULL;
COMMIT;
SQL
				)"; then
					cleanup_failed=true
					printf 'M9 cleanup release failed for %s after exit status %d: %s\n' \
						"${reset_object_keys[index]}" "${original_status}" "${cleanup_output}" >&2
				fi
			done
		fi
		if [[ "${cleanup_failed}" == true && "${original_status}" -eq 0 ]]; then
			original_status=1
		fi
		exit "${original_status}"
	}
	trap release_reset_claim EXIT
	upload_dir="${UPLOAD_DIR:-./uploads}"
	if [[ "${upload_dir}" != /* ]]; then
		upload_dir="${REPO_ROOT}/${upload_dir#./}"
	fi
	upload_dir="$(realpath -m -- "${upload_dir}")"
	reset_manifest="${upload_dir}/.rims-m9-reset-attachments"
	if [[ -f "${reset_manifest}" ]]; then
		while IFS= read -r manifest_key || [[ -n "${manifest_key}" ]]; do
			[[ -n "${manifest_key}" && "${manifest_key}" != /* ]] ||
				fail "unsafe untrusted M9 reset manifest"
			if [[ "/${manifest_key}/" == *"/../"* ||
				"/${manifest_key}/" == *"/./"* ]]; then
				fail "unsafe untrusted M9 reset manifest"
			fi
		done < "${reset_manifest}"
		echo "Ignoring untrusted M9 reset manifest; deletion ownership is derived from PostgreSQL." >&2
	fi

	reset_sql_output="$("${PSQL[@]}" -qAt -v claim_token="${reset_claim_token}" -v claim_lease="${claim_lease}" -v advisory_lock_timeout="${advisory_lock_timeout}" -f - <<'SQL'
BEGIN;

SELECT set_config('lock_timeout', :'advisory_lock_timeout', true);
SELECT pg_advisory_xact_lock(908130011);
LOCK TABLE documents, inventory_transactions, file_attachments,
  file_storage_cleanup_queue
  IN SHARE ROW EXCLUSIVE MODE;

CREATE TEMP TABLE m9_reset_documents (
    id BIGINT PRIMARY KEY,
    doc_no VARCHAR(32) NOT NULL,
    remark TEXT NOT NULL
) ON COMMIT DROP;

CREATE TEMP TABLE m9_reset_attachment_keys (
    object_key VARCHAR(512) PRIMARY KEY
) ON COMMIT DROP;

CREATE TABLE IF NOT EXISTS rims_dev_fixture_attachment_cleanup (
    object_key VARCHAR(512) PRIMARY KEY,
    source_document_id BIGINT NOT NULL,
    source_doc_no VARCHAR(32) NOT NULL,
    source_remark TEXT NOT NULL,
    claim_token VARCHAR(128),
    claim_version BIGINT NOT NULL DEFAULT 0,
    claimed_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    queued_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT rims_dev_fixture_attachment_cleanup_source_check CHECK (
      source_doc_no LIKE 'M9DOC%'
      OR source_remark LIKE 'M9-E2E:%'
    )
);

ALTER TABLE rims_dev_fixture_attachment_cleanup
  ADD COLUMN IF NOT EXISTS claim_token VARCHAR(128),
  ADD COLUMN IF NOT EXISTS claim_version BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;

INSERT INTO m9_reset_documents (id, doc_no, remark)
SELECT id, doc_no, COALESCE(remark, '')
FROM documents
WHERE doc_no LIKE 'M9DOC%'
   OR remark LIKE 'M9-E2E:%';

INSERT INTO m9_reset_attachment_keys (object_key)
SELECT fa.object_key
FROM file_attachments AS fa
WHERE fa.business_type = 'doc_attachment'
  AND fa.business_id IN (SELECT id FROM m9_reset_documents);

INSERT INTO rims_dev_fixture_attachment_cleanup (
  object_key,
  source_document_id,
  source_doc_no,
  source_remark
)
SELECT
  fa.object_key,
  d.id,
  d.doc_no,
  d.remark
FROM file_attachments AS fa
JOIN m9_reset_documents AS d ON d.id = fa.business_id
WHERE fa.business_type = 'doc_attachment'
ON CONFLICT (object_key) DO NOTHING;

DELETE FROM file_attachments
WHERE business_type = 'doc_attachment'
  AND business_id IN (SELECT id FROM m9_reset_documents);

DELETE FROM inventory_transactions
WHERE doc_id IN (SELECT id FROM m9_reset_documents)
   OR doc_no IN (SELECT doc_no FROM m9_reset_documents);

DELETE FROM document_lines
WHERE document_id IN (SELECT id FROM m9_reset_documents);

DELETE FROM documents
WHERE id IN (SELECT id FROM m9_reset_documents);

DELETE FROM non_std_inventories
WHERE temp_label LIKE 'M9-NS-%';

DELETE FROM inventories AS i
USING products AS p
WHERE i.product_id = p.id
  AND p.code LIKE 'M9-PAGE-%';

DELETE FROM products
WHERE code LIKE 'M9-PAGE-%';

DELETE FROM user_warehouses
WHERE user_id IN (SELECT id FROM users WHERE username = 'm9_operator')
   OR warehouse_id IN (SELECT id FROM warehouses WHERE code = 'M9-WH-02');

DELETE FROM users
WHERE username = 'm9_operator';

DELETE FROM warehouses
WHERE code = 'M9-WH-02';

UPDATE rims_dev_fixture_attachment_cleanup AS pending
SET claim_token = :'claim_token',
    claim_version = pending.claim_version + 1,
    claimed_at = CURRENT_TIMESTAMP
WHERE pending.completed_at IS NULL
  AND (
    pending.claim_token IS NULL
    OR pending.claimed_at IS NULL
    OR pending.claimed_at < CURRENT_TIMESTAMP - :'claim_lease'::interval
  );

SELECT 'RIMS_M9_RESET_OBJECT_KEY ' || replace(
  encode(convert_to(object_key, 'UTF8'), 'base64'),
  E'\n',
  ''
) || ' ' || claim_version
FROM rims_dev_fixture_attachment_cleanup
WHERE claim_token = :'claim_token'
ORDER BY object_key;

COMMIT;
SQL
)"
	reset_claim_active=true
	while IFS= read -r reset_line; do
		case "${reset_line}" in
			'RIMS_M9_RESET_OBJECT_KEY '*)
				claim_record=${reset_line#RIMS_M9_RESET_OBJECT_KEY }
				encoded_key=${claim_record% *}
				claim_version=${claim_record##* }
				[[ "${claim_version}" =~ ^[1-9][0-9]*$ ]] ||
					fail "invalid database-produced M9 attachment claim version"
				object_key="$(printf '%s' "${encoded_key}" | base64 --decode)" ||
					fail "invalid database-produced M9 attachment key encoding"
				reset_object_keys+=("${object_key}")
				reset_claim_versions+=("${claim_version}")
				;;
		esac
	done <<< "${reset_sql_output}"
	for object_key in "${reset_object_keys[@]}"; do
		[[ -n "${object_key}" && "${object_key}" != /* ]] ||
			fail "unsafe database-produced M9 attachment object key"
		if [[ "/${object_key}/" == *"/../"* ||
			"/${object_key}/" == *"/./"* ]]; then
			fail "unsafe database-produced M9 attachment object key"
		fi
		object_path="$(realpath -m -- "${upload_dir}/${object_key}")"
		[[ "${object_path}" == "${upload_dir}/"* ]] ||
			fail "database-produced M9 attachment escaped UPLOAD_DIR"
	done
	if (( ${#reset_object_keys[@]} > 0 )); then
		active_reference_output="$("${PSQL[@]}" -qAt -v claim_token="${reset_claim_token}" -v advisory_lock_timeout="${advisory_lock_timeout}" -f - <<'SQL'
BEGIN;
SELECT set_config('lock_timeout', :'advisory_lock_timeout', true);
SELECT pg_advisory_xact_lock(908130011);
LOCK TABLE file_attachments IN SHARE ROW EXCLUSIVE MODE;
SELECT 'RIMS_M9_ACTIVE_ATTACHMENT_REFERENCE_COUNT ' || count(*)
FROM rims_dev_fixture_attachment_cleanup AS pending
JOIN file_attachments AS attachment
  ON attachment.object_key = pending.object_key
WHERE pending.claim_token = :'claim_token'
  AND pending.completed_at IS NULL
  AND attachment.deleted_at IS NULL;
COMMIT;
SQL
)"
		active_reference_count=-1
		while IFS= read -r active_reference_line; do
			case "${active_reference_line}" in
				'RIMS_M9_ACTIVE_ATTACHMENT_REFERENCE_COUNT '*)
					active_reference_count=${active_reference_line#RIMS_M9_ACTIVE_ATTACHMENT_REFERENCE_COUNT }
					;;
			esac
		done <<< "${active_reference_output}"
		[[ "${active_reference_count}" =~ ^[0-9]+$ && "${active_reference_count}" -eq 0 ]] ||
			fail "an active attachment still references an M9 pending object"
	fi
	for index in "${!reset_object_keys[@]}"; do
		object_key="${reset_object_keys[index]}"
		encoded_key="$(printf '%s' "${object_key}" | base64 | tr -d '\n')"
		delete_entitlement_output="$("${PSQL[@]}" -qAt \
			-v claim_token="${reset_claim_token}" \
			-v claim_version="${reset_claim_versions[index]}" \
			-v claim_lease="${claim_lease}" \
			-v object_key_b64="${encoded_key}" \
			-v advisory_lock_timeout="${advisory_lock_timeout}" \
			-f - <<'SQL'
BEGIN;
SELECT set_config('lock_timeout', :'advisory_lock_timeout', true);
SELECT pg_advisory_xact_lock(908130011);
LOCK TABLE file_attachments IN SHARE ROW EXCLUSIVE MODE;
WITH entitled AS (
  UPDATE rims_dev_fixture_attachment_cleanup AS pending
  SET claimed_at = CURRENT_TIMESTAMP
  WHERE pending.object_key = convert_from(decode(:'object_key_b64', 'base64'), 'UTF8')
    AND pending.claim_token = :'claim_token'
    AND pending.claim_version = :'claim_version'::bigint
    AND pending.completed_at IS NULL
    AND pending.claimed_at >= CURRENT_TIMESTAMP - :'claim_lease'::interval
    AND NOT EXISTS (
      SELECT 1
      FROM file_attachments AS attachment
      WHERE attachment.object_key = pending.object_key
        AND attachment.deleted_at IS NULL
    )
  RETURNING 1
)
SELECT 'RIMS_M9_DELETE_ENTITLEMENT ' || count(*) FROM entitled;
COMMIT;
SQL
)"
		delete_entitlement_count=-1
		while IFS= read -r entitlement_line; do
			case "${entitlement_line}" in
				'RIMS_M9_DELETE_ENTITLEMENT '*)
					delete_entitlement_count=${entitlement_line#RIMS_M9_DELETE_ENTITLEMENT }
					;;
			esac
		done <<< "${delete_entitlement_output}"
		[[ "${delete_entitlement_count}" == 1 ]] ||
			fail "lost M9 attachment delete entitlement for ${object_key}; physical file was not removed"
		object_path="$(realpath -m -- "${upload_dir}/${object_key}")"
		rm -f -- "${object_path}" ||
			fail "failed to remove M9 attachment ${object_key}; cleanup responsibility remains in PostgreSQL"
	done
	namespace_attachment_files=0
	for object_key in "${reset_object_keys[@]}"; do
		object_path="$(realpath -m -- "${upload_dir}/${object_key}")"
		[[ ! -e "${object_path}" ]] ||
			namespace_attachment_files=$((namespace_attachment_files + 1))
	done
	[[ "${namespace_attachment_files}" -eq 0 ]] ||
		fail "M9 attachment files remain after reset; cleanup responsibility remains in PostgreSQL"
	for index in "${!reset_object_keys[@]}"; do
		encoded_key="$(printf '%s' "${reset_object_keys[index]}" | base64 | tr -d '\n')"
		finalize_claim_output="$("${PSQL[@]}" -qAt \
			-v claim_token="${reset_claim_token}" \
			-v claim_version="${reset_claim_versions[index]}" \
			-v object_key_b64="${encoded_key}" \
			-v advisory_lock_timeout="${advisory_lock_timeout}" \
			-f - <<'SQL'
BEGIN;
SELECT set_config('lock_timeout', :'advisory_lock_timeout', true);
SELECT pg_advisory_xact_lock(908130011);
LOCK TABLE file_attachments IN SHARE ROW EXCLUSIVE MODE;
WITH finalized AS (
  UPDATE rims_dev_fixture_attachment_cleanup AS pending
  SET completed_at = CURRENT_TIMESTAMP,
      claim_token = NULL,
      claimed_at = NULL
  WHERE pending.object_key = convert_from(decode(:'object_key_b64', 'base64'), 'UTF8')
    AND pending.claim_token = :'claim_token'
    AND pending.claim_version = :'claim_version'::bigint
    AND pending.completed_at IS NULL
    AND NOT EXISTS (
      SELECT 1
      FROM file_attachments AS attachment
      WHERE attachment.object_key = pending.object_key
        AND attachment.deleted_at IS NULL
    )
  RETURNING 1
)
SELECT 'RIMS_M9_FINALIZED_TOMBSTONE ' || count(*) FROM finalized;
COMMIT;
SQL
		)"
		finalized_tombstone_count=-1
		while IFS= read -r finalized_line; do
			case "${finalized_line}" in
				'RIMS_M9_FINALIZED_TOMBSTONE '*)
					finalized_tombstone_count=${finalized_line#RIMS_M9_FINALIZED_TOMBSTONE }
					;;
			esac
		done <<< "${finalize_claim_output}"
		[[ "${finalized_tombstone_count}" == 1 ]] ||
			fail "lost M9 attachment finalize entitlement for ${reset_object_keys[index]}"
	done
	finalize_output="$("${PSQL[@]}" -qAt -v claim_token="${reset_claim_token}" -v advisory_lock_timeout="${advisory_lock_timeout}" -f - <<'SQL'
BEGIN;
SELECT set_config('lock_timeout', :'advisory_lock_timeout', true);
SELECT pg_advisory_xact_lock(908130011);
LOCK TABLE documents, inventory_transactions, file_attachments
  IN SHARE ROW EXCLUSIVE MODE;
SELECT 'RIMS_M9_CLAIMED_PENDING_COUNT ' || count(*)
FROM rims_dev_fixture_attachment_cleanup
WHERE completed_at IS NULL
  AND claim_token = :'claim_token';
SELECT 'RIMS_M9_PENDING_ATTACHMENT_COUNT ' || count(*)
FROM rims_dev_fixture_attachment_cleanup
WHERE completed_at IS NULL;
SELECT 'RIMS_M9_TOMBSTONE_ATTACHMENT_COUNT ' || count(*)
FROM rims_dev_fixture_attachment_cleanup
WHERE completed_at IS NOT NULL;
SELECT 'RIMS_FILE_STORAGE_CLEANUP_PENDING_COUNT ' || count(*)
FROM file_storage_cleanup_queue;
SELECT 'RIMS_M9_RESET_COUNTS ' || json_build_object(
  'namespaceDocuments', (
    SELECT count(*)
    FROM documents
    WHERE doc_no LIKE 'M9DOC%'
       OR remark LIKE 'M9-E2E:%'
  ),
  'namespaceTransactions', (
    SELECT count(*)
    FROM inventory_transactions AS transaction
    WHERE transaction.doc_no LIKE 'M9DOC%'
       OR EXISTS (
         SELECT 1
         FROM documents AS document
         WHERE document.id = transaction.doc_id
           AND (
             document.doc_no LIKE 'M9DOC%'
             OR document.remark LIKE 'M9-E2E:%'
           )
       )
  ),
  'namespaceAttachments', (
    SELECT count(*)
    FROM file_attachments AS attachment
    JOIN documents AS document ON document.id = attachment.business_id
    WHERE attachment.business_type = 'doc_attachment'
      AND (
        document.doc_no LIKE 'M9DOC%'
        OR document.remark LIKE 'M9-E2E:%'
      )
  )
)::text;
COMMIT;
SQL
)"
	claimed_pending_count=-1
	pending_attachment_count=-1
	tombstone_attachment_count=-1
	storage_cleanup_pending_count=-1
	reset_counts_seen=false
	reset_namespace_documents=-1
	reset_namespace_transactions=-1
	reset_namespace_attachments=-1
	reset_counts_line=''
	while IFS= read -r finalize_line; do
		case "${finalize_line}" in
			'RIMS_M9_CLAIMED_PENDING_COUNT '*)
				claimed_pending_count=${finalize_line#RIMS_M9_CLAIMED_PENDING_COUNT }
				;;
			'RIMS_M9_PENDING_ATTACHMENT_COUNT '*)
				pending_attachment_count=${finalize_line#RIMS_M9_PENDING_ATTACHMENT_COUNT }
				;;
			'RIMS_M9_TOMBSTONE_ATTACHMENT_COUNT '*)
				tombstone_attachment_count=${finalize_line#RIMS_M9_TOMBSTONE_ATTACHMENT_COUNT }
				;;
			'RIMS_FILE_STORAGE_CLEANUP_PENDING_COUNT '*)
				storage_cleanup_pending_count=${finalize_line#RIMS_FILE_STORAGE_CLEANUP_PENDING_COUNT }
				;;
			'RIMS_M9_RESET_COUNTS '*)
				reset_counts_json=${finalize_line#RIMS_M9_RESET_COUNTS }
				if [[ "${reset_counts_json}" =~ \"namespaceDocuments\"[[:space:]]*:[[:space:]]*([0-9]+).*\"namespaceTransactions\"[[:space:]]*:[[:space:]]*([0-9]+).*\"namespaceAttachments\"[[:space:]]*:[[:space:]]*([0-9]+) ]]; then
					reset_namespace_documents=${BASH_REMATCH[1]}
					reset_namespace_transactions=${BASH_REMATCH[2]}
					reset_namespace_attachments=${BASH_REMATCH[3]}
					reset_counts_seen=true
					reset_counts_line=${finalize_line}
				else
					fail "invalid M9 reset database count evidence"
				fi
				;;
		esac
	done <<< "${finalize_output}"
	[[ "${claimed_pending_count}" =~ ^[0-9]+$ && "${claimed_pending_count}" -eq 0 ]] ||
		fail "this reset still owns incomplete attachment cleanup responsibility"
	[[ "${pending_attachment_count}" =~ ^[0-9]+$ && "${pending_attachment_count}" -eq 0 ]] ||
		fail "M9 attachment cleanup responsibility remains after reset"
	[[ "${tombstone_attachment_count}" =~ ^[0-9]+$ ]] ||
		fail "M9 reset omitted retained attachment tombstone evidence"
	[[ "${storage_cleanup_pending_count}" =~ ^[0-9]+$ ]] ||
		fail "M9 reset omitted pending file storage cleanup evidence"
	(( tombstone_attachment_count <= max_tombstones )) ||
		fail "M9 cleanup tombstone count ${tombstone_attachment_count} exceeds configured limit ${max_tombstones}"
	(( storage_cleanup_pending_count <= max_storage_cleanup_pending )) ||
		fail "file storage cleanup pending count ${storage_cleanup_pending_count} exceeds configured limit ${max_storage_cleanup_pending}"
	[[ "${reset_counts_seen}" == true ]] ||
		fail "M9 reset omitted final database count evidence"
	[[ "${reset_namespace_documents}" -eq 0 &&
		"${reset_namespace_transactions}" -eq 0 &&
		"${reset_namespace_attachments}" -eq 0 ]] ||
		fail "M9 reset database namespace cleanup is incomplete"
	printf '%s\n' "${reset_counts_line}"
	printf 'RIMS_M9_CLEANUP_COUNTS {"pending":%d,"tombstones":%d,"tombstoneLimit":%d,"storagePending":%d,"storagePendingLimit":%d}\n' \
		"${pending_attachment_count}" "${tombstone_attachment_count}" "${max_tombstones}" \
		"${storage_cleanup_pending_count}" "${max_storage_cleanup_pending}"
	reset_claim_active=false
	trap - EXIT
	rm -f -- "${reset_manifest}"
fi

"${PSQL[@]}" -v advisory_lock_timeout="${advisory_lock_timeout}" -f - < "${SQL_FILE}"

namespace_attachment_files="${namespace_attachment_files:-0}"
fixture_counts="$("${PSQL[@]}" -qAt -v namespace_attachment_files="${namespace_attachment_files}" -f - <<'SQL'
SELECT json_build_object(
  'database', current_database(),
  'products', (SELECT count(*) FROM products WHERE code LIKE 'M9-PAGE-%'),
  'operatorUsers', (SELECT count(*) FROM users WHERE username = 'm9_operator' AND deleted_at IS NULL),
  'warehouses', (SELECT count(*) FROM warehouses WHERE code = 'M9-WH-02' AND deleted_at IS NULL),
  'operatorBindings', (SELECT count(*) FROM user_warehouses uw JOIN users u ON u.id = uw.user_id WHERE u.username = 'm9_operator' AND uw.deleted_at IS NULL),
  'inventories', (SELECT count(*) FROM inventories i JOIN products p ON p.id = i.product_id WHERE p.code LIKE 'M9-PAGE-%' AND i.deleted_at IS NULL),
  'nonStandardInventories', (SELECT count(*) FROM non_std_inventories WHERE temp_label LIKE 'M9-NS-%' AND deleted_at IS NULL),
  'documents', (SELECT count(*) FROM documents WHERE doc_no LIKE 'M9DOC%'),
  'transactions', (SELECT count(*) FROM inventory_transactions WHERE doc_no LIKE 'M9DOC%'),
  'fixtureStockQuantity', (
    SELECT coalesce(sum(i.quantity), 0)
    FROM inventories i
    JOIN products p ON p.id = i.product_id
    WHERE p.code LIKE 'M9-PAGE-%' AND i.deleted_at IS NULL
  ),
  'namespaceDocuments', (
    SELECT count(*) FROM documents WHERE remark LIKE 'M9-E2E:%'
  ),
  'namespaceTransactions', (
    SELECT count(*)
    FROM inventory_transactions t
    JOIN documents d ON d.id = t.doc_id
    WHERE d.remark LIKE 'M9-E2E:%'
  ),
  'namespaceAttachments', (
    SELECT count(*)
    FROM file_attachments fa
    JOIN documents d ON d.id = fa.business_id
    WHERE fa.business_type = 'doc_attachment'
      AND d.remark LIKE 'M9-E2E:%'
  ),
  'namespaceAttachmentFiles', :'namespace_attachment_files'::bigint,
  'fixtureAttachments', (
    SELECT count(*)
    FROM file_attachments fa
    JOIN documents d ON d.id = fa.business_id
    WHERE fa.business_type = 'doc_attachment'
      AND d.doc_no LIKE 'M9DOC%'
  )
)::text;
SQL
)"
echo "RIMS_M9_FIXTURE_COUNTS ${fixture_counts}"
echo "M9 development fixtures applied to ${DB_NAME} at ${DB_HOST}:${DB_PORT:-5432}."
