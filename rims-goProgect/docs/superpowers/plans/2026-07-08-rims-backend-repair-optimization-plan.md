# RIMS Backend Repair Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the M8 integration repairs into durable backend contracts, route authorization coverage, list-query consistency, and deletion-conflict guardrails.

**Architecture:** Keep the current Gin + GORM modular backend architecture. Repair work should stay inside each module boundary, with route authorization in module routes, business invariants in services, persistence mechanics in repositories, and repeatable API proof scripts under backend test or script locations.

**Tech Stack:** Go, Gin, GORM, PostgreSQL-compatible migrations, WSL Go toolchain at `~/local/go/bin/go`, PowerShell, curl, existing backend migrations and tests.

---

## Current Baseline

The M8 integration pass found four backend defects and repaired them:

- `M8-B-001`: current warehouse selection did not persist for session restore.
- `M8-B-002`: normal users could read role and permission metadata.
- `M8-C-001`: inventory keyword search returned filtered totals with unfiltered rows.
- `M8-F-001`: warehouse deletion was allowed while active user bindings existed.

Detailed defect evidence is recorded in:

```text
docs/superpowers/plans/2026-07-08-rims-m8-backend-integration-defect-summary.md
```

## Files And Ownership

- Modify: `internal/modules/*/routes.go`
  - Responsibility: route registration, authentication middleware, permission middleware.
- Modify: `internal/modules/*/service.go`
  - Responsibility: business state transitions and deletion-conflict policy.
- Modify: `internal/modules/*/repository.go`
  - Responsibility: query construction, count/list consistency, persistence helpers.
- Modify or create: `internal/modules/**/*_test.go`
  - Responsibility: route permission tests, service invariant tests, repository query tests.
- Modify: `migrations/*.sql`
  - Responsibility: permission seed data and data-contract migrations.
- Create: `scripts/m8_backend_smoke.sh`
  - Responsibility: repeatable backend API probes for the repaired integration contracts.
- Create: `docs/superpowers/plans/2026-07-08-rims-backend-permission-matrix.md`
  - Responsibility: human-readable route-permission matrix.

## Task 1: Freeze The Repaired M8 Baseline

**Files:**
- Read: `docs/superpowers/plans/2026-07-08-rims-m8-backend-integration-defect-summary.md`
- Read: `internal/modules/product/repository.go`
- Read: `internal/modules/user/routes.go`
- Read: `internal/modules/warehouse/service.go`
- Test: `internal/modules/product/repository_test.go`
- Test: `internal/modules/user/handler_role_auth_test.go`
- Test: `internal/modules/warehouse/routes_permission_test.go`

- [ ] **Step 1: Confirm backend test baseline**

Run:

```powershell
wsl -e bash -lc 'cd "/mnt/e/My Work/RIMS/rims-goProgect" && ~/local/go/bin/go test ./internal/modules/user ./internal/modules/warehouse ./internal/modules/product'
```

Expected:

```text
ok  	.../internal/modules/user
ok  	.../internal/modules/warehouse
ok  	.../internal/modules/product
```

- [ ] **Step 2: Confirm full backend baseline**

Run:

```powershell
wsl -e bash -lc 'cd "/mnt/e/My Work/RIMS/rims-goProgect" && ~/local/go/bin/go test ./...'
wsl -e bash -lc 'cd "/mnt/e/My Work/RIMS/rims-goProgect" && ~/local/go/bin/go build ./...'
```

Expected:

```text
go test ./... exits 0
go build ./... exits 0
```

- [ ] **Step 3: Confirm working tree scope**

Run:

```powershell
git -C "E:\My Work\RIMS\rims-goProgect" status --short
```

Expected changed files are limited to the repaired backend files and the two backend handoff documents:

```text
internal/modules/product/repository.go
internal/modules/product/repository_test.go
internal/modules/user/handler_role_auth_test.go
internal/modules/user/routes.go
internal/modules/warehouse/routes_permission_test.go
internal/modules/warehouse/service.go
migrations/000013_role_read_permission_seed.sql
docs/superpowers/plans/2026-07-08-rims-m8-backend-integration-defect-summary.md
docs/superpowers/plans/2026-07-08-rims-backend-repair-optimization-plan.md
```

- [ ] **Step 4: Commit the repaired baseline**

Run after reviewing the diff:

