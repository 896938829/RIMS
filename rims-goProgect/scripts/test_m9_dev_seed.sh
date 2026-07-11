#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (c) 2026 ShangBin Wang

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
WORKSPACE_ROOT="${RIMS_WORKSPACE_ROOT:-$(cd "${REPO_ROOT}/.." && pwd)}"
ENV_FILE="${RIMS_ENV_FILE:-${WORKSPACE_ROOT}/.env}"
SEED_SCRIPT="${SCRIPT_DIR}/m9_dev_seed.sh"

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

[[ -f "${ENV_FILE}" ]] || fail "environment file not found: ${ENV_FILE}"
[[ -f "${SEED_SCRIPT}" ]] || fail "seed script not found: ${SEED_SCRIPT}"

GUARD_TMP_DIR="$(mktemp -d)"
cleanup() {
	rm -rf "${GUARD_TMP_DIR}"
}
trap cleanup EXIT

if RIMS_ENV_FILE="${ENV_FILE}" bash "${SEED_SCRIPT}" >/dev/null 2>&1; then
	fail "seed accepted a run without RIMS_ALLOW_DEV_SEED=1"
fi

write_guard_env() {
	local path="$1"
	local app_env="$2"
	local db_host="$3"
	local db_name="$4"
	cat > "${path}" <<EOF
APP_ENV=${app_env}
DB_HOST=${db_host}
DB_PORT=5432
DB_USER=app
DB_PASSWORD=guard-only
DB_NAME=${db_name}
EOF
}

write_guard_env "${GUARD_TMP_DIR}/prod.env" "prod" "127.0.0.1" "appdb"
if RIMS_ALLOW_DEV_SEED=1 RIMS_ENV_FILE="${GUARD_TMP_DIR}/prod.env" bash "${SEED_SCRIPT}" >/dev/null 2>&1; then
	fail "seed accepted APP_ENV=prod"
fi

write_guard_env "${GUARD_TMP_DIR}/remote.env" "dev" "db.example.com" "appdb"
if RIMS_ALLOW_DEV_SEED=1 RIMS_ENV_FILE="${GUARD_TMP_DIR}/remote.env" bash "${SEED_SCRIPT}" >/dev/null 2>&1; then
	fail "seed accepted a remote DB_HOST"
fi

write_guard_env "${GUARD_TMP_DIR}/wrong-db.env" "dev" "127.0.0.1" "production"
if RIMS_ALLOW_DEV_SEED=1 RIMS_ENV_FILE="${GUARD_TMP_DIR}/wrong-db.env" bash "${SEED_SCRIPT}" >/dev/null 2>&1; then
	fail "seed accepted a non-local DB_NAME"
fi

dotenv_probe="${GUARD_TMP_DIR}/dotenv-executed"
write_guard_env "${GUARD_TMP_DIR}/literal.env" "dev" "127.0.0.1" "production"
echo "M9_DOTENV_PROBE=\$(touch ${dotenv_probe})" >> "${GUARD_TMP_DIR}/literal.env"
if RIMS_ALLOW_DEV_SEED=1 RIMS_ENV_FILE="${GUARD_TMP_DIR}/literal.env" bash "${SEED_SCRIPT}" >/dev/null 2>&1; then
	fail "literal dotenv guard unexpectedly reached the database"
fi
[[ ! -e "${dotenv_probe}" ]] || fail "seed executed dotenv content"

