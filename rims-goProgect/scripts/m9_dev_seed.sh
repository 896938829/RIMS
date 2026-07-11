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

[[ "${RIMS_ALLOW_DEV_SEED:-}" == "1" ]] || fail "set RIMS_ALLOW_DEV_SEED=1 explicitly"
[[ -f "${ENV_FILE}" ]] || fail "environment file not found: ${ENV_FILE}"
[[ -f "${SQL_FILE}" ]] || fail "SQL file not found: ${SQL_FILE}"
case "${MODE}" in
	seed|--reset) ;;
	*) fail "supported modes are seed and --reset" ;;
esac

set -a
# shellcheck disable=SC1090
source "${ENV_FILE}"
set +a

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
	"${PSQL[@]}" -f - <<'SQL'
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

COMMIT;
SQL
fi

"${PSQL[@]}" -f - < "${SQL_FILE}"

fixture_counts="$("${PSQL[@]}" -qAt -c "
SELECT json_build_object(
  'products', (SELECT count(*) FROM products WHERE code LIKE 'M9-PAGE-%'),
  'operatorUsers', (SELECT count(*) FROM users WHERE username = 'm9_operator' AND deleted_at IS NULL),
  'warehouses', (SELECT count(*) FROM warehouses WHERE code = 'M9-WH-02' AND deleted_at IS NULL),
  'operatorBindings', (SELECT count(*) FROM user_warehouses uw JOIN users u ON u.id = uw.user_id WHERE u.username = 'm9_operator' AND uw.deleted_at IS NULL),
  'inventories', (SELECT count(*) FROM inventories i JOIN products p ON p.id = i.product_id WHERE p.code LIKE 'M9-PAGE-%' AND i.deleted_at IS NULL),
  'nonStandardInventories', (SELECT count(*) FROM non_std_inventories WHERE temp_label LIKE 'M9-NS-%' AND deleted_at IS NULL),
  'documents', (SELECT count(*) FROM documents WHERE doc_no LIKE 'M9DOC%'),
  'transactions', (SELECT count(*) FROM inventory_transactions WHERE doc_no LIKE 'M9DOC%')
)::text;")"
echo "RIMS_M9_FIXTURE_COUNTS ${fixture_counts}"
echo "M9 development fixtures applied to ${DB_NAME} at ${DB_HOST}:${DB_PORT:-5432}."
