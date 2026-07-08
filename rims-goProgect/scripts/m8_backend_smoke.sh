#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (c) 2026 ShangBin Wang

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
ADMIN_USERNAME="${ADMIN_USERNAME:-admin}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-admin123}"
OPERATOR_USERNAME="${OPERATOR_USERNAME:-m8_operator_20260707164313}"
OPERATOR_PASSWORD="${OPERATOR_PASSWORD:-password123}"
INVENTORY_KEYWORD="${INVENTORY_KEYWORD:-sale_1776407432915}"

BASE_URL="${BASE_URL%/}"
TMP_DIR="$(mktemp -d)"
LAST_BODY=""
CREATED_WAREHOUSE_ID=""
ADMIN_USER_ID=""
CREATED_OPERATOR_ID=""
CREATED_OPERATOR_WAREHOUSE_IDS=()

cleanup() {
	if [[ -n "${CREATED_WAREHOUSE_ID}" && -n "${ADMIN_TOKEN:-}" && -n "${ADMIN_USER_ID}" ]]; then
		api "cleanup unbind admin warehouse" DELETE "/api/v1/warehouses/${CREATED_WAREHOUSE_ID}/users/${ADMIN_USER_ID}" 204 "${ADMIN_TOKEN}" "" true || true
		api "cleanup delete warehouse" DELETE "/api/v1/warehouses/${CREATED_WAREHOUSE_ID}" 204 "${ADMIN_TOKEN}" "" true || true
	fi
	if [[ -n "${CREATED_OPERATOR_ID}" && -n "${ADMIN_TOKEN:-}" ]]; then
		for warehouse_id in "${CREATED_OPERATOR_WAREHOUSE_IDS[@]}"; do
			api "cleanup unbind smoke operator ${warehouse_id}" DELETE "/api/v1/warehouses/${warehouse_id}/users/${CREATED_OPERATOR_ID}" 204 "${ADMIN_TOKEN}" "" true || true
		done
		api "cleanup delete smoke operator" DELETE "/api/v1/users/${CREATED_OPERATOR_ID}" 204 "${ADMIN_TOKEN}" "" true || true
	fi
	rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

fail() {
	local probe="$1"
	local detail="${2:-}"
	echo "FAIL: ${probe}" >&2
	if [[ -n "${detail}" ]]; then
		echo "${detail}" >&2
	fi
	if [[ -n "${LAST_BODY}" && -f "${LAST_BODY}" ]]; then
		echo "--- response body ---" >&2
		cat "${LAST_BODY}" >&2
		echo >&2
	fi
	exit 1
}

safe_name() {
	printf '%s' "$1" | tr -c 'A-Za-z0-9_' '_'
}

api() {
	local probe="$1"
	local method="$2"
	local path="$3"
	local expected_status="$4"
	local token="${5:-}"
	local body="${6:-}"
	local allow_failure="${7:-false}"
	local status

	LAST_BODY="${TMP_DIR}/$(safe_name "${probe}").json"
	local curl_args=(-sS -o "${LAST_BODY}" -w "%{http_code}" -X "${method}" "${BASE_URL}${path}" -H "Accept: application/json")
	if [[ -n "${token}" ]]; then
		curl_args+=(-H "Authorization: Bearer ${token}")
	fi
	if [[ -n "${body}" ]]; then
		curl_args+=(-H "Content-Type: application/json" --data "${body}")
	fi

	if ! status="$(curl "${curl_args[@]}")"; then
		if [[ "${allow_failure}" == "true" ]]; then
			return 1
		fi
		fail "${probe}" "curl request failed"
	fi
	if [[ "${status}" != "${expected_status}" ]]; then
		if [[ "${allow_failure}" == "true" ]]; then
			return 1
		fi
		fail "${probe}" "HTTP ${status}, expected ${expected_status}"
	fi
}

json_value() {
	local file="$1"
	local path="$2"
	python3 - "$file" "$path" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as f:
    value = json.load(f)

for part in sys.argv[2].split("."):
    if not part:
        continue
    if part.isdigit():
        value = value[int(part)]
    else:
        value = value[part]

if isinstance(value, bool):
    print("true" if value else "false")
elif value is None:
    print("")
else:
    print(value)
PY
}

urlencode() {
	python3 - "$1" <<'PY'
import sys
import urllib.parse

print(urllib.parse.quote(sys.argv[1]))
PY
}

assert_success_envelope() {
	local probe="$1"
	python3 - "$LAST_BODY" "$probe" <<'PY' || fail "$probe" "response envelope code was not 0"
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as f:
    payload = json.load(f)
if payload.get("code") != 0:
    raise SystemExit(f"{sys.argv[2]} code={payload.get('code')}")
PY
}

LOGIN_TOKEN=""
LOGIN_USER_ID=""

login_probe() {
	local username="$1"
	local password="$2"
	local probe="login ${username}"
	api "${probe}" POST "/api/v1/auth/login" 200 "" "{\"username\":\"${username}\",\"password\":\"${password}\"}"
	assert_success_envelope "${probe}"
	LOGIN_TOKEN="$(json_value "${LAST_BODY}" "data.token")"
	LOGIN_USER_ID="$(json_value "${LAST_BODY}" "data.user.id")"
}

try_login_probe() {
	local username="$1"
	local password="$2"
	local probe="login ${username}"
	if ! api "${probe}" POST "/api/v1/auth/login" 200 "" "{\"username\":\"${username}\",\"password\":\"${password}\"}" true; then
		return 1
	fi
	assert_success_envelope "${probe}"
	LOGIN_TOKEN="$(json_value "${LAST_BODY}" "data.token")"
	LOGIN_USER_ID="$(json_value "${LAST_BODY}" "data.user.id")"
}

register_temporary_operator() {
	local username="m8_smoke_operator_$(date +%s)"
	local password="${OPERATOR_PASSWORD}"
	api "register temporary operator" POST "/api/v1/auth/register" 201 "" "{\"username\":\"${username}\",\"password\":\"${password}\",\"realName\":\"M8 Smoke Operator\"}"
	assert_success_envelope "register temporary operator"
	OPERATOR_TOKEN="$(json_value "${LAST_BODY}" "data.token")"
	CREATED_OPERATOR_ID="$(json_value "${LAST_BODY}" "data.user.id")"

	api "temporary operator warehouse list" GET "/api/v1/users/me/warehouses" 200 "${OPERATOR_TOKEN}"
	assert_success_envelope "temporary operator warehouse list"
	mapfile -t CREATED_OPERATOR_WAREHOUSE_IDS < <(python3 - "$LAST_BODY" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as f:
    payload = json.load(f)
for item in payload.get("data") or []:
    print(item["warehouseId"])
PY
)
}

api "health endpoint" GET "/healthz" 200
python3 - "$LAST_BODY" <<'PY' || fail "health endpoint" "status was not ok"
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as f:
    payload = json.load(f)
if payload.get("status") != "ok":
    raise SystemExit("health status mismatch")
PY

login_probe "${ADMIN_USERNAME}" "${ADMIN_PASSWORD}"
ADMIN_TOKEN="${LOGIN_TOKEN}"
ADMIN_USER_ID="${LOGIN_USER_ID}"
if try_login_probe "${OPERATOR_USERNAME}" "${OPERATOR_PASSWORD}"; then
	OPERATOR_TOKEN="${LOGIN_TOKEN}"
else
	register_temporary_operator
fi

api "admin can list roles" GET "/api/v1/roles" 200 "${ADMIN_TOKEN}"
assert_success_envelope "admin can list roles"

api "operator cannot list roles" GET "/api/v1/roles" 403 "${OPERATOR_TOKEN}"
if [[ "$(json_value "${LAST_BODY}" "code")" != "10002" ]]; then
	fail "operator cannot list roles" "expected permission denied code 10002"
fi

api "operator cannot list permissions" GET "/api/v1/permissions" 403 "${OPERATOR_TOKEN}"
if [[ "$(json_value "${LAST_BODY}" "code")" != "10002" ]]; then
	fail "operator cannot list permissions" "expected permission denied code 10002"
fi

api "operator warehouse list" GET "/api/v1/users/me/warehouses" 200 "${OPERATOR_TOKEN}"
assert_success_envelope "operator warehouse list"
TARGET_WAREHOUSE_ID="$(python3 - "$LAST_BODY" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as f:
    payload = json.load(f)
items = payload.get("data") or []
if not items:
    raise SystemExit("operator has no warehouses")
current = next((item for item in items if not item.get("isCurrent")), None)
target = current or items[0]
print(target["warehouseId"])
PY
)" || fail "operator warehouse list" "operator has no warehouses"

