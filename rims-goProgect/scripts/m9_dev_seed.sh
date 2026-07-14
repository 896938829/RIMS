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
	upload_dir="${UPLOAD_DIR:-./uploads}"
	if [[ "${upload_dir}" != /* ]]; then
		upload_dir="${REPO_ROOT}/${upload_dir#./}"
	fi
	upload_dir="$(realpath -m -- "${upload_dir}")"
	reset_manifest="${upload_dir}/.rims-m9-reset-attachments"
	reset_object_key_output="$("${PSQL[@]}" -qAt -c "
SELECT fa.object_key
FROM file_attachments AS fa
JOIN documents AS d ON d.id = fa.business_id
WHERE fa.business_type = 'doc_attachment'
  AND (d.doc_no LIKE 'M9DOC%' OR d.remark LIKE 'M9-E2E:%');")"
	reset_object_keys=()
	if [[ -f "${reset_manifest}" ]]; then
		mapfile -t reset_object_keys < "${reset_manifest}"
	fi
	if [[ -n "${reset_object_key_output}" ]]; then
		while IFS= read -r object_key; do
			reset_object_keys+=("${object_key}")
		done <<< "${reset_object_key_output}"
	fi
	for object_key in "${reset_object_keys[@]}"; do
		[[ -n "${object_key}" && "${object_key}" != /* ]] ||
			fail "unsafe M9 attachment object key"
		if [[ "/${object_key}/" == *"/../"* ]]; then
			fail "unsafe M9 attachment object key"
		fi
		object_path="$(realpath -m -- "${upload_dir}/${object_key}")"
		[[ "${object_path}" == "${upload_dir}/"* ]] ||
			fail "M9 attachment escaped UPLOAD_DIR"
	done
	if (( ${#reset_object_keys[@]} > 0 )); then
		mkdir -p -- "${upload_dir}"
		reset_manifest_tmp="$(mktemp "${upload_dir}/.rims-m9-reset-attachments.XXXXXX")"
		printf '%s\n' "${reset_object_keys[@]}" | sort -u > "${reset_manifest_tmp}"
		mv -f -- "${reset_manifest_tmp}" "${reset_manifest}"
	fi

	"${PSQL[@]}" -qAt -f - <<'SQL'
BEGIN;

CREATE TEMP TABLE m9_reset_documents (
    id BIGINT PRIMARY KEY,
    doc_no VARCHAR(32) NOT NULL
) ON COMMIT DROP;

INSERT INTO m9_reset_documents (id, doc_no)
SELECT id, doc_no
FROM documents
WHERE doc_no LIKE 'M9DOC%'
   OR remark LIKE 'M9-E2E:%';

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

SELECT 'RIMS_M9_RESET_COUNTS ' || json_build_object(
  'namespaceDocuments', (
    SELECT count(*) FROM documents
    WHERE id IN (SELECT id FROM m9_reset_documents)
  ),
  'namespaceTransactions', (
    SELECT count(*) FROM inventory_transactions
    WHERE doc_id IN (SELECT id FROM m9_reset_documents)
       OR doc_no IN (SELECT doc_no FROM m9_reset_documents)
  ),
  'namespaceAttachments', (
    SELECT count(*) FROM file_attachments
    WHERE business_type = 'doc_attachment'
      AND business_id IN (SELECT id FROM m9_reset_documents)
  )
)::text;

COMMIT;
SQL
	for object_key in "${reset_object_keys[@]}"; do
		object_path="$(realpath -m -- "${upload_dir}/${object_key}")"
		rm -f -- "${object_path}" ||
			fail "failed to remove M9 attachment ${object_key}; retry reset using ${reset_manifest}"
	done
	namespace_attachment_files=0
	for object_key in "${reset_object_keys[@]}"; do
		object_path="$(realpath -m -- "${upload_dir}/${object_key}")"
		[[ ! -e "${object_path}" ]] ||
			namespace_attachment_files=$((namespace_attachment_files + 1))
	done
	[[ "${namespace_attachment_files}" -eq 0 ]] ||
		fail "M9 attachment files remain after reset; retry using ${reset_manifest}"
	rm -f -- "${reset_manifest}"
fi

"${PSQL[@]}" -f - < "${SQL_FILE}"

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