```powershell
git -C "E:\My Work\RIMS\rims-goProgect" add internal/modules/product/repository.go internal/modules/product/repository_test.go internal/modules/user/handler_role_auth_test.go internal/modules/user/routes.go internal/modules/warehouse/routes_permission_test.go internal/modules/warehouse/service.go migrations/000013_role_read_permission_seed.sql docs/superpowers/plans/2026-07-08-rims-m8-backend-integration-defect-summary.md docs/superpowers/plans/2026-07-08-rims-backend-repair-optimization-plan.md
git -C "E:\My Work\RIMS\rims-goProgect" commit -m "fix: repair M8 backend integration blockers"
```

Expected:

```text
[branch commit] fix: repair M8 backend integration blockers
```

## Task 2: Harden Route Permission Coverage

**Files:**
- Modify: `internal/modules/*/routes.go`
- Create: `docs/superpowers/plans/2026-07-08-rims-backend-permission-matrix.md`
- Modify or create: `internal/modules/**/*_permission_test.go`
- Modify when new permissions are required: `migrations/*.sql`

- [ ] **Step 1: List all backend route registrations**

Run:

```powershell
wsl -e bash -lc 'cd "/mnt/e/My Work/RIMS/rims-goProgect" && rg -n "GET|POST|PUT|PATCH|DELETE" internal/modules -g "routes.go"'
```

Expected:

```text
Every module route registration is printed with file and line number.
```

- [ ] **Step 2: Create the permission matrix document**

Create `docs/superpowers/plans/2026-07-08-rims-backend-permission-matrix.md` with this structure:

```markdown
# RIMS Backend Permission Matrix

| Method | Path | Module | Auth Required | Permission Required | Public Reason |
| --- | --- | --- | --- | --- | --- |
| GET | /healthz | system | No | None | Health probe |
| GET | /api/v1/users/me | user | Yes | None | Current authenticated user's own profile |
| GET | /api/v1/roles | user | Yes | role:list | Management metadata |
| GET | /api/v1/roles/:id | user | Yes | role:read | Management metadata |
| GET | /api/v1/permissions | user | Yes | permission:list | Management metadata |
```

Extend the table with every route found in Step 1. Routes with `Auth Required=Yes` and `Permission Required=None` must have a concrete self-service reason in `Public Reason`.

- [ ] **Step 3: Add missing permission tests before changing routes**

For each management route that lacks a permission gate, add a test following the existing pattern in `internal/modules/user/handler_role_auth_test.go` and `internal/modules/warehouse/routes_permission_test.go`.

Each test must prove two outcomes:

```text
normal authenticated user without the permission receives 403
admin or explicitly permitted user receives the expected success status
```

Run a focused test command for the module being changed:

```powershell
wsl -e bash -lc 'cd "/mnt/e/My Work/RIMS/rims-goProgect" && ~/local/go/bin/go test ./internal/modules/user'
```

Expected before implementation:

```text
FAIL because the unprotected route returns 200 for the normal user.
```

- [ ] **Step 4: Add permission middleware gates**

In the relevant `routes.go` file, apply the same permission middleware style already used by protected routes.

Required baseline gates:

```text
GET /api/v1/roles -> role:list
GET /api/v1/roles/:id -> role:read
GET /api/v1/permissions -> permission:list
```

For newly discovered management routes, use the narrowest action name matching the route:

```text
<resource>:list for list routes
<resource>:read for detail routes
<resource>:create for create routes
<resource>:update for update routes
<resource>:delete for delete routes
<resource>:manage only when the route performs mixed administrative work
```

- [ ] **Step 5: Seed any new permissions**

When Step 4 introduces a permission that does not exist in seed migrations, add a new numbered migration under `migrations/`.

The migration must:

```text
insert the permission idempotently
grant the permission to the admin role idempotently
leave existing role assignments unchanged
```

Run:

```powershell
wsl -e bash -lc 'cd "/mnt/e/My Work/RIMS/rims-goProgect" && ~/local/go/bin/go test ./internal/modules/user ./internal/modules/warehouse'
```

Expected:

```text
All permission tests pass.
```

## Task 3: Make Current Warehouse Contract Explicit

**Files:**
- Modify: `internal/modules/warehouse/service.go`
- Modify: `internal/modules/warehouse/**/*_test.go`
- Modify if response DTOs exist separately: `internal/modules/warehouse/**/model*.go`
- Modify if user profile DTOs expose warehouse state: `internal/modules/user/**/*.go`
- Modify: `docs/swagger.yaml`
- Modify: `docs/swagger.json`
- Modify: `docs/docs.go`