api "switch current warehouse" PUT "/api/v1/users/me/warehouses/current" 200 "${OPERATOR_TOKEN}" "{\"warehouseId\":${TARGET_WAREHOUSE_ID}}"
assert_success_envelope "switch current warehouse"

api "current warehouse visible after restore" GET "/api/v1/users/me/warehouses" 200 "${OPERATOR_TOKEN}"
assert_success_envelope "current warehouse visible after restore"
python3 - "$LAST_BODY" "$TARGET_WAREHOUSE_ID" <<'PY' || fail "current warehouse visible after restore" "selected warehouse was not marked isCurrent/isDefault"
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as f:
    payload = json.load(f)
target_id = int(sys.argv[2])
matches = [item for item in payload.get("data", []) if int(item.get("warehouseId", 0)) == target_id]
if not matches:
    raise SystemExit("target warehouse missing")
target = matches[0]
if target.get("isCurrent") is not True or target.get("isDefault") is not True:
    raise SystemExit("target warehouse is not current/default")
PY

encoded_keyword="$(urlencode "${INVENTORY_KEYWORD}")"
api "inventory keyword search consistency" GET "/api/v1/inventory?keyword=${encoded_keyword}&page=1&pageSize=20" 200 "${OPERATOR_TOKEN}"
assert_success_envelope "inventory keyword search consistency"
python3 - "$LAST_BODY" <<'PY' || fail "inventory keyword search consistency" "total/list mismatch or missing keyword fixture"
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as f:
    payload = json.load(f)
