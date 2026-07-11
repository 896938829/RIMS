#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (c) 2026 ShangBin Wang

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
WORKSPACE_ROOT="${RIMS_WORKSPACE_ROOT:-$(cd "${REPO_ROOT}/.." && pwd)}"
ENV_FILE="${RIMS_ENV_FILE:-${WORKSPACE_ROOT}/.env}"
SQL_FILE="${SCRIPT_DIR}/m9_dev_seed.sql"

fail() {
	echo "M9 development seed refused: $*" >&2
	exit 1
}

[[ "${RIMS_ALLOW_DEV_SEED:-}" == "1" ]] || fail "set RIMS_ALLOW_DEV_SEED=1 explicitly"
[[ -f "${ENV_FILE}" ]] || fail "environment file not found: ${ENV_FILE}"
[[ -f "${SQL_FILE}" ]] || fail "SQL file not found: ${SQL_FILE}"

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

"${PSQL[@]}" -f - < "${SQL_FILE}"

echo "M9 development fixtures applied to ${DB_NAME} at ${DB_HOST}:${DB_PORT:-5432}."
