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

assert_eq() {
	local actual="$1"
	local expected="$2"
	local label="$3"
	[[ "${actual}" == "${expected}" ]] ||
		fail "${label}: expected ${expected}, got ${actual}"
}

[[ -f "${ENV_FILE}" ]] || fail "environment file not found: ${ENV_FILE}"
[[ -f "${SEED_SCRIPT}" ]] || fail "seed script not found: ${SEED_SCRIPT}"
SEED_SQL="${SCRIPT_DIR}/m9_dev_seed.sql"
[[ -f "${SEED_SQL}" ]] || fail "seed SQL not found: ${SEED_SQL}"
CLEANUP_GUARD_MIGRATION="${REPO_ROOT}/migrations/000015_fixture_attachment_cleanup_guard.sql"
[[ -f "${CLEANUP_GUARD_MIGRATION}" ]] || fail "fixture attachment cleanup guard migration is missing"
STORAGE_CLEANUP_MIGRATION="${REPO_ROOT}/migrations/000016_file_storage_cleanup_queue.sql"
[[ -f "${STORAGE_CLEANUP_MIGRATION}" ]] || fail "file storage cleanup queue migration is missing"
for storage_fragment in \
	'CREATE TABLE IF NOT EXISTS file_storage_cleanup_queue' \
	'primary_error TEXT NOT NULL' \
	'cleanup_error TEXT NOT NULL' \
	'attempt_count BIGINT NOT NULL' \
	'ADD COLUMN IF NOT EXISTS ready_at'; do
	grep -Fq "${storage_fragment}" "${STORAGE_CLEANUP_MIGRATION}" ||
		fail "file storage cleanup migration missing ${storage_fragment}"
done
for guard_fragment in \
	'CREATE TABLE IF NOT EXISTS rims_dev_fixture_attachment_cleanup' \
	'BEFORE INSERT OR UPDATE OF object_key' \
	"object_key LIKE 'm9-e2e/%'" \
	'RAISE EXCEPTION' \
	'rims_dev_fixture_attachment_cleanup'; do
	grep -Fq "${guard_fragment}" "${CLEANUP_GUARD_MIGRATION}" ||
		fail "fixture attachment cleanup guard migration missing ${guard_fragment}"
done
for lease_fragment in \
	'RIMS_M9_CLAIM_LEASE_MS' \
	"claimed_at < CURRENT_TIMESTAMP - :'claim_lease'::interval" \
	"pending.claim_version = :'claim_version'::bigint" \
	"pending.claimed_at >= CURRENT_TIMESTAMP - :'claim_lease'::interval" \
	'RIMS_M9_DELETE_ENTITLEMENT' \
	'RIMS_M9_FINALIZED_TOMBSTONE' \
	'RIMS_M9_TOMBSTONE_ATTACHMENT_COUNT' \
	'RIMS_FILE_STORAGE_CLEANUP_PENDING_COUNT' \
	'RIMS_M9_MAX_TOMBSTONES' \
	'RIMS_STORAGE_CLEANUP_MAX_PENDING' \
	'completed_at IS NULL' \
	"set_config('statement_timeout', :'cleanup_statement_timeout', true)" \
	'M9 cleanup release failed'; do
	grep -Fq "${lease_fragment}" "${SEED_SCRIPT}" ||
		fail "reset cleanup lease protocol missing ${lease_fragment}"