data = payload.get("data") or {}
items = data.get("list") or []
total = int(data.get("total") or 0)
if total == 0:
    raise SystemExit("keyword returned zero rows")
if total != len(items):
    raise SystemExit(f"total={total}, len={len(items)}")
PY

warehouse_code="M8SMOKE$(date +%s)"
api "create smoke warehouse" POST "/api/v1/warehouses" 201 "${ADMIN_TOKEN}" "{\"code\":\"${warehouse_code}\",\"name\":\"M8 Smoke Warehouse\",\"status\":1}"
assert_success_envelope "create smoke warehouse"
CREATED_WAREHOUSE_ID="$(json_value "${LAST_BODY}" "data.id")"

api "bind admin to smoke warehouse" POST "/api/v1/warehouses/${CREATED_WAREHOUSE_ID}/users" 200 "${ADMIN_TOKEN}" "{\"userIds\":[${ADMIN_USER_ID}]}"
assert_success_envelope "bind admin to smoke warehouse"

api "warehouse delete rejects active binding" DELETE "/api/v1/warehouses/${CREATED_WAREHOUSE_ID}" 422 "${ADMIN_TOKEN}"
if [[ "$(json_value "${LAST_BODY}" "code")" != "20002" ]]; then
	fail "warehouse delete rejects active binding" "expected invalid-state code 20002"
fi

api "cleanup unbind admin warehouse" DELETE "/api/v1/warehouses/${CREATED_WAREHOUSE_ID}/users/${ADMIN_USER_ID}" 204 "${ADMIN_TOKEN}"
api "cleanup delete warehouse" DELETE "/api/v1/warehouses/${CREATED_WAREHOUSE_ID}" 204 "${ADMIN_TOKEN}"
CREATED_WAREHOUSE_ID=""

echo "All M8 backend smoke probes passed."