while IFS= read -r env_line || [[ -n "${env_line}" ]]; do
	env_line=${env_line%$'\r'}
	[[ "${env_line}" =~ ^[[:space:]]*$ ]] && continue
	[[ "${env_line}" =~ ^[[:space:]]*# ]] && continue
	[[ "${env_line}" =~ ^([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]] ||
		fail "malformed environment record in test configuration"
	env_key=${BASH_REMATCH[1]}
	env_value=${BASH_REMATCH[2]}
	env_length=${#env_value}
	if (( env_length > 1 )); then
		env_first=${env_value:0:1}
		env_last=${env_value:env_length-1:1}
		if [[ "${env_first}" == '"' || "${env_first}" == "'" ]]; then
			[[ "${env_last}" == "${env_first}" ]] ||
				fail "unmatched environment quote in test configuration"
			env_value=${env_value:1:env_length-2}
		fi
	fi
	export "${env_key}=${env_value}"
done < "${ENV_FILE}"

export PGPASSWORD="${DB_PASSWORD:?DB_PASSWORD is required}"
if command -v psql >/dev/null 2>&1; then
	PSQL=(
		psql -X -qAt -v ON_ERROR_STOP=1
		-h "${DB_HOST:-127.0.0.1}"
		-p "${DB_PORT:-5432}"
		-U "${DB_USER:-app}"
		-d "${DB_NAME:-appdb}"
	)
else
	postgres_container="${RIMS_POSTGRES_CONTAINER:-rims-postgres}"
	docker inspect "${postgres_container}" >/dev/null 2>&1 ||
		fail "psql is unavailable and PostgreSQL container ${postgres_container} was not found"
	PSQL=(
		docker exec -i
		-e "PGPASSWORD=${PGPASSWORD}"
		"${postgres_container}"
		psql -X -qAt -v ON_ERROR_STOP=1
		-U "${DB_USER:-app}"
		-d "${DB_NAME:-appdb}"
	)
fi

sql() {
	"${PSQL[@]}" -c "$1"
}

assert_eq() {
	local actual="$1"
	local expected="$2"
	local label="$3"
	[[ "${actual}" == "${expected}" ]] ||
		fail "${label}: expected ${expected}, got ${actual}"
}

fixture_fingerprint() {
	sql "
SELECT concat_ws('|',
  (SELECT count(*) FROM products WHERE code LIKE 'M9-PAGE-%'),
  (SELECT count(*) FROM users WHERE username = 'm9_operator' AND deleted_at IS NULL),
  (SELECT count(*) FROM warehouses WHERE code = 'M9-WH-02' AND deleted_at IS NULL),
  (SELECT count(*) FROM user_warehouses uw JOIN users u ON u.id = uw.user_id WHERE u.username = 'm9_operator' AND uw.deleted_at IS NULL),
  (SELECT count(*) FROM inventories i JOIN products p ON p.id = i.product_id WHERE p.code LIKE 'M9-PAGE-%' AND i.deleted_at IS NULL),
  (SELECT count(*) FROM non_std_inventories WHERE temp_label LIKE 'M9-NS-%' AND deleted_at IS NULL),
  (SELECT count(*) FROM documents WHERE doc_no LIKE 'M9DOC%'),
  (SELECT count(*) FROM document_lines dl JOIN documents d ON d.id = dl.document_id WHERE d.doc_no LIKE 'M9DOC%'),
  (SELECT count(*) FROM inventory_transactions WHERE doc_no LIKE 'M9DOC%')
);"
}

non_fixture_products_before="$(sql "SELECT count(*) FROM products WHERE code NOT LIKE 'M9-%'")"
non_fixture_documents_before="$(sql "SELECT count(*) FROM documents WHERE doc_no NOT LIKE 'M9DOC%' AND remark NOT LIKE 'M9-E2E:%'")"

RIMS_ALLOW_DEV_SEED=1 RIMS_ENV_FILE="${ENV_FILE}" bash "${SEED_SCRIPT}"
first_fingerprint="$(fixture_fingerprint)"
RIMS_ALLOW_DEV_SEED=1 RIMS_ENV_FILE="${ENV_FILE}" bash "${SEED_SCRIPT}"
second_fingerprint="$(fixture_fingerprint)"

assert_eq "${second_fingerprint}" "${first_fingerprint}" "fixture fingerprint after second seed"
assert_eq "$(sql "SELECT count(*) FROM products WHERE code LIKE 'M9-PAGE-%'")" "45" "fixture products"
assert_eq "$(sql "SELECT count(*) FROM users WHERE username = 'm9_operator' AND deleted_at IS NULL")" "1" "fixture operator"
assert_eq "$(sql "SELECT count(*) FROM warehouses WHERE code = 'M9-WH-02' AND deleted_at IS NULL")" "1" "fixture warehouse"
assert_eq "$(sql "SELECT count(*) FROM user_warehouses uw JOIN users u ON u.id = uw.user_id WHERE u.username = 'm9_operator' AND uw.deleted_at IS NULL")" "2" "operator warehouse bindings"
assert_eq "$(sql "SELECT count(*) FROM documents WHERE doc_no LIKE 'M9DOC%'")" "15" "fixture documents"
assert_eq "$(sql "SELECT count(*) FROM inventory_transactions WHERE doc_no LIKE 'M9DOC%'")" "15" "fixture transactions"
assert_eq "$(sql "SELECT count(*) FROM non_std_inventories WHERE temp_label LIKE 'M9-NS-%' AND deleted_at IS NULL")" "25" "fixture non-standard inventory"

for warehouse_code in WH001 M9-WH-02; do
	assert_eq "$(sql "SELECT count(*) FROM inventories i JOIN products p ON p.id = i.product_id JOIN warehouses w ON w.id = i.warehouse_id WHERE p.code LIKE 'M9-PAGE-%' AND w.code = '${warehouse_code}' AND i.deleted_at IS NULL")" "45" "${warehouse_code} fixture inventory"
	low_stock_count="$(sql "SELECT count(*) FROM inventories i JOIN products p ON p.id = i.product_id JOIN warehouses w ON w.id = i.warehouse_id WHERE p.code LIKE 'M9-PAGE-%' AND w.code = '${warehouse_code}' AND i.deleted_at IS NULL AND i.quantity <= i.alert_threshold")"
	(( low_stock_count >= 5 )) || fail "${warehouse_code} low-stock rows: expected at least 5, got ${low_stock_count}"
done

assert_eq "$(sql "SELECT count(*) FROM products WHERE code NOT LIKE 'M9-%'")" "${non_fixture_products_before}" "non-fixture products"
assert_eq "$(sql "SELECT count(*) FROM documents WHERE doc_no NOT LIKE 'M9DOC%' AND remark NOT LIKE 'M9-E2E:%'")" "${non_fixture_documents_before}" "non-fixture documents"

sql "DELETE FROM documents WHERE doc_no = 'M9E2E-RESET-PROBE';
INSERT INTO documents (
  doc_no, doc_type, status, warehouse_id, remark, created_by, updated_by
)
SELECT 'M9E2E-RESET-PROBE', 2, 1, w.id, 'M9-E2E: reset probe', u.id, u.id
FROM warehouses w, users u
WHERE w.code = 'WH001' AND w.deleted_at IS NULL
  AND u.username = 'admin' AND u.deleted_at IS NULL;"
assert_eq "$(sql "SELECT count(*) FROM documents WHERE remark = 'M9-E2E: reset probe'")" "1" "reset probe setup"

RIMS_ALLOW_DEV_SEED=1 RIMS_ENV_FILE="${ENV_FILE}" bash "${SEED_SCRIPT}" --reset
reset_fingerprint="$(fixture_fingerprint)"
assert_eq "${reset_fingerprint}" "${second_fingerprint}" "fixture fingerprint after reset"
assert_eq "$(sql "SELECT count(*) FROM documents WHERE remark = 'M9-E2E: reset probe'")" "0" "M9 E2E reset probe cleanup"
assert_eq "$(sql "SELECT count(*) FROM products WHERE code NOT LIKE 'M9-%'")" "${non_fixture_products_before}" "non-fixture products after reset"
assert_eq "$(sql "SELECT count(*) FROM documents WHERE doc_no NOT LIKE 'M9DOC%' AND remark NOT LIKE 'M9-E2E:%'")" "${non_fixture_documents_before}" "non-fixture documents after reset"

echo "M9 development seed idempotency and reset test passed: ${reset_fingerprint}"