- [ ] **Step 1: Document the current behavior with a service or handler test**

Add or extend a test that performs this sequence:

```text
bind user to warehouse A and warehouse B
switch current warehouse to B
call the API or service used by session restore
assert B is the current marker
switch current warehouse back to A
assert A is the current marker
```

Run:

```powershell
wsl -e bash -lc 'cd "/mnt/e/My Work/RIMS/rims-goProgect" && ~/local/go/bin/go test ./internal/modules/warehouse'
```

Expected:

```text
The test passes with the existing repaired behavior.
```

- [ ] **Step 2: Choose and encode the contract**

Use one of these two contracts and document it in Swagger:

```text
Contract A: isDefault is the current warehouse marker for this app.
Contract B: session APIs return explicit current fields such as isCurrent and currentWarehouseId.
```

Prefer Contract B if backend consumers need to distinguish "default at login" from "current during this session".

- [ ] **Step 3: Preserve frontend compatibility**

If Contract B is implemented, keep `isDefault` stable for one release and add the explicit current field instead of removing or renaming existing fields.

Expected response shape for the richer contract:

```json
{
  "id": 2,
  "name": "Warehouse B",
  "isDefault": true,
  "isCurrent": true
}
```

- [ ] **Step 4: Regenerate or update API docs**

Run the repository's established Swagger generation command. If the project uses swag, run:

```powershell
wsl -e bash -lc 'cd "/mnt/e/My Work/RIMS/rims-goProgect" && swag init'
```

Expected:

```text
docs/docs.go, docs/swagger.json, and docs/swagger.yaml reflect the current warehouse contract.
```

- [ ] **Step 5: Verify full compatibility**

Run:

```powershell
wsl -e bash -lc 'cd "/mnt/e/My Work/RIMS/rims-goProgect" && ~/local/go/bin/go test ./internal/modules/warehouse ./internal/modules/user'
```

Expected:

```text
All warehouse and user module tests pass.
```

## Task 4: Standardize Count/List Query Consistency

**Files:**
- Modify: `internal/modules/product/repository.go`
- Inspect and modify as needed: `internal/modules/**/repository.go`
- Test: `internal/modules/product/repository_test.go`
- Create or modify tests: `internal/modules/**/*repository_test.go`

- [ ] **Step 1: Find list methods with separate count and row queries**

Run:

```powershell
wsl -e bash -lc 'cd "/mnt/e/My Work/RIMS/rims-goProgect" && rg -n "Count\\(|Find\\(|Offset\\(|Limit\\(" internal/modules -g "*repository.go"'
```

Expected:

```text
Repository list methods with pagination and counting are visible for review.
```

- [ ] **Step 2: Add failing tests for any divergence risk**

For each list method with filters, create a test that:

```text
creates at least two records
applies a filter that should match exactly one record
asserts total equals 1
asserts len(list) equals 1
asserts the returned row is the matched record
```

Use `TestInventoryRepositoryListByWarehouseAppliesKeywordToRowsAndTotal` as the model test.

- [ ] **Step 3: Reuse filtered query construction**

For each repaired repository method, structure the query so count and rows share the same filter-building path:

```go
query := r.getDB(ctx).Model(&Entity{}).Where("warehouse_id = ?", warehouseID)
if keyword != "" {
    query = query.Where("name ILIKE ? OR code ILIKE ?", "%"+keyword+"%", "%"+keyword+"%")
}
if err := query.Count(&total).Error; err != nil {
    return nil, 0, err
}
if err := query.Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
    return nil, 0, err
}
```

When a query needs joins or preloads, keep joins in the filtered query and add preloads only before the row `Find`.

- [ ] **Step 4: Run repository tests**

Run:

```powershell
wsl -e bash -lc 'cd "/mnt/e/My Work/RIMS/rims-goProgect" && ~/local/go/bin/go test ./internal/modules/product'
```

Expected:

```text
All product repository tests pass.
```

- [ ] **Step 5: Run full backend tests**

Run:

```powershell
wsl -e bash -lc 'cd "/mnt/e/My Work/RIMS/rims-goProgect" && ~/local/go/bin/go test ./...'
```

Expected:

```text
All backend tests pass.
```

## Task 5: Harden Deletion-Conflict Policy

