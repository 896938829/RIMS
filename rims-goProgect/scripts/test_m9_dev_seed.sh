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
for lock_file in "${SEED_SCRIPT}" "${SEED_SQL}"; do
	grep -Fq 'pg_advisory_xact_lock(908130011)' "${lock_file}" ||
		fail "fixture namespace advisory lock missing from ${lock_file}"
	lock_count="$(grep -Fc 'pg_advisory_xact_lock(908130011)' "${lock_file}")"
	timeout_count="$(grep -Fc "set_config('lock_timeout', :'advisory_lock_timeout', true)" "${lock_file}")"
	assert_eq "${timeout_count}" "${lock_count}" "fixture advisory lock deadline coverage in ${lock_file}"
done
if grep -Eq '^DELETE FROM rims_dev_fixture_attachment_cleanup;[[:space:]]*$' "${SEED_SCRIPT}"; then
	fail "reset still contains an unconditional pending-table delete"
fi

GUARD_TMP_DIR="$(mktemp -d)"
RESET_PROBE_FIXTURE_FILE=''
RESET_PROBE_NON_FIXTURE_FILE=''
LOCK_HOLDER_PID=''
cleanup() {
	if [[ -n "${LOCK_HOLDER_PID}" ]]; then
		kill "${LOCK_HOLDER_PID}" >/dev/null 2>&1 || true
		wait "${LOCK_HOLDER_PID}" >/dev/null 2>&1 || true
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

retry_dir="${GUARD_TMP_DIR}/physical-retry"
retry_bin="${retry_dir}/bin"
retry_uploads="${retry_dir}/uploads"
retry_state="${retry_dir}/state"
retry_object_key="2026/07/m11-retry.bin"
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
			xargs -r printf 'RIMS_M9_RESET_OBJECT_KEY %s\n'
	elif [[ "${input}" == *"rims_dev_fixture_attachment_cleanup"* &&
		-e "${state_dir}/pending" ]]; then
		printf '%s' "${object_key}" | base64 | tr -d '\n' |
			xargs -r printf 'RIMS_M9_RESET_OBJECT_KEY %s\n'
	fi
	printf '%s\n' 'RIMS_M9_RESET_COUNTS {"namespaceDocuments":0,"namespaceTransactions":0,"namespaceAttachments":0}'
elif [[ "${input}" == *"RIMS_M9_ACTIVE_ATTACHMENT_REFERENCE_COUNT"* ]]; then
	printf '%s\n' 'RIMS_M9_ACTIVE_ATTACHMENT_REFERENCE_COUNT 0'
elif [[ "${input}" == *"RIMS_M9_CLAIMED_PENDING_COUNT"* ]]; then
	rm -f -- "${state_dir}/pending"
	printf '%s\n' 'RIMS_M9_CLAIMED_PENDING_COUNT 0'
	printf '%s\n' 'RIMS_M9_PENDING_ATTACHMENT_COUNT 0'
	printf '%s\n' 'RIMS_M9_RESET_COUNTS {"namespaceDocuments":0,"namespaceTransactions":0,"namespaceAttachments":0}'
elif [[ "${input}" == *"DELETE FROM rims_dev_fixture_attachment_cleanup"* ]]; then
	rm -f -- "${state_dir}/pending"
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
shared_object_key="2026/07/shared-key.bin"
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
		xargs -r printf 'RIMS_M9_RESET_OBJECT_KEY %s\n'
	printf '%s\n' 'RIMS_M9_RESET_COUNTS {"namespaceDocuments":0,"namespaceTransactions":0,"namespaceAttachments":0}'
elif [[ "${input}" == *"RIMS_M9_ACTIVE_ATTACHMENT_REFERENCE_COUNT"* ]]; then
	printf '%s\n' 'RIMS_M9_ACTIVE_ATTACHMENT_REFERENCE_COUNT 1'
elif [[ "${input}" == *"DELETE FROM rims_dev_fixture_attachment_cleanup"* ]]; then
	rm -f -- "${state_dir}/pending"
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
concurrent_object_key="2026/07/claimed-a.bin"
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
		xargs -r printf 'RIMS_M9_RESET_OBJECT_KEY %s\n'
	printf '%s\n' 'RIMS_M9_RESET_COUNTS {"namespaceDocuments":0,"namespaceTransactions":0,"namespaceAttachments":0}'
elif [[ "${input}" == *"RIMS_M9_ACTIVE_ATTACHMENT_REFERENCE_COUNT"* ]]; then
	printf '%s\n' 'RIMS_M9_ACTIVE_ATTACHMENT_REFERENCE_COUNT 0'
elif [[ "${input}" == *"RIMS_M9_CLAIMED_PENDING_COUNT"* ]]; then
	touch "${state_dir}/pending-b"
	rm -f -- "${state_dir}/pending-a"
	printf '%s\n' 'RIMS_M9_CLAIMED_PENDING_COUNT 0'
	printf 'RIMS_M9_PENDING_ATTACHMENT_COUNT %s\n' "$(pending_count)"
	printf '%s\n' 'RIMS_M9_RESET_COUNTS {"namespaceDocuments":0,"namespaceTransactions":0,"namespaceAttachments":0}'
elif [[ "${input}" == *"DELETE FROM rims_dev_fixture_attachment_cleanup"* ]]; then
	touch "${state_dir}/pending-b"
	if [[ "${input}" == *"claim_token"* ]]; then
		rm -f -- "${state_dir}/pending-a"
	else
		rm -f -- "${state_dir}/pending-a" "${state_dir}/pending-b"
		touch "${state_dir}/concurrent-row-cleared"
	fi
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
assert_eq "$(sql "SELECT count(*) FROM rims_dev_fixture_attachment_cleanup")" "0" "pending fixture attachment cleanup"
[[ ! -e "${RESET_PROBE_FIXTURE_FILE}" ]] || fail "M9 E2E attachment file survived reset"
[[ -f "${RESET_PROBE_NON_FIXTURE_FILE}" ]] || fail "reset removed a non-fixture attachment file"
assert_eq "$(sql "SELECT count(*) FROM products WHERE code NOT LIKE 'M9-%'")" "${non_fixture_products_before}" "non-fixture products after reset"
assert_eq "$(sql "SELECT count(*) FROM documents WHERE doc_no NOT LIKE 'M9DOC%' AND remark NOT LIKE 'M9-E2E:%'")" "${non_fixture_documents_before}" "non-fixture documents after reset"

echo "M9 development seed idempotency and reset test passed: ${reset_fingerprint}"