done
for lock_file in "${SEED_SCRIPT}" "${SEED_SQL}"; do
	grep -Fq 'pg_advisory_xact_lock(908130011)' "${lock_file}" ||
		fail "fixture namespace advisory lock missing from ${lock_file}"
	lock_count="$(grep -Fc 'pg_advisory_xact_lock(908130011)' "${lock_file}")"
	timeout_count="$(grep -Fc "set_config('lock_timeout'" "${lock_file}")"
	assert_eq "${timeout_count}" "${lock_count}" "fixture advisory lock deadline coverage in ${lock_file}"
done
if grep -Eq '^DELETE FROM rims_dev_fixture_attachment_cleanup;[[:space:]]*$' "${SEED_SCRIPT}"; then
	fail "reset still contains an unconditional pending-table delete"
fi

GUARD_TMP_DIR="$(mktemp -d)"
RESET_PROBE_FIXTURE_FILE=''
RESET_PROBE_NON_FIXTURE_FILE=''
LOCK_HOLDER_PID=''
POST_ENTITLEMENT_WORKER_PID=''
POST_ENTITLEMENT_PROCEED=''
cleanup() {
	if [[ -n "${LOCK_HOLDER_PID}" ]]; then
		kill "${LOCK_HOLDER_PID}" >/dev/null 2>&1 || true
		wait "${LOCK_HOLDER_PID}" >/dev/null 2>&1 || true
	fi
	if [[ -n "${POST_ENTITLEMENT_WORKER_PID}" ]]; then
		[[ -z "${POST_ENTITLEMENT_PROCEED}" ]] || touch "${POST_ENTITLEMENT_PROCEED}"
		kill "${POST_ENTITLEMENT_WORKER_PID}" >/dev/null 2>&1 || true
		wait "${POST_ENTITLEMENT_WORKER_PID}" >/dev/null 2>&1 || true
	fi
	for probe_file in "${RESET_PROBE_FIXTURE_FILE}" "${RESET_PROBE_NON_FIXTURE_FILE}"; do
		[[ -z "${probe_file}" || ! -e "${probe_file}" ]] || rm -f -- "${probe_file}"
	done
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

manifest_guard_dir="${GUARD_TMP_DIR}/manifest-guard"
manifest_guard_bin="${manifest_guard_dir}/bin"
manifest_guard_uploads="${manifest_guard_dir}/uploads"
mkdir -p \
	"${manifest_guard_bin}" \
	"${manifest_guard_uploads}/ordinary" \
	"${manifest_guard_uploads}/m9-e2e"
printf 'ordinary upload\n' > "${manifest_guard_uploads}/ordinary/keep.bin"
printf 'same directory, not database-owned\n' > "${manifest_guard_uploads}/m9-e2e/ordinary.bin"
cat > "${manifest_guard_uploads}/.rims-m9-reset-attachments" <<'EOF'
ordinary/keep.bin
m9-e2e/ordinary.bin
EOF
cat > "${manifest_guard_bin}/psql" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
input="$(cat)"
if [[ " $* " == *" -v namespace_attachment_files="* ]]; then
	printf '%s\n' '{"namespaceAttachmentFiles":0}'
elif [[ "${input}" == *"INSERT INTO m9_reset_documents"* ]]; then
	printf '%s\n' 'RIMS_M9_RESET_COUNTS {"namespaceDocuments":0,"namespaceTransactions":0,"namespaceAttachments":0}'
elif [[ "${input}" == *"RIMS_M9_CLAIMED_PENDING_COUNT"* ]]; then
	printf '%s\n' 'RIMS_M9_CLAIMED_PENDING_COUNT 0'
	printf '%s\n' 'RIMS_M9_PENDING_ATTACHMENT_COUNT 0'
	printf '%s\n' 'RIMS_M9_TOMBSTONE_ATTACHMENT_COUNT 0'
	printf '%s\n' 'RIMS_FILE_STORAGE_CLEANUP_PENDING_COUNT 0'
	printf '%s\n' 'RIMS_M9_RESET_COUNTS {"namespaceDocuments":0,"namespaceTransactions":0,"namespaceAttachments":0}'
elif [[ "${input}" == *"RIMS_M9_PENDING_ATTACHMENT_COUNT"* ]]; then
	printf '%s\n' 'RIMS_M9_PENDING_ATTACHMENT_COUNT 0'
fi
EOF
chmod +x "${manifest_guard_bin}/psql"
write_guard_env "${manifest_guard_dir}/test.env" "test" "127.0.0.1" "appdb"
PATH="${manifest_guard_bin}:${PATH}" \
	RIMS_ALLOW_DEV_SEED=1 \
	RIMS_ENV_FILE="${manifest_guard_dir}/test.env" \
	UPLOAD_DIR="${manifest_guard_uploads}" \
	bash "${SEED_SCRIPT}" --reset >/dev/null
[[ -f "${manifest_guard_uploads}/ordinary/keep.bin" ]] ||
	fail "reset trusted an injected manifest pointing to an ordinary upload"
[[ -f "${manifest_guard_uploads}/m9-e2e/ordinary.bin" ]] ||
	fail "reset trusted an injected manifest pointing beside fixture uploads"

printf '../outside.bin\n' > "${manifest_guard_uploads}/.rims-m9-reset-attachments"
printf 'outside upload\n' > "${manifest_guard_dir}/outside.bin"
if PATH="${manifest_guard_bin}:${PATH}" \
	RIMS_ALLOW_DEV_SEED=1 \
	RIMS_ENV_FILE="${manifest_guard_dir}/test.env" \
	UPLOAD_DIR="${manifest_guard_uploads}" \
	bash "${SEED_SCRIPT}" --reset >/dev/null 2>&1; then
	fail "reset accepted a path-traversal manifest"
fi
[[ -f "${manifest_guard_dir}/outside.bin" ]] ||
	fail "reset path traversal removed a file outside UPLOAD_DIR"

printf '\n' > "${manifest_guard_uploads}/.rims-m9-reset-attachments"
if PATH="${manifest_guard_bin}:${PATH}" \
	RIMS_ALLOW_DEV_SEED=1 \
	RIMS_ENV_FILE="${manifest_guard_dir}/test.env" \
	UPLOAD_DIR="${manifest_guard_uploads}" \
	bash "${SEED_SCRIPT}" --reset >/dev/null 2>&1; then
	fail "reset accepted a damaged empty manifest"
fi
[[ -f "${manifest_guard_uploads}/ordinary/keep.bin" ]] ||
	fail "damaged manifest removed an ordinary upload"

growth_guard_dir="${GUARD_TMP_DIR}/growth-guard"
growth_guard_bin="${growth_guard_dir}/bin"
mkdir -p "${growth_guard_bin}"
cat > "${growth_guard_bin}/psql" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
input="$(cat)"
if [[ " $* " == *" -v namespace_attachment_files="* ]]; then
	printf '%s\n' '{"namespaceAttachmentFiles":0}'
elif [[ "${input}" == *"INSERT INTO m9_reset_documents"* ]]; then
	printf '%s\n' 'RIMS_M9_RESET_COUNTS {"namespaceDocuments":0,"namespaceTransactions":0,"namespaceAttachments":0}'
elif [[ "${input}" == *"RIMS_M9_CLAIMED_PENDING_COUNT"* ]]; then
	printf '%s\n' 'RIMS_M9_CLAIMED_PENDING_COUNT 0'
	printf '%s\n' 'RIMS_M9_PENDING_ATTACHMENT_COUNT 0'
	printf 'RIMS_M9_TOMBSTONE_ATTACHMENT_COUNT %s\n' "${RIMS_TEST_GROWTH_TOMBSTONES:?}"
	printf 'RIMS_FILE_STORAGE_CLEANUP_PENDING_COUNT %s\n' "${RIMS_TEST_GROWTH_STORAGE_PENDING:?}"
	printf '%s\n' 'RIMS_M9_RESET_COUNTS {"namespaceDocuments":0,"namespaceTransactions":0,"namespaceAttachments":0}'
fi
EOF
chmod +x "${growth_guard_bin}/psql"
write_guard_env "${growth_guard_dir}/test.env" "test" "127.0.0.1" "appdb"
growth_tombstone_log="${growth_guard_dir}/tombstone.log"
if timeout --signal=TERM --kill-after=1s 10s env \
	PATH="${growth_guard_bin}:${PATH}" \
	RIMS_ALLOW_DEV_SEED=1 \
	RIMS_ENV_FILE="${growth_guard_dir}/test.env" \
	UPLOAD_DIR="${growth_guard_dir}/uploads" \
	RIMS_M9_MAX_TOMBSTONES=1 \
	RIMS_STORAGE_CLEANUP_MAX_PENDING=1 \
	RIMS_TEST_GROWTH_TOMBSTONES=2 \
	RIMS_TEST_GROWTH_STORAGE_PENDING=0 \
	bash "${SEED_SCRIPT}" --reset >"${growth_tombstone_log}" 2>&1; then
	fail "reset accepted tombstone growth above its configured limit"
fi
grep -Fq 'M9 cleanup tombstone count 2 exceeds configured limit 1' "${growth_tombstone_log}" ||
	fail "tombstone growth limit failure was not diagnosable"
growth_storage_log="${growth_guard_dir}/storage-pending.log"
if timeout --signal=TERM --kill-after=1s 10s env \
	PATH="${growth_guard_bin}:${PATH}" \
	RIMS_ALLOW_DEV_SEED=1 \
	RIMS_ENV_FILE="${growth_guard_dir}/test.env" \
	UPLOAD_DIR="${growth_guard_dir}/uploads" \
	RIMS_M9_MAX_TOMBSTONES=100 \
	RIMS_STORAGE_CLEANUP_MAX_PENDING=100 \
	RIMS_TEST_GROWTH_TOMBSTONES=0 \
	RIMS_TEST_GROWTH_STORAGE_PENDING=1 \
	bash "${SEED_SCRIPT}" --reset >"${growth_storage_log}" 2>&1; then
	fail "reset accepted non-zero storage cleanup responsibility below its configured capacity limit"
fi
grep -Fq 'file storage cleanup pending count 1 prevents a clean reset' "${growth_storage_log}" ||
	fail "non-zero storage cleanup clean-gate failure was not diagnosable"
if ! timeout --signal=TERM --kill-after=1s 10s env \
	PATH="${growth_guard_bin}:${PATH}" \
	RIMS_ALLOW_DEV_SEED=1 \
	RIMS_ENV_FILE="${growth_guard_dir}/test.env" \
	UPLOAD_DIR="${growth_guard_dir}/uploads" \
	RIMS_M9_MAX_TOMBSTONES=100 \
	RIMS_STORAGE_CLEANUP_MAX_PENDING=100 \
	RIMS_TEST_GROWTH_TOMBSTONES=0 \
	RIMS_TEST_GROWTH_STORAGE_PENDING=0 \
	bash "${SEED_SCRIPT}" --reset >/dev/null 2>&1; then
	fail "reset rejected zero pending storage cleanup responsibility"
fi

retry_dir="${GUARD_TMP_DIR}/physical-retry"
retry_bin="${retry_dir}/bin"
retry_uploads="${retry_dir}/uploads"
retry_state="${retry_dir}/state"
retry_object_key="m9-e2e/2026/07/m11-retry.bin"
retry_object_path="${retry_uploads}/${retry_object_key}"
mkdir -p "${retry_bin}" "$(dirname "${retry_object_path}")" "${retry_state}"
printf 'retry fixture attachment\n' > "${retry_object_path}"
cat > "${retry_bin}/psql" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
input="$(cat)"
state_dir=${RIMS_TEST_RETRY_STATE:?}
object_key=${RIMS_TEST_RETRY_OBJECT_KEY:?}
if [[ " $* " == *" -v namespace_attachment_files="* ]]; then
	printf '%s\n' '{"namespaceAttachmentFiles":0}'
elif [[ "${input}" == *"INSERT INTO m9_reset_documents"* ]]; then
	if [[ ! -e "${state_dir}/db-row-deleted" ]]; then
		touch "${state_dir}/db-row-deleted" "${state_dir}/pending"
		printf '%s' "${object_key}" | base64 | tr -d '\n' |
			xargs -r printf 'RIMS_M9_RESET_OBJECT_KEY %s 1\n'
	elif [[ "${input}" == *"rims_dev_fixture_attachment_cleanup"* &&
		-e "${state_dir}/pending" ]]; then
		printf '%s' "${object_key}" | base64 | tr -d '\n' |
			xargs -r printf 'RIMS_M9_RESET_OBJECT_KEY %s 2\n'
	fi
	printf '%s\n' 'RIMS_M9_RESET_COUNTS {"namespaceDocuments":0,"namespaceTransactions":0,"namespaceAttachments":0}'
elif [[ "${input}" == *"RIMS_M9_ACTIVE_ATTACHMENT_REFERENCE_COUNT"* ]]; then
	printf '%s\n' 'RIMS_M9_ACTIVE_ATTACHMENT_REFERENCE_COUNT 0'
elif [[ "${input}" == *"RIMS_M9_DELETE_ENTITLEMENT"* ]]; then
	printf '%s\n' 'RIMS_M9_DELETE_ENTITLEMENT 1'
elif [[ "${input}" == *"RIMS_M9_FINALIZED_TOMBSTONE"* ]]; then
	rm -f -- "${state_dir}/pending"
	touch "${state_dir}/tombstone"
	printf '%s\n' 'RIMS_M9_FINALIZED_TOMBSTONE 1'
elif [[ "${input}" == *"RIMS_M9_CLAIMED_PENDING_COUNT"* ]]; then
	printf '%s\n' 'RIMS_M9_CLAIMED_PENDING_COUNT 0'
	printf '%s\n' 'RIMS_M9_PENDING_ATTACHMENT_COUNT 0'
	printf '%s\n' 'RIMS_M9_TOMBSTONE_ATTACHMENT_COUNT 1'
	printf '%s\n' 'RIMS_FILE_STORAGE_CLEANUP_PENDING_COUNT 0'
	printf '%s\n' 'RIMS_M9_RESET_COUNTS {"namespaceDocuments":0,"namespaceTransactions":0,"namespaceAttachments":0}'
elif [[ "${input}" == *"RIMS_M9_PENDING_ATTACHMENT_COUNT"* ]]; then
	if [[ -e "${state_dir}/pending" ]]; then
		printf '%s\n' 'RIMS_M9_PENDING_ATTACHMENT_COUNT 1'
	else
		printf '%s\n' 'RIMS_M9_PENDING_ATTACHMENT_COUNT 0'
	fi
fi
EOF
cat > "${retry_bin}/rm" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
target=${!#}
if [[ "${target}" == "${RIMS_TEST_RETRY_FAIL_PATH:?}" &&
	! -e "${RIMS_TEST_RETRY_STATE:?}/rm-failed" ]]; then
	printf 'attempt\n' >> "${RIMS_TEST_RETRY_STATE}/rm-attempts"
	touch "${RIMS_TEST_RETRY_STATE}/rm-failed"
	exit 73
fi
if [[ "${target}" == "${RIMS_TEST_RETRY_FAIL_PATH}" ]]; then
	printf 'attempt\n' >> "${RIMS_TEST_RETRY_STATE}/rm-attempts"
fi
exec /bin/rm "$@"
EOF
chmod +x "${retry_bin}/psql" "${retry_bin}/rm"
write_guard_env "${retry_dir}/test.env" "test" "127.0.0.1" "appdb"
if PATH="${retry_bin}:${PATH}" \
	RIMS_ALLOW_DEV_SEED=1 \
	RIMS_ENV_FILE="${retry_dir}/test.env" \
	UPLOAD_DIR="${retry_uploads}" \
	RIMS_TEST_RETRY_STATE="${retry_state}" \
	RIMS_TEST_RETRY_OBJECT_KEY="${retry_object_key}" \
	RIMS_TEST_RETRY_FAIL_PATH="${retry_object_path}" \
	bash "${SEED_SCRIPT}" --reset >/dev/null 2>&1; then
	fail "reset ignored an injected physical attachment deletion failure"
fi
[[ -f "${retry_object_path}" ]] ||
	fail "failed attachment deletion unexpectedly removed the retry fixture"
[[ -e "${retry_state}/db-row-deleted" ]] ||
	fail "physical deletion failure occurred before the database cleanup committed"
[[ -e "${retry_state}/pending" ]] ||
	fail "database cleanup did not persist physical attachment responsibility"
PATH="${retry_bin}:${PATH}" \
	RIMS_ALLOW_DEV_SEED=1 \
	RIMS_ENV_FILE="${retry_dir}/test.env" \
	UPLOAD_DIR="${retry_uploads}" \
	RIMS_TEST_RETRY_STATE="${retry_state}" \
	RIMS_TEST_RETRY_OBJECT_KEY="${retry_object_key}" \
	RIMS_TEST_RETRY_FAIL_PATH="${retry_object_path}" \
	bash "${SEED_SCRIPT}" --reset >/dev/null
[[ ! -e "${retry_object_path}" ]] ||
	fail "second reset lost the pending physical attachment cleanup"
[[ ! -e "${retry_state}/pending" ]] ||
	fail "second reset did not discharge persisted attachment responsibility"
assert_eq "$(wc -l < "${retry_state}/rm-attempts" | tr -d '[:space:]')" "2" "physical attachment retry attempts"

shared_key_dir="${GUARD_TMP_DIR}/shared-key"
shared_key_bin="${shared_key_dir}/bin"
shared_key_uploads="${shared_key_dir}/uploads"
shared_key_state="${shared_key_dir}/state"
shared_object_key="m9-e2e/2026/07/shared-key.bin"
shared_object_path="${shared_key_uploads}/${shared_object_key}"
mkdir -p "${shared_key_bin}" "$(dirname "${shared_object_path}")" "${shared_key_state}"
touch "${shared_key_state}/pending" "${shared_key_state}/ordinary-reference"
printf 'shared attachment bytes\n' > "${shared_object_path}"
cat > "${shared_key_bin}/psql" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
input="$(cat)"
state_dir=${RIMS_TEST_SHARED_STATE:?}
object_key=${RIMS_TEST_SHARED_OBJECT_KEY:?}
if [[ " $* " == *" -v namespace_attachment_files="* ]]; then
	printf '%s\n' '{"namespaceAttachmentFiles":0}'
elif [[ "${input}" == *"INSERT INTO m9_reset_documents"* ]]; then
	printf '%s' "${object_key}" | base64 | tr -d '\n' |
		xargs -r printf 'RIMS_M9_RESET_OBJECT_KEY %s 1\n'
	printf '%s\n' 'RIMS_M9_RESET_COUNTS {"namespaceDocuments":0,"namespaceTransactions":0,"namespaceAttachments":0}'
elif [[ "${input}" == *"RIMS_M9_ACTIVE_ATTACHMENT_REFERENCE_COUNT"* ]]; then
	printf '%s\n' 'RIMS_M9_ACTIVE_ATTACHMENT_REFERENCE_COUNT 1'
elif [[ "${input}" == *"RIMS_M9_DELETE_ENTITLEMENT"* ]]; then
	printf '%s\n' 'RIMS_M9_DELETE_ENTITLEMENT 1'
elif [[ "${input}" == *"RIMS_M9_PENDING_ATTACHMENT_COUNT"* ]]; then
	if [[ -e "${state_dir}/pending" ]]; then
		printf '%s\n' 'RIMS_M9_PENDING_ATTACHMENT_COUNT 1'
	else
		printf '%s\n' 'RIMS_M9_PENDING_ATTACHMENT_COUNT 0'
	fi
fi
EOF
chmod +x "${shared_key_bin}/psql"
write_guard_env "${shared_key_dir}/test.env" "test" "127.0.0.1" "appdb"
if PATH="${shared_key_bin}:${PATH}" \
	RIMS_ALLOW_DEV_SEED=1 \
	RIMS_ENV_FILE="${shared_key_dir}/test.env" \
	UPLOAD_DIR="${shared_key_uploads}" \
	RIMS_TEST_SHARED_STATE="${shared_key_state}" \
	RIMS_TEST_SHARED_OBJECT_KEY="${shared_object_key}" \
	bash "${SEED_SCRIPT}" --reset >/dev/null 2>&1; then
	fail "reset deleted a pending object still referenced by an ordinary attachment"
fi
[[ -f "${shared_object_path}" ]] ||
	fail "reset removed physical bytes shared with an ordinary attachment"
[[ -e "${shared_key_state}/ordinary-reference" ]] ||
	fail "reset removed the simulated ordinary attachment reference"
[[ -e "${shared_key_state}/pending" ]] ||
	fail "reset dropped pending responsibility for a still-referenced object"

concurrent_dir="${GUARD_TMP_DIR}/concurrent-pending"
concurrent_bin="${concurrent_dir}/bin"
concurrent_uploads="${concurrent_dir}/uploads"
concurrent_state="${concurrent_dir}/state"
concurrent_object_key="m9-e2e/2026/07/claimed-a.bin"
concurrent_object_path="${concurrent_uploads}/${concurrent_object_key}"
mkdir -p "${concurrent_bin}" "$(dirname "${concurrent_object_path}")" "${concurrent_state}"
touch "${concurrent_state}/pending-a"
printf 'claimed attachment bytes\n' > "${concurrent_object_path}"
cat > "${concurrent_bin}/psql" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
input="$(cat)"
state_dir=${RIMS_TEST_CONCURRENT_STATE:?}
object_key=${RIMS_TEST_CONCURRENT_OBJECT_KEY:?}
pending_count() {
	find "${state_dir}" -maxdepth 1 -type f -name 'pending-*' | wc -l | tr -d '[:space:]'
}
if [[ " $* " == *" -v namespace_attachment_files="* ]]; then
	printf '%s\n' '{"namespaceAttachmentFiles":0}'
elif [[ "${input}" == *"INSERT INTO m9_reset_documents"* ]]; then
	printf '%s' "${object_key}" | base64 | tr -d '\n' |
		xargs -r printf 'RIMS_M9_RESET_OBJECT_KEY %s 1\n'
	printf '%s\n' 'RIMS_M9_RESET_COUNTS {"namespaceDocuments":0,"namespaceTransactions":0,"namespaceAttachments":0}'
elif [[ "${input}" == *"RIMS_M9_ACTIVE_ATTACHMENT_REFERENCE_COUNT"* ]]; then
	printf '%s\n' 'RIMS_M9_ACTIVE_ATTACHMENT_REFERENCE_COUNT 0'
elif [[ "${input}" == *"RIMS_M9_DELETE_ENTITLEMENT"* ]]; then
	printf '%s\n' 'RIMS_M9_DELETE_ENTITLEMENT 1'
elif [[ "${input}" == *"RIMS_M9_FINALIZED_TOMBSTONE"* ]]; then
	touch "${state_dir}/pending-b" "${state_dir}/tombstone-a"
	rm -f -- "${state_dir}/pending-a"
	printf '%s\n' 'RIMS_M9_FINALIZED_TOMBSTONE 1'
elif [[ "${input}" == *"RIMS_M9_CLAIMED_PENDING_COUNT"* ]]; then
	printf '%s\n' 'RIMS_M9_CLAIMED_PENDING_COUNT 0'
	printf 'RIMS_M9_PENDING_ATTACHMENT_COUNT %s\n' "$(pending_count)"
	printf '%s\n' 'RIMS_M9_TOMBSTONE_ATTACHMENT_COUNT 1'
	printf '%s\n' 'RIMS_FILE_STORAGE_CLEANUP_PENDING_COUNT 0'
	printf '%s\n' 'RIMS_M9_RESET_COUNTS {"namespaceDocuments":0,"namespaceTransactions":0,"namespaceAttachments":0}'
elif [[ "${input}" == *"RIMS_M9_PENDING_ATTACHMENT_COUNT"* ]]; then
	printf 'RIMS_M9_PENDING_ATTACHMENT_COUNT %s\n' "$(pending_count)"
fi
EOF
chmod +x "${concurrent_bin}/psql"
write_guard_env "${concurrent_dir}/test.env" "test" "127.0.0.1" "appdb"
if PATH="${concurrent_bin}:${PATH}" \
	RIMS_ALLOW_DEV_SEED=1 \
	RIMS_ENV_FILE="${concurrent_dir}/test.env" \
	UPLOAD_DIR="${concurrent_uploads}" \
	RIMS_TEST_CONCURRENT_STATE="${concurrent_state}" \
	RIMS_TEST_CONCURRENT_OBJECT_KEY="${concurrent_object_key}" \
	bash "${SEED_SCRIPT}" --reset >/dev/null 2>&1; then
	fail "reset reported success after a concurrent pending responsibility appeared"
fi
[[ -e "${concurrent_state}/pending-b" ]] ||
	fail "reset cleared another instance's concurrent pending responsibility"
[[ ! -e "${concurrent_state}/concurrent-row-cleared" ]] ||
	fail "reset used an unconditional pending-table delete"

lost_owner_dir="${GUARD_TMP_DIR}/lost-owner"
lost_owner_bin="${lost_owner_dir}/bin"
lost_owner_uploads="${lost_owner_dir}/uploads"
lost_owner_state="${lost_owner_dir}/state"
lost_owner_key="2026/07/reused-legacy-key.bin"
lost_owner_path="${lost_owner_uploads}/${lost_owner_key}"
mkdir -p "${lost_owner_bin}" "$(dirname "${lost_owner_path}")" "${lost_owner_state}"
printf 'worker A fixture bytes\n' > "${lost_owner_path}"
cat > "${lost_owner_bin}/psql" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
input="$(cat)"
state_dir=${RIMS_TEST_LOST_OWNER_STATE:?}
object_key=${RIMS_TEST_LOST_OWNER_KEY:?}
object_path=${RIMS_TEST_LOST_OWNER_PATH:?}
if [[ " $* " == *" -v namespace_attachment_files="* ]]; then
	printf '%s\n' '{"namespaceAttachmentFiles":0}'
elif [[ " $* " == *" -v cleanup_statement_timeout="* &&
	"${input}" == *"UPDATE rims_dev_fixture_attachment_cleanup"* ]]; then
	touch "${state_dir}/stale-release-attempted"
elif [[ "${input}" == *"RIMS_M9_FINALIZED_TOMBSTONE"* ]]; then
	touch "${state_dir}/stale-finalize-called"
	printf '%s\n' 'RIMS_M9_FINALIZED_TOMBSTONE 0'
elif [[ "${input}" == *"INSERT INTO m9_reset_documents"* ]]; then
	printf '%s' "${object_key}" | base64 | tr -d '\n' |
		xargs -r printf 'RIMS_M9_RESET_OBJECT_KEY %s 1\n'
	printf '%s\n' 'RIMS_M9_RESET_COUNTS {"namespaceDocuments":0,"namespaceTransactions":0,"namespaceAttachments":0}'
elif [[ "${input}" == *"RIMS_M9_ACTIVE_ATTACHMENT_REFERENCE_COUNT"* ]]; then
	touch "${state_dir}/worker-b-finalized" "${state_dir}/worker-b-owner-intact"
	printf 'ordinary attachment after worker B finalize\n' > "${object_path}"
	printf '%s\n' 'RIMS_M9_ACTIVE_ATTACHMENT_REFERENCE_COUNT 0'
elif [[ "${input}" == *"RIMS_M9_DELETE_ENTITLEMENT"* ]]; then
	printf '%s\n' 'RIMS_M9_DELETE_ENTITLEMENT 0'
elif [[ "${input}" == *"RIMS_M9_CLAIMED_PENDING_COUNT"* ]]; then
	printf '%s\n' 'RIMS_M9_CLAIMED_PENDING_COUNT 0'
	printf '%s\n' 'RIMS_M9_PENDING_ATTACHMENT_COUNT 0'
	printf '%s\n' 'RIMS_M9_TOMBSTONE_ATTACHMENT_COUNT 0'
	printf '%s\n' 'RIMS_FILE_STORAGE_CLEANUP_PENDING_COUNT 0'
	printf '%s\n' 'RIMS_M9_RESET_COUNTS {"namespaceDocuments":0,"namespaceTransactions":0,"namespaceAttachments":0}'
fi
EOF
cat > "${lost_owner_bin}/rm" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${!#}" == "${RIMS_TEST_LOST_OWNER_PATH:?}" ]]; then
	touch "${RIMS_TEST_LOST_OWNER_STATE:?}/stale-worker-rm-called"
fi
exec /bin/rm "$@"
EOF
chmod +x "${lost_owner_bin}/psql" "${lost_owner_bin}/rm"
write_guard_env "${lost_owner_dir}/test.env" "test" "127.0.0.1" "appdb"
if timeout --signal=TERM --kill-after=1s 10s env \
	PATH="${lost_owner_bin}:${PATH}" \
	RIMS_ALLOW_DEV_SEED=1 \
	RIMS_ENV_FILE="${lost_owner_dir}/test.env" \
	UPLOAD_DIR="${lost_owner_uploads}" \
	RIMS_TEST_LOST_OWNER_STATE="${lost_owner_state}" \
	RIMS_TEST_LOST_OWNER_KEY="${lost_owner_key}" \
	RIMS_TEST_LOST_OWNER_PATH="${lost_owner_path}" \
	bash "${SEED_SCRIPT}" --reset >/dev/null 2>&1; then
	fail "stale worker reported success after losing physical-delete ownership"
fi
[[ -e "${lost_owner_state}/worker-b-finalized" ]] ||
	fail "lost-owner fixture did not reach worker B finalize interleave"
[[ ! -e "${lost_owner_state}/stale-worker-rm-called" ]] ||
	fail "stale worker called rm after worker B changed the claim version"
[[ -e "${lost_owner_state}/stale-release-attempted" ]] ||
	fail "lost-owner fixture did not exercise stale release fencing"
[[ ! -e "${lost_owner_state}/stale-finalize-called" ]] ||
	fail "stale worker reached finalize after losing delete entitlement"
[[ -e "${lost_owner_state}/worker-b-owner-intact" ]] ||
	fail "stale worker release changed worker B ownership"
[[ -f "${lost_owner_path}" ]] ||
	fail "stale worker deleted an ordinary attachment that reused a legacy key"
grep -Fq 'ordinary attachment after worker B finalize' "${lost_owner_path}" ||
	fail "stale worker changed the replacement ordinary attachment bytes"

cleanup_timeout_dir="${GUARD_TMP_DIR}/cleanup-timeout"
cleanup_timeout_bin="${cleanup_timeout_dir}/bin"
cleanup_timeout_uploads="${cleanup_timeout_dir}/uploads"
cleanup_timeout_state="${cleanup_timeout_dir}/state"
cleanup_timeout_key="m9-e2e/2026/07/cleanup-timeout.bin"
cleanup_timeout_path="${cleanup_timeout_uploads}/${cleanup_timeout_key}"
cleanup_timeout_log="${cleanup_timeout_dir}/reset.log"
mkdir -p "${cleanup_timeout_bin}" "$(dirname "${cleanup_timeout_path}")" "${cleanup_timeout_state}"
printf 'cleanup timeout attachment\n' > "${cleanup_timeout_path}"
cat > "${cleanup_timeout_bin}/psql" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
input="$(cat)"
state_dir=${RIMS_TEST_CLEANUP_TIMEOUT_STATE:?}
object_key=${RIMS_TEST_CLEANUP_TIMEOUT_KEY:?}
if [[ " $* " == *" -v namespace_attachment_files="* ]]; then
	printf '%s\n' '{"namespaceAttachmentFiles":0}'
elif [[ " $* " == *" -v cleanup_statement_timeout="* &&
	"${input}" == *"UPDATE rims_dev_fixture_attachment_cleanup"* ]]; then
	touch "${state_dir}/release-attempted"
	printf '%s\n' 'ERROR: canceling statement due to lock timeout' >&2
	exit 55
elif [[ "${input}" == *"INSERT INTO m9_reset_documents"* ]]; then
	printf '%s' "${object_key}" | base64 | tr -d '\n' |
		xargs -r printf 'RIMS_M9_RESET_OBJECT_KEY %s 7\n'
	printf '%s\n' 'RIMS_M9_RESET_COUNTS {"namespaceDocuments":0,"namespaceTransactions":0,"namespaceAttachments":0}'
elif [[ "${input}" == *"RIMS_M9_ACTIVE_ATTACHMENT_REFERENCE_COUNT"* ]]; then
	printf '%s\n' 'RIMS_M9_ACTIVE_ATTACHMENT_REFERENCE_COUNT 0'
elif [[ "${input}" == *"RIMS_M9_DELETE_ENTITLEMENT"* ]]; then
	printf '%s\n' 'RIMS_M9_DELETE_ENTITLEMENT 1'
fi
EOF
cat > "${cleanup_timeout_bin}/rm" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${!#}" == "${RIMS_TEST_CLEANUP_TIMEOUT_PATH:?}" ]]; then
	printf '%s\n' 'injected primary rm failure' >&2
	exit 73
fi
exec /bin/rm "$@"
EOF
chmod +x "${cleanup_timeout_bin}/psql" "${cleanup_timeout_bin}/rm"
write_guard_env "${cleanup_timeout_dir}/test.env" "test" "127.0.0.1" "appdb"
set +e
PATH="${cleanup_timeout_bin}:${PATH}" \
	RIMS_ALLOW_DEV_SEED=1 \
	RIMS_ENV_FILE="${cleanup_timeout_dir}/test.env" \
	RIMS_M9_CLEANUP_TIMEOUT_MS=100 \
	UPLOAD_DIR="${cleanup_timeout_uploads}" \
	RIMS_TEST_CLEANUP_TIMEOUT_STATE="${cleanup_timeout_state}" \
	RIMS_TEST_CLEANUP_TIMEOUT_KEY="${cleanup_timeout_key}" \
	RIMS_TEST_CLEANUP_TIMEOUT_PATH="${cleanup_timeout_path}" \
	bash "${SEED_SCRIPT}" --reset >"${cleanup_timeout_log}" 2>&1
cleanup_timeout_exit=$?
set -e
[[ "${cleanup_timeout_exit}" -ne 0 ]] || fail "reset hid its primary failure and cleanup lock timeout"
grep -Fq "failed to remove M9 attachment ${cleanup_timeout_key}" "${cleanup_timeout_log}" ||
	fail "reset log lost the first business failure"
grep -Fq 'M9 cleanup release failed' "${cleanup_timeout_log}" ||
	fail "reset log omitted the cleanup failure"
grep -Fqi 'lock timeout' "${cleanup_timeout_log}" ||
	fail "reset cleanup failure was not diagnosable as a lock timeout"
[[ -e "${cleanup_timeout_state}/release-attempted" ]] ||
	fail "reset did not attempt bounded claim release after its business failure"
[[ -f "${cleanup_timeout_path}" ]] ||
	fail "cleanup timeout test unexpectedly deleted the claimed attachment"

namespace_race_dir="${GUARD_TMP_DIR}/namespace-race"
namespace_race_bin="${namespace_race_dir}/bin"
namespace_race_uploads="${namespace_race_dir}/uploads"
namespace_race_state="${namespace_race_dir}/state"
mkdir -p "${namespace_race_bin}" "${namespace_race_uploads}" "${namespace_race_state}"
cat > "${namespace_race_bin}/psql" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
input="$(cat)"
state_dir=${RIMS_TEST_NAMESPACE_RACE_STATE:?}
if [[ " $* " == *" -v namespace_attachment_files="* ]]; then
	printf '%s\n' '{"namespaceAttachmentFiles":0}'
elif [[ "${input}" == *"INSERT INTO m9_reset_documents"* ]]; then
	printf '%s\n' 'RIMS_M9_RESET_COUNTS {"namespaceDocuments":0,"namespaceTransactions":0,"namespaceAttachments":0}'
elif [[ "${input}" == *"RIMS_M9_CLAIMED_PENDING_COUNT"* ]]; then
	touch "${state_dir}/namespace-inserted-after-snapshot"
	printf '%s\n' 'RIMS_M9_CLAIMED_PENDING_COUNT 0'
	printf '%s\n' 'RIMS_M9_PENDING_ATTACHMENT_COUNT 0'
	printf '%s\n' 'RIMS_M9_TOMBSTONE_ATTACHMENT_COUNT 0'
	printf '%s\n' 'RIMS_FILE_STORAGE_CLEANUP_PENDING_COUNT 0'
	printf '%s\n' 'RIMS_M9_RESET_COUNTS {"namespaceDocuments":1,"namespaceTransactions":0,"namespaceAttachments":0}'
fi
EOF
chmod +x "${namespace_race_bin}/psql"
write_guard_env "${namespace_race_dir}/test.env" "test" "127.0.0.1" "appdb"
if PATH="${namespace_race_bin}:${PATH}" \
	RIMS_ALLOW_DEV_SEED=1 \
	RIMS_ENV_FILE="${namespace_race_dir}/test.env" \
	UPLOAD_DIR="${namespace_race_uploads}" \
	RIMS_TEST_NAMESPACE_RACE_STATE="${namespace_race_state}" \
	bash "${SEED_SCRIPT}" --reset >/dev/null 2>&1; then
	fail "reset trusted a stale namespace snapshot after a concurrent fixture insert"
fi
[[ -e "${namespace_race_state}/namespace-inserted-after-snapshot" ]] ||
	fail "namespace race fixture did not reach the post-snapshot interleave"

failure_safe_dir="${GUARD_TMP_DIR}/failure-safe"
failure_safe_bin="${failure_safe_dir}/bin"
failure_safe_uploads="${failure_safe_dir}/uploads"
mkdir -p "${failure_safe_bin}" "${failure_safe_uploads}/m9" "${failure_safe_uploads}/other"
printf 'referenced fixture\n' > "${failure_safe_uploads}/m9/referenced.bin"
printf 'non fixture\n' > "${failure_safe_uploads}/other/keep.bin"
cat > "${failure_safe_bin}/psql" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
for argument in "$@"; do
	if [[ "${argument}" == *"SELECT fa.object_key"* ]]; then
		printf '%s\n' 'm9/referenced.bin'
		exit 0
	fi
done
cat >/dev/null
exit 47
EOF
chmod +x "${failure_safe_bin}/psql"
write_guard_env "${failure_safe_dir}/test.env" "test" "127.0.0.1" "appdb"
if PATH="${failure_safe_bin}:${PATH}" \
	RIMS_ALLOW_DEV_SEED=1 \
	RIMS_ENV_FILE="${failure_safe_dir}/test.env" \
	UPLOAD_DIR="${failure_safe_uploads}" \
	bash "${SEED_SCRIPT}" --reset >/dev/null 2>&1; then
	fail "reset accepted an injected database cleanup failure"
fi
[[ -f "${failure_safe_uploads}/m9/referenced.bin" ]] ||
	fail "reset deleted a referenced fixture attachment before the database transaction committed"
[[ -f "${failure_safe_uploads}/other/keep.bin" ]] ||
	fail "reset deleted a non-fixture attachment after a database failure"

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

legacy_migration_sql="${GUARD_TMP_DIR}/legacy-cleanup-migration.sql"
{
	cat <<'SQL'
BEGIN;
DROP TRIGGER IF EXISTS trg_guard_fixture_attachment_object_key ON file_attachments;
DROP TABLE IF EXISTS rims_dev_fixture_attachment_cleanup CASCADE;
CREATE TABLE rims_dev_fixture_attachment_cleanup (
  object_key VARCHAR(512) PRIMARY KEY,
  source_document_id BIGINT NOT NULL,
  source_doc_no VARCHAR(32) NOT NULL,
  source_remark TEXT NOT NULL,
  queued_at TIMESTAMPTZ
);
INSERT INTO rims_dev_fixture_attachment_cleanup (
  object_key, source_document_id, source_doc_no, source_remark, queued_at
) VALUES ('2026/07/legacy-upgrade.bin', 1, 'M9DOC-LEGACY', 'M9-E2E: legacy migration', NULL);
SQL
	cat "${CLEANUP_GUARD_MIGRATION}"
	cat "${CLEANUP_GUARD_MIGRATION}"
	cat <<'SQL'
DO $$
DECLARE
  claim_version_nullable TEXT;
  claim_version_default TEXT;
  completed_at_nullable TEXT;
  queued_at_nullable TEXT;
  lease_index_definition TEXT;
  lease_version BIGINT;
BEGIN
  SELECT is_nullable, column_default
  INTO claim_version_nullable, claim_version_default
  FROM information_schema.columns
  WHERE table_schema = current_schema()
    AND table_name = 'rims_dev_fixture_attachment_cleanup'
    AND column_name = 'claim_version';
  IF claim_version_nullable IS DISTINCT FROM 'NO'
     OR COALESCE(claim_version_default, '') NOT LIKE '%0%' THEN
    RAISE EXCEPTION 'claim_version upgrade is incomplete: nullable %, default %',
      claim_version_nullable, claim_version_default;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = current_schema()
      AND table_name = 'rims_dev_fixture_attachment_cleanup'
      AND column_name IN ('claim_token', 'claimed_at', 'completed_at')
    GROUP BY table_name HAVING count(*) = 3
  ) THEN
    RAISE EXCEPTION 'claim lease or completion columns are missing after legacy migration';
  END IF;
  SELECT is_nullable INTO completed_at_nullable
  FROM information_schema.columns
  WHERE table_schema = current_schema()
    AND table_name = 'rims_dev_fixture_attachment_cleanup'
    AND column_name = 'completed_at';
  IF completed_at_nullable IS DISTINCT FROM 'YES' THEN
    RAISE EXCEPTION 'completed_at must remain nullable for pending rows';
  END IF;
  SELECT is_nullable INTO queued_at_nullable
  FROM information_schema.columns
  WHERE table_schema = current_schema()
    AND table_name = 'rims_dev_fixture_attachment_cleanup'
    AND column_name = 'queued_at';
  IF queued_at_nullable IS DISTINCT FROM 'NO' THEN
    RAISE EXCEPTION 'queued_at was not backfilled and constrained';
  END IF;
  SELECT indexdef INTO lease_index_definition
  FROM pg_indexes
    WHERE schemaname = current_schema()
      AND tablename = 'rims_dev_fixture_attachment_cleanup'
      AND indexname = 'idx_rims_dev_fixture_cleanup_claim_lease';
  IF lease_index_definition IS NULL
     OR lease_index_definition NOT LIKE '%completed_at IS NULL%'
     OR lease_index_definition NOT LIKE '%claim_token IS NOT NULL%' THEN
    RAISE EXCEPTION 'claim lease index is missing its pending-row predicate: %',
      lease_index_definition;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'rims_dev_fixture_attachment_cleanup'::regclass
      AND conname = 'rims_dev_fixture_attachment_cleanup_source_check'
  ) THEN
    RAISE EXCEPTION 'fixture source constraint is missing after upgrade';
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'rims_dev_fixture_attachment_cleanup'::regclass
      AND conname = 'rims_dev_fixture_attachment_cleanup_completed_claim_check'
  ) THEN
    RAISE EXCEPTION 'completed tombstone claim constraint is missing after upgrade';
  END IF;
  IF (
    SELECT count(*)
    FROM pg_trigger
    WHERE NOT tgisinternal
      AND tgname IN (
        'trg_guard_fixture_attachment_object_key',
        'trg_guard_fixture_cleanup_object_key'
      )
  ) <> 2 THEN
    RAISE EXCEPTION 'fixture cleanup guard triggers are missing after upgrade';
  END IF;
  UPDATE rims_dev_fixture_attachment_cleanup
  SET claim_token = 'migration-owner',
      claim_version = claim_version + 1,
      claimed_at = CURRENT_TIMESTAMP
  WHERE object_key = '2026/07/legacy-upgrade.bin'
  RETURNING claim_version INTO lease_version;
  IF lease_version <> 1 THEN
    RAISE EXCEPTION 'legacy row could not enter lease protocol: %', lease_version;
  END IF;
END;
$$;
ROLLBACK;
SQL
} > "${legacy_migration_sql}"
timeout --signal=TERM --kill-after=2s 30s "${PSQL[@]}" -f - < "${legacy_migration_sql}" >/dev/null

"${PSQL[@]}" -f - < "${CLEANUP_GUARD_MIGRATION}" >/dev/null
"${PSQL[@]}" -f - < "${STORAGE_CLEANUP_MIGRATION}" >/dev/null
"${PSQL[@]}" -f - < "${STORAGE_CLEANUP_MIGRATION}" >/dev/null
assert_eq "$(sql "SELECT count(*) FROM information_schema.columns
WHERE table_schema = current_schema()
  AND table_name = 'file_storage_cleanup_queue'
  AND column_name IN (
    'object_key', 'source_operation', 'primary_error', 'cleanup_error',
    'attempt_count', 'ready_at', 'queued_at', 'updated_at'
  )")" "8" "file storage cleanup migration columns"

post_entitlement_suffix="$$-${RANDOM}"
post_entitlement_key="2026/07/post-entitlement-${post_entitlement_suffix}.bin"
post_entitlement_upload_dir="${UPLOAD_DIR:-./uploads}"
if [[ "${post_entitlement_upload_dir}" != /* ]]; then
	post_entitlement_upload_dir="${REPO_ROOT}/${post_entitlement_upload_dir#./}"
fi
post_entitlement_upload_dir="$(realpath -m -- "${post_entitlement_upload_dir}")"
post_entitlement_path="${post_entitlement_upload_dir}/${post_entitlement_key}"
post_entitlement_dir="${GUARD_TMP_DIR}/post-entitlement"
post_entitlement_bin="${post_entitlement_dir}/bin"
post_entitlement_state="${post_entitlement_dir}/state"
post_entitlement_a_log="${post_entitlement_dir}/worker-a.log"
post_entitlement_b_log="${post_entitlement_dir}/worker-b.log"
mkdir -p "${post_entitlement_bin}" "${post_entitlement_state}" "$(dirname "${post_entitlement_path}")"
printf 'worker A fixture before pause\n' > "${post_entitlement_path}"
cat > "${post_entitlement_bin}/rm" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
target=${!#}
if [[ "${target}" == "${RIMS_TEST_POST_ENTITLEMENT_PATH:?}" ]]; then
	touch "${RIMS_TEST_POST_ENTITLEMENT_STATE:?}/worker-a-paused"
	for _ in $(seq 1 400); do
		[[ ! -e "${RIMS_TEST_POST_ENTITLEMENT_STATE}/worker-a-proceed" ]] || break
		sleep 0.05
	done
	[[ -e "${RIMS_TEST_POST_ENTITLEMENT_STATE}/worker-a-proceed" ]] || {
		printf '%s\n' 'post-entitlement pause exceeded hard deadline' >&2
		exit 124
	}
fi
exec /bin/rm "$@"
EOF
chmod +x "${post_entitlement_bin}/rm"
sql "DELETE FROM file_attachments WHERE object_key = '${post_entitlement_key}';
DELETE FROM rims_dev_fixture_attachment_cleanup WHERE object_key = '${post_entitlement_key}';
DELETE FROM documents WHERE doc_no = 'POST-ENTITLEMENT-${post_entitlement_suffix}';
INSERT INTO rims_dev_fixture_attachment_cleanup (
  object_key, source_document_id, source_doc_no, source_remark
) VALUES (
  '${post_entitlement_key}', 1, 'M9DOC-POST-ENTITLEMENT', 'M9-E2E: post entitlement pause'
);"
POST_ENTITLEMENT_PROCEED="${post_entitlement_state}/worker-a-proceed"
timeout --signal=TERM --kill-after=2s 45s env \
	PATH="${post_entitlement_bin}:${PATH}" \
	RIMS_ALLOW_DEV_SEED=1 \
	RIMS_ENV_FILE="${ENV_FILE}" \
	RIMS_M9_CLAIM_LEASE_MS=1000 \
	UPLOAD_DIR="${post_entitlement_upload_dir}" \
	RIMS_TEST_POST_ENTITLEMENT_PATH="${post_entitlement_path}" \
	RIMS_TEST_POST_ENTITLEMENT_STATE="${post_entitlement_state}" \
	bash "${SEED_SCRIPT}" --reset >"${post_entitlement_a_log}" 2>&1 &
POST_ENTITLEMENT_WORKER_PID=$!
post_entitlement_paused=false
for _ in $(seq 1 200); do
	if [[ -e "${post_entitlement_state}/worker-a-paused" ]]; then
		post_entitlement_paused=true
		break
	fi
	if ! kill -0 "${POST_ENTITLEMENT_WORKER_PID}" >/dev/null 2>&1; then
		break
	fi
	sleep 0.05
done
if [[ "${post_entitlement_paused}" != true ]]; then
	touch "${POST_ENTITLEMENT_PROCEED}"
	wait "${POST_ENTITLEMENT_WORKER_PID}" >/dev/null 2>&1 || true
	POST_ENTITLEMENT_WORKER_PID=''
	fail "worker A did not reach the post-entitlement pause: $(tr '\n' ' ' < "${post_entitlement_a_log}")"
fi
sleep 1.2
set +e
timeout --signal=TERM --kill-after=2s 60s env \
	RIMS_ALLOW_DEV_SEED=1 \
	RIMS_ENV_FILE="${ENV_FILE}" \
	RIMS_M9_CLAIM_LEASE_MS=1000 \
	UPLOAD_DIR="${post_entitlement_upload_dir}" \
	bash "${SEED_SCRIPT}" --reset >"${post_entitlement_b_log}" 2>&1
post_entitlement_b_exit=$?
set -e
sql "INSERT INTO documents (
  doc_no, doc_type, status, warehouse_id, remark, created_by, updated_by
)
SELECT 'POST-ENTITLEMENT-${post_entitlement_suffix}', 2, 1, w.id,
       'ordinary post-entitlement key reuse', u.id, u.id
FROM warehouses w, users u
WHERE w.code = 'WH001' AND w.deleted_at IS NULL
  AND u.username = 'admin' AND u.deleted_at IS NULL;"
printf 'ordinary attachment after worker B completion\n' > "${post_entitlement_path}"
set +e
post_entitlement_insert_log="$(sql "INSERT INTO file_attachments (
  business_type, business_id, object_key, file_url, original_name,
  file_size, file_hash, mime_type, created_by, updated_by
)
SELECT 'doc_attachment', d.id, '${post_entitlement_key}',
       '/uploads/${post_entitlement_key}', 'ordinary.bin', 44, repeat('c', 64),
       'application/octet-stream', d.created_by, d.updated_by
FROM documents d WHERE d.doc_no = 'POST-ENTITLEMENT-${post_entitlement_suffix}';" 2>&1)"
post_entitlement_insert_exit=$?
set -e
if [[ "${post_entitlement_insert_exit}" -ne 0 ]]; then
	/bin/rm -f -- "${post_entitlement_path}"
fi
touch "${POST_ENTITLEMENT_PROCEED}"
set +e
wait "${POST_ENTITLEMENT_WORKER_PID}"
post_entitlement_a_exit=$?
set -e
POST_ENTITLEMENT_WORKER_PID=''
post_entitlement_attachment_count="$(sql "SELECT count(*) FROM file_attachments WHERE object_key = '${post_entitlement_key}'")"
post_entitlement_tombstone_state="$(sql "SELECT concat_ws('|', claim_version, completed_at IS NOT NULL,
  claim_token IS NULL, claimed_at IS NULL)
FROM rims_dev_fixture_attachment_cleanup WHERE object_key = '${post_entitlement_key}'")"
post_entitlement_file_exists=false
[[ ! -e "${post_entitlement_path}" ]] || post_entitlement_file_exists=true
sql "DELETE FROM file_attachments WHERE object_key = '${post_entitlement_key}';
DELETE FROM documents WHERE doc_no = 'POST-ENTITLEMENT-${post_entitlement_suffix}';
DELETE FROM rims_dev_fixture_attachment_cleanup WHERE object_key = '${post_entitlement_key}';"
/bin/rm -f -- "${post_entitlement_path}"
[[ "${post_entitlement_b_exit}" -eq 0 ]] ||
	fail "worker B could not reclaim and complete cleanup: $(tr '\n' ' ' < "${post_entitlement_b_log}")"
[[ "${post_entitlement_a_exit}" -ne 0 ]] ||
	fail "stale worker A reported success after worker B completed its tombstone"
grep -Fq 'lost M9 attachment finalize entitlement' "${post_entitlement_a_log}" ||
	fail "stale worker A did not fail through exact token/version finalize fencing"
[[ "${post_entitlement_insert_exit}" -ne 0 ]] ||
	fail "ordinary attachment reused a legacy key after worker B removed cleanup ownership; worker A exit=${post_entitlement_a_exit}, attachment=${post_entitlement_attachment_count}, file=${post_entitlement_file_exists}"
grep -Fq 'fixture cleanup' <<< "${post_entitlement_insert_log}" ||
	fail "post-entitlement tombstone rejection was not diagnosable"
assert_eq "${post_entitlement_attachment_count}" "0" "post-entitlement ordinary attachment rejection"
assert_eq "${post_entitlement_tombstone_state}" "2|t|t|t" \
	"post-entitlement permanent cleanup tombstone"
grep -Fq 'RIMS_M9_CLEANUP_COUNTS {"pending":0,"tombstones":' "${post_entitlement_b_log}" ||
	fail "worker B did not report pending and retained tombstone evidence separately"

guard_suffix="$$-${RANDOM}"
guard_ordinary_key="2026/07/ordinary-after-check-${guard_suffix}.bin"
sql "DELETE FROM file_attachments WHERE object_key = '${guard_ordinary_key}';
DELETE FROM rims_dev_fixture_attachment_cleanup WHERE object_key = '${guard_ordinary_key}';
DELETE FROM documents WHERE doc_no = 'M9-GUARD-ORDINARY-${guard_suffix}';
INSERT INTO documents (
  doc_no, doc_type, status, warehouse_id, remark, created_by, updated_by
)
SELECT 'M9-GUARD-ORDINARY-${guard_suffix}', 2, 1, w.id,
       'ordinary attachment inserted after cleanup reference check', u.id, u.id
FROM warehouses w, users u
WHERE w.code = 'WH001' AND w.deleted_at IS NULL
  AND u.username = 'admin' AND u.deleted_at IS NULL;
INSERT INTO rims_dev_fixture_attachment_cleanup (
  object_key, source_document_id, source_doc_no, source_remark
) VALUES (
  '${guard_ordinary_key}', 1, 'M9DOC-TOCTOU-GUARD', 'M9-E2E: post-check insert guard'
);"
set +e
guard_insert_log="$(sql "INSERT INTO file_attachments (
  business_type, business_id, object_key, file_url, original_name,
  file_size, file_hash, mime_type, created_by, updated_by
)
SELECT 'doc_attachment', d.id, '${guard_ordinary_key}',
       '/uploads/${guard_ordinary_key}', 'ordinary.bin', 8, repeat('b', 64),
       'application/octet-stream', d.created_by, d.updated_by
FROM documents d WHERE d.doc_no = 'M9-GUARD-ORDINARY-${guard_suffix}';" 2>&1)"
guard_insert_exit=$?
set -e
[[ "${guard_insert_exit}" -ne 0 ]] ||
	fail "ordinary attachment entered the reserved fixture namespace after cleanup check"
grep -Fq 'fixture cleanup history owns object key' <<< "${guard_insert_log}" ||
	fail "post-check pending-key rejection was not diagnosable"
assert_eq "$(sql "SELECT count(*) FROM file_attachments WHERE object_key = '${guard_ordinary_key}'")" "0" \
	"ordinary pending-key attachment rejection"
assert_eq "$(sql "SELECT count(*) FROM rims_dev_fixture_attachment_cleanup WHERE object_key = '${guard_ordinary_key}'")" "1" \
	"post-check cleanup ownership preservation"
sql "DELETE FROM rims_dev_fixture_attachment_cleanup WHERE object_key = '${guard_ordinary_key}';
DELETE FROM documents WHERE doc_no = 'M9-GUARD-ORDINARY-${guard_suffix}';"

lease_active_key="m9-e2e/lease-active-${guard_suffix}.bin"
sql "DELETE FROM rims_dev_fixture_attachment_cleanup WHERE object_key = '${lease_active_key}';
INSERT INTO rims_dev_fixture_attachment_cleanup (
  object_key, source_document_id, source_doc_no, source_remark,
  claim_token, claim_version, claimed_at
) VALUES (
  '${lease_active_key}', 1, 'M9DOC-LEASE-ACTIVE', 'M9-E2E: active lease',
  'active-owner', 9, CURRENT_TIMESTAMP
);"
active_lease_log="${GUARD_TMP_DIR}/active-lease.log"
if RIMS_ALLOW_DEV_SEED=1 RIMS_ENV_FILE="${ENV_FILE}" \
	RIMS_M9_CLAIM_LEASE_MS=60000 bash "${SEED_SCRIPT}" --reset >"${active_lease_log}" 2>&1; then
	fail "reset reported success while another cleanup claim lease was active"
fi
assert_eq "$(sql "SELECT concat_ws('|', claim_token, claim_version) FROM rims_dev_fixture_attachment_cleanup WHERE object_key = '${lease_active_key}'")" \
	"active-owner|9" "unexpired cleanup claim ownership"
sql "DELETE FROM rims_dev_fixture_attachment_cleanup WHERE object_key = '${lease_active_key}';"

lease_expired_key="m9-e2e/lease-expired-${guard_suffix}.bin"
lease_expired_upload_dir="${UPLOAD_DIR:-./uploads}"
if [[ "${lease_expired_upload_dir}" != /* ]]; then
	lease_expired_upload_dir="${REPO_ROOT}/${lease_expired_upload_dir#./}"
fi
lease_expired_upload_dir="$(realpath -m -- "${lease_expired_upload_dir}")"
lease_expired_path="${lease_expired_upload_dir}/${lease_expired_key}"
mkdir -p "$(dirname "${lease_expired_path}")"
printf 'expired lease bytes\n' > "${lease_expired_path}"
sql "DELETE FROM rims_dev_fixture_attachment_cleanup WHERE object_key = '${lease_expired_key}';
INSERT INTO rims_dev_fixture_attachment_cleanup (
  object_key, source_document_id, source_doc_no, source_remark,
  claim_token, claim_version, claimed_at
) VALUES (
  '${lease_expired_key}', 1, 'M9DOC-LEASE-EXPIRED', 'M9-E2E: expired lease',
  'killed-owner', 9, CURRENT_TIMESTAMP - INTERVAL '1 hour'
);"
RIMS_ALLOW_DEV_SEED=1 RIMS_ENV_FILE="${ENV_FILE}" \
	RIMS_M9_CLAIM_LEASE_MS=1000 bash "${SEED_SCRIPT}" --reset >/dev/null
assert_eq "$(sql "SELECT concat_ws('|', claim_version, completed_at IS NOT NULL,
  claim_token IS NULL, claimed_at IS NULL)
FROM rims_dev_fixture_attachment_cleanup WHERE object_key = '${lease_expired_key}'")" \
	"10|t|t|t" "expired cleanup claim reclamation tombstone"
[[ ! -e "${lease_expired_path}" ]] || fail "expired cleanup claim did not delete its fixture bytes"
printf 'manual recreation behind historical tombstone\n' > "${lease_expired_path}"
RIMS_ALLOW_DEV_SEED=1 RIMS_ENV_FILE="${ENV_FILE}" \
	RIMS_M9_CLAIM_LEASE_MS=1000 bash "${SEED_SCRIPT}" --reset >/dev/null
assert_eq "$(sql "SELECT concat_ws('|', claim_version, completed_at IS NOT NULL)
FROM rims_dev_fixture_attachment_cleanup WHERE object_key = '${lease_expired_key}'")" \
	"10|t" "completed cleanup history is not reclaimed"
[[ -f "${lease_expired_path}" ]] || fail "reset reclaimed a completed tombstone key"
/bin/rm -f -- "${lease_expired_path}"
sql "DELETE FROM rims_dev_fixture_attachment_cleanup WHERE object_key = '${lease_expired_key}';"

lease_version_key="m9-e2e/lease-version-${guard_suffix}.bin"
sql "DELETE FROM rims_dev_fixture_attachment_cleanup WHERE object_key = '${lease_version_key}';
INSERT INTO rims_dev_fixture_attachment_cleanup (
  object_key, source_document_id, source_doc_no, source_remark,
  claim_token, claim_version, claimed_at
) VALUES (
  '${lease_version_key}', 1, 'M9DOC-LEASE-VERSION', 'M9-E2E: claim version',
  'new-owner', 11, CURRENT_TIMESTAMP
);
UPDATE rims_dev_fixture_attachment_cleanup
SET completed_at = CURRENT_TIMESTAMP,
    claim_token = NULL,
    claimed_at = NULL
WHERE object_key = '${lease_version_key}'
  AND claim_token = 'old-owner'
  AND claim_version = 10
  AND completed_at IS NULL;"
assert_eq "$(sql "SELECT concat_ws('|', claim_token, claim_version) FROM rims_dev_fixture_attachment_cleanup WHERE object_key = '${lease_version_key}'")" \
	"new-owner|11" "old owner finalize fencing"
sql "DELETE FROM rims_dev_fixture_attachment_cleanup WHERE object_key = '${lease_version_key}';"

lock_holder_log="${GUARD_TMP_DIR}/advisory-lock-holder.log"
timeout 3s "${PSQL[@]}" -c "
BEGIN;
SET LOCAL statement_timeout = '2500ms';
SELECT pg_advisory_xact_lock(908130011);
SELECT pg_sleep(2);
COMMIT;
" >"${lock_holder_log}" 2>&1 &
LOCK_HOLDER_PID=$!
lock_ready=false
for _ in $(seq 1 50); do
	if [[ "$(sql "SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND granted AND classid = 0 AND objid = 908130011;")" -gt 0 ]]; then
		lock_ready=true
		break
	fi
	if ! kill -0 "${LOCK_HOLDER_PID}" >/dev/null 2>&1; then
		break
	fi
	sleep 0.05
done
[[ "${lock_ready}" == true ]] || fail "advisory lock holder did not become ready before its deadline: $(tr '\n' ' ' < "${lock_holder_log}")"

lock_wait_log="${GUARD_TMP_DIR}/advisory-lock-wait.log"
set +e
timeout 1s env \
	RIMS_ALLOW_DEV_SEED=1 \
	RIMS_ENV_FILE="${ENV_FILE}" \
	RIMS_M9_ADVISORY_LOCK_TIMEOUT_MS=150 \
	bash "${SEED_SCRIPT}" >"${lock_wait_log}" 2>&1
lock_wait_exit=$?
set -e
wait "${LOCK_HOLDER_PID}" >/dev/null 2>&1 || true
LOCK_HOLDER_PID=''
[[ "${lock_wait_exit}" -ne 0 ]] || fail "seed acquired a held fixture advisory lock instead of respecting its deadline"
[[ "${lock_wait_exit}" -ne 124 ]] || fail "advisory lock wait exceeded the self-test hard deadline"
grep -qi 'lock timeout' "${lock_wait_log}" ||
	fail "advisory lock failure did not report the database lock timeout"

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
assert_eq "$(sql "SELECT i.quantity FROM inventories i JOIN products p ON p.id = i.product_id JOIN warehouses w ON w.id = i.warehouse_id WHERE p.code = 'M9-PAGE-0001' AND w.code = 'WH001' AND i.deleted_at IS NULL")" "2" "WH001 M9-PAGE-0001 quantity"
assert_eq "$(sql "SELECT i.quantity FROM inventories i JOIN products p ON p.id = i.product_id JOIN warehouses w ON w.id = i.warehouse_id WHERE p.code = 'M9-PAGE-0001' AND w.code = 'M9-WH-02' AND i.deleted_at IS NULL")" "12" "M9-WH-02 M9-PAGE-0001 quantity"
assert_eq "$(sql "SELECT barcode FROM products WHERE code = 'M9-PAGE-0001' AND deleted_at IS NULL")" "M10-ACTIVE-001" "M10 active barcode"
assert_eq "$(sql "SELECT concat_ws('|', barcode, status) FROM products WHERE code = 'M9-PAGE-0002' AND deleted_at IS NULL")" "M10-DISABLED-001|0" "M10 disabled barcode"
assert_eq "$(sql "SELECT barcode FROM products WHERE code = 'M9-PAGE-0003' AND deleted_at IS NULL")" "M10-WH001-ONLY-001" "M10 warehouse barcode"
assert_eq "$(sql "SELECT i.status FROM inventories i JOIN products p ON p.id = i.product_id JOIN warehouses w ON w.id = i.warehouse_id WHERE p.code = 'M9-PAGE-0003' AND w.code = 'WH001' AND i.deleted_at IS NULL")" "1" "M10 WH001 barcode inventory status"
assert_eq "$(sql "SELECT i.status FROM inventories i JOIN products p ON p.id = i.product_id JOIN warehouses w ON w.id = i.warehouse_id WHERE p.code = 'M9-PAGE-0003' AND w.code = 'M9-WH-02' AND i.deleted_at IS NULL")" "0" "M10 wrong-warehouse inventory status"
assert_eq "$(sql "SELECT concat_ws('|', d.doc_no, w.code, d.remark) FROM documents d JOIN warehouses w ON w.id = d.warehouse_id WHERE d.doc_no = 'M9DOC0001' AND d.deleted_at IS NULL")" "M9DOC0001|WH001|M10 attachment target WH001" "M10 WH001 attachment document"
assert_eq "$(sql "SELECT concat_ws('|', d.doc_no, w.code, d.remark) FROM documents d JOIN warehouses w ON w.id = d.warehouse_id WHERE d.doc_no = 'M9DOC0002' AND d.deleted_at IS NULL")" "M9DOC0002|M9-WH-02|M10 attachment target M9-WH-02" "M10 second-warehouse attachment document"

for warehouse_code in WH001 M9-WH-02; do
	assert_eq "$(sql "SELECT count(*) FROM inventories i JOIN products p ON p.id = i.product_id JOIN warehouses w ON w.id = i.warehouse_id WHERE p.code LIKE 'M9-PAGE-%' AND w.code = '${warehouse_code}' AND i.deleted_at IS NULL")" "45" "${warehouse_code} fixture inventory"
done
assert_eq "$(sql "SELECT count(*) FROM inventories i JOIN products p ON p.id = i.product_id JOIN warehouses w ON w.id = i.warehouse_id WHERE p.code LIKE 'M9-PAGE-%' AND w.code = 'WH001' AND i.deleted_at IS NULL AND i.quantity <= i.alert_threshold")" "5" "WH001 low-stock rows"
assert_eq "$(sql "SELECT count(*) FROM inventories i JOIN products p ON p.id = i.product_id JOIN warehouses w ON w.id = i.warehouse_id WHERE p.code LIKE 'M9-PAGE-%' AND w.code = 'M9-WH-02' AND i.deleted_at IS NULL AND i.quantity <= i.alert_threshold")" "0" "M9-WH-02 low-stock rows"

assert_eq "$(sql "SELECT count(*) FROM products WHERE code NOT LIKE 'M9-%'")" "${non_fixture_products_before}" "non-fixture products"
assert_eq "$(sql "SELECT count(*) FROM documents WHERE doc_no NOT LIKE 'M9DOC%' AND remark NOT LIKE 'M9-E2E:%'")" "${non_fixture_documents_before}" "non-fixture documents"

reset_probe_suffix="$$-${RANDOM}"
reset_probe_object_key="m9-e2e/reset-probe-${reset_probe_suffix}.bin"
reset_probe_non_fixture_key="manual/m9-nonfixture-${reset_probe_suffix}.bin"
sql "DELETE FROM file_attachments WHERE object_key = '${reset_probe_object_key}';
DELETE FROM documents WHERE doc_no = 'M9E2E-RESET-PROBE';
INSERT INTO documents (
  doc_no, doc_type, status, warehouse_id, remark, created_by, updated_by
)
SELECT 'M9E2E-RESET-PROBE', 2, 1, w.id, 'M9-E2E: reset probe', u.id, u.id
FROM warehouses w, users u
WHERE w.code = 'WH001' AND w.deleted_at IS NULL
  AND u.username = 'admin' AND u.deleted_at IS NULL;"
assert_eq "$(sql "SELECT count(*) FROM documents WHERE remark = 'M9-E2E: reset probe'")" "1" "reset probe setup"

reset_upload_dir="${UPLOAD_DIR:-./uploads}"
if [[ "${reset_upload_dir}" != /* ]]; then
	reset_upload_dir="${REPO_ROOT}/${reset_upload_dir#./}"
fi
reset_upload_dir="$(realpath -m -- "${reset_upload_dir}")"
mkdir -p "${reset_upload_dir}/m9-e2e" "${reset_upload_dir}/manual"
RESET_PROBE_FIXTURE_FILE="${reset_upload_dir}/${reset_probe_object_key}"
RESET_PROBE_NON_FIXTURE_FILE="${reset_upload_dir}/${reset_probe_non_fixture_key}"
printf 'fixture attachment\n' > "${RESET_PROBE_FIXTURE_FILE}"
printf 'unrelated attachment\n' > "${RESET_PROBE_NON_FIXTURE_FILE}"
sql "INSERT INTO file_attachments (
  business_type, business_id, object_key, file_url, original_name,
  file_size, file_hash, mime_type, created_by, updated_by
)
SELECT
  'doc_attachment', d.id, '${reset_probe_object_key}',
  '/uploads/${reset_probe_object_key}', 'reset-probe.bin',
  19, repeat('a', 64), 'application/octet-stream', d.created_by, d.updated_by
FROM documents d
WHERE d.doc_no = 'M9E2E-RESET-PROBE';"
assert_eq "$(sql "SELECT count(*) FROM file_attachments WHERE object_key = '${reset_probe_object_key}'")" "1" "reset attachment setup"

RIMS_ALLOW_DEV_SEED=1 RIMS_ENV_FILE="${ENV_FILE}" bash "${SEED_SCRIPT}" --reset
reset_fingerprint="$(fixture_fingerprint)"
assert_eq "${reset_fingerprint}" "${second_fingerprint}" "fixture fingerprint after reset"
assert_eq "$(sql "SELECT count(*) FROM documents WHERE remark = 'M9-E2E: reset probe'")" "0" "M9 E2E reset probe cleanup"
assert_eq "$(sql "SELECT count(*) FROM file_attachments WHERE object_key = '${reset_probe_object_key}'")" "0" "M9 E2E attachment row cleanup"
assert_eq "$(sql "SELECT count(*) FROM rims_dev_fixture_attachment_cleanup WHERE completed_at IS NULL")" \
	"0" "pending fixture attachment cleanup"
assert_eq "$(sql "SELECT concat_ws('|', completed_at IS NOT NULL, claim_token IS NULL, claimed_at IS NULL)
FROM rims_dev_fixture_attachment_cleanup WHERE object_key = '${reset_probe_object_key}'")" \
	"t|t|t" "retained fixture cleanup tombstone"
[[ ! -e "${RESET_PROBE_FIXTURE_FILE}" ]] || fail "M9 E2E attachment file survived reset"
[[ -f "${RESET_PROBE_NON_FIXTURE_FILE}" ]] || fail "reset removed a non-fixture attachment file"
assert_eq "$(sql "SELECT count(*) FROM products WHERE code NOT LIKE 'M9-%'")" "${non_fixture_products_before}" "non-fixture products after reset"
assert_eq "$(sql "SELECT count(*) FROM documents WHERE doc_no NOT LIKE 'M9DOC%' AND remark NOT LIKE 'M9-E2E:%'")" "${non_fixture_documents_before}" "non-fixture documents after reset"
sql "DELETE FROM rims_dev_fixture_attachment_cleanup WHERE object_key = '${reset_probe_object_key}';"

echo "M9 development seed idempotency and reset test passed: ${reset_fingerprint}"