**Files:**
- Modify: `internal/modules/warehouse/service.go`
- Modify: `internal/modules/warehouse/repository.go`
- Modify or create: `internal/modules/warehouse/*_test.go`
- Inspect and modify as needed: `internal/modules/product/service.go`
- Inspect and modify as needed: `internal/modules/user/service.go`

- [ ] **Step 1: Replace list-for-conflict checks with count helpers**

Add repository helpers for conflict checks where the current code uses list queries only to know whether dependencies exist.

Required helper for warehouse bindings:

```go
CountActiveBindingsByWarehouseID(ctx context.Context, warehouseID uint) (int64, error)
```

Expected behavior:

```text
returns the number of active user-warehouse bindings for a warehouse
excludes soft-deleted bindings
does not load full binding rows
```

- [ ] **Step 2: Keep service-level business error behavior stable**

The warehouse delete service must continue returning:

```text
ErrInvalidState("仓库已绑定用户，无法删除")
```

The API must continue returning:

```text
HTTP 422
```

- [ ] **Step 3: Add conflict tests for every destructive management operation**

For each delete operation that can break a user workflow or historical record, add tests that prove:

```text
delete is rejected while dependent active records exist
delete succeeds after dependencies are removed or deactivated
delete returns a stable business error status
```

Minimum operations to inspect:

```text
warehouse deletion with user bindings
product deletion with inventory or document usage
user deletion with active warehouse bindings
role deletion with assigned users
```

- [ ] **Step 4: Run affected module tests**

Run:

```powershell
wsl -e bash -lc 'cd "/mnt/e/My Work/RIMS/rims-goProgect" && ~/local/go/bin/go test ./internal/modules/warehouse ./internal/modules/product ./internal/modules/user'
```

Expected:

```text
All destructive-operation tests pass.
```

## Task 6: Add Repeatable Backend M8 Smoke Probes

**Files:**
- Create: `scripts/m8_backend_smoke.sh`
- Modify: `README.md`
- Reference: `docs/superpowers/plans/2026-07-08-rims-m8-backend-integration-defect-summary.md`

- [ ] **Step 1: Create smoke script with explicit probes**

Create `scripts/m8_backend_smoke.sh`.

The script must check:

```text
health endpoint returns ok
admin can list roles
operator cannot list roles
operator cannot list permissions
current warehouse switch is visible after session restore
inventory keyword search has matching total and row count
warehouse delete with active binding returns 422
```

- [ ] **Step 2: Make the script configurable**

Use these environment variables:

```bash
BASE_URL="${BASE_URL:-http://localhost:8080}"
ADMIN_USERNAME="${ADMIN_USERNAME:-admin}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-admin123}"
OPERATOR_USERNAME="${OPERATOR_USERNAME:-m8_operator_20260707164313}"
OPERATOR_PASSWORD="${OPERATOR_PASSWORD:-password123}"
```

The script must exit non-zero on the first failed assertion and print the failing probe name.

- [ ] **Step 3: Document how to run the smoke**

Add this section to `README.md`:

````markdown
## M8 Backend Smoke

Run after migrations are applied and the API server is available:

```bash
BASE_URL=http://localhost:8080 ./scripts/m8_backend_smoke.sh
```

The smoke verifies the repaired M8 backend contracts for permissions, current warehouse restore, inventory search consistency, and deletion conflicts.
````

- [ ] **Step 4: Run the smoke against local backend**

Run:

```powershell
wsl -e bash -lc 'cd "/mnt/e/My Work/RIMS/rims-goProgect" && BASE_URL=http://localhost:8080 ./scripts/m8_backend_smoke.sh'
```

Expected:

```text
All M8 backend smoke probes pass.
```

## Exit Criteria

- `go test ./...` passes.
- `go build ./...` passes.
- Permission matrix exists and every authenticated management route has either a permission gate or a documented self-service reason.
- Current warehouse restore contract is explicit in tests and API docs.
- Filtered list endpoints tested during this loop have matching `total` and returned rows.
- Destructive management operations reject active dependency conflicts.
- `scripts/m8_backend_smoke.sh` passes against local backend.
- Frontend smoke still passes from `E:\My Work\rims-frontend`:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\rims_smoke.ps1
```

## Commit Sequence

Use small commits in this order:

```text
fix: lock M8 backend integration repairs
test: cover backend route permission matrix
fix: make warehouse current contract explicit
fix: standardize backend list query filtering
fix: enforce deletion conflict policy
test: add M8 backend smoke probes
docs: record backend integration hardening plan
```
