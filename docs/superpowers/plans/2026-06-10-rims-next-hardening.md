# RIMS Next Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the next four backend hardening gaps: public upload abuse, real RBAC permission enforcement, remaining high-value audit coverage, and operational migration/cleanup support.

**Architecture:** Keep security invariants in services when they protect business data, and use route middleware for static RBAC checks. Preserve existing warehouse-scope and document type checks, and keep document audit records inside the same business transaction when inventory/status changes are transactional.

**Tech Stack:** Go 1.25, Gin, GORM, PostgreSQL, existing `types.AppError`, WSL-only Go test/build commands.

---

## File Structure

- Modify `rims-goProgect/internal/modules/file/service.go`: reject unbound public `product_image` uploads before storage.
- Modify `rims-goProgect/internal/modules/file/service_acl_test.go`: tests for public image upload rejection and bound image success.
- Modify `rims-goProgect/internal/middleware/permission.go`: new permission middleware.
- Modify `rims-goProgect/internal/middleware/permission_test.go`: middleware unit tests.
- Modify `rims-goProgect/internal/modules/user/repository.go`: expose `HasPermission(ctx, roleID, code)`.
- Modify `rims-goProgect/internal/modules/user/routes.go`, `warehouse/routes.go`, `product/routes.go`, `audit/routes.go`, and `internal/app/router.go`: wire permission middleware to static admin-only routes.
- Modify handler tests in `user`, `warehouse`, and `product` as needed after admin checks move from handlers to middleware.
- Modify `rims-goProgect/migrations/000010_permission_seed.sql`: seed permission codes and grant admin all permissions.
- Modify `product/handler.go`, `warehouse/handler.go`, `file/handler.go`, `user/handler.go`: add missing handler-level audit records.
- Modify `document/service.go` and document tests: add transaction-bound create/confirm/settle audit.
- Add `rims-goProgect/internal/migration/runner.go` and `cmd/migrate/main.go`: explicit SQL migration runner.
- Add `rims-goProgect/internal/maintenance/cleanup.go` and `cmd/cleanup/main.go`: explicit cleanup command.
- Modify `rims-goProgect/internal/config/config.go`: migration and cleanup settings.
- Modify `.gitignore` or `rims-goProgect/.gitignore`: ignore generated uploads and Python cache without ignoring `.env`.
- Modify docs/README references: document migration runner, cleanup command, and Docker init limitations.

---

## Task 1: Block Unbound Public Product Images

**Files:**
- Modify: `rims-goProgect/internal/modules/file/service.go`
- Modify: `rims-goProgect/internal/modules/file/service_acl_test.go`

- [ ] **Step 1: Write the failing service test**

Add a test named `TestFileServiceUploadProductImageWithoutBusinessIDRejectsBeforeStorageAndCreate` near the existing upload ACL tests. It must:
- call `Upload` with `BusinessTypeProductImage`, `BusinessID: nil`, non-admin actor, valid filename and content
- expect `types.ErrCodeValidation`
- assert storage save count is zero
- assert repository create count is zero
- assert access checker call count is zero

Run:

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/modules/file -run TestFileServiceUploadProductImageWithoutBusinessIDRejectsBeforeStorageAndCreate -count=1"
```

Expected: FAIL because current service accepts unbound `product_image` uploads.

- [ ] **Step 2: Implement the service invariant**

In `FileService.Upload`, immediately after business type validation and before file reading/storage, add:

```go
if businessType == BusinessTypeProductImage && req.BusinessID == nil {
	return nil, types.ErrValidation("product_image必须关联业务对象")
}
```

- [ ] **Step 3: Add bound product image regression**

Add `TestFileServiceUploadProductImageWithBusinessIDAuthorizesAndReturnsPublicURL`. It must verify a bound `product_image` still calls `FileActionCreate`, saves the object, creates the record, sets `IsPublic=true`, and returns a `/uploads/` URL.

Run:

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/modules/file -count=1"
```

Expected: PASS.

---

## Task 2: Add Route-Level Permission Middleware

**Files:**
- Create: `rims-goProgect/internal/middleware/permission.go`
- Create/modify: `rims-goProgect/internal/middleware/permission_test.go`
- Modify: `rims-goProgect/internal/modules/user/repository.go`
- Modify: `rims-goProgect/internal/app/router.go`
- Modify: `rims-goProgect/internal/modules/user/routes.go`
- Modify: `rims-goProgect/internal/modules/warehouse/routes.go`
- Modify: `rims-goProgect/internal/modules/product/routes.go`
- Modify: `rims-goProgect/internal/modules/audit/routes.go`
- Create: `rims-goProgect/migrations/000010_permission_seed.sql`

- [ ] **Step 1: Write failing middleware tests**

Add tests:
- `TestPermissionAllowsAdminWithoutChecker`
- `TestPermissionAllowsRoleWithPermission`
- `TestPermissionDeniesRoleWithoutPermission`
- `TestPermissionRejectsMissingRoleID`
- `TestPermissionReturnsSystemErrorOnCheckerFailure`

Run:

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/middleware -run Permission -count=1"
```

Expected: FAIL because `middleware.Permission` does not exist.

- [ ] **Step 2: Implement middleware and repository lookup**

Implement:

```go
type PermissionChecker interface {
	HasPermission(ctx context.Context, roleID uint, code string) (bool, error)
}

func Permission(checker PermissionChecker, code string) gin.HandlerFunc
```

Rules:
- admin role code bypasses DB and allows
- missing role ID denies with `types.ErrForbidden()`
- checker errors return `types.ErrSystem(err)`
- missing permission denies with `types.ErrForbidden()`

Extend `RoleRepository` with:

```go
HasPermission(ctx context.Context, roleID uint, code string) (bool, error)
```

Use `role_permissions` joined to `permissions` by permission ID.

- [ ] **Step 3: Seed permissions**

Create `000010_permission_seed.sql` with idempotent inserts for codes used in this task, including:
- `user:create`, `user:update`, `user:delete`, `user:reset_password`
- `role:create`, `role:update`, `role:delete`, `role:assign_permissions`
- `warehouse:create`, `warehouse:update`, `warehouse:delete`, `warehouse:bind_user`, `warehouse:list_users`
- `product:create`, `product:update`, `product:delete`
- `inventory:update`
- `non_std:create`, `non_std:update`, `non_std:delete`, `non_std:convert`, `non_std:read`
- `audit:read`

Grant all permissions to the `admin` role idempotently.

- [ ] **Step 4: Wire first-batch static routes**

In `router.go`, build helpers from `roleRepo` and pass permission middleware into route registration.

Apply permission middleware to fixed admin-only routes only. Keep document type rules and file ACL unchanged.

Route group expectations:
- user/role writes require their matching permission
- warehouse CRUD/bind/unbind/list users require warehouse permissions
- product create/update/delete require product permissions
- inventory update requires `inventory:update` and still uses warehouse scope
- non-std routes require non-std permissions and still use warehouse scope
- audit list/get require `audit:read`

Remove duplicate handler `types.IsAdmin(c)` checks only where the route middleware now provides the decision. Keep `types.IsAdmin(c)` for field masking, document body-dependent permissions, and other dynamic rules.

- [ ] **Step 5: Run permission-focused tests**

Run:

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/middleware ./internal/modules/user ./internal/modules/warehouse ./internal/modules/product ./internal/modules/audit -count=1"
```

Expected: PASS.

---

## Task 3: Fill High-Value Audit Gaps

**Files:**
- Modify: `rims-goProgect/internal/modules/product/handler.go`
- Modify: `rims-goProgect/internal/modules/product/handler_audit_test.go`
- Modify: `rims-goProgect/internal/modules/warehouse/handler.go`
- Modify: `rims-goProgect/internal/modules/warehouse/handler_audit_test.go`
- Modify: `rims-goProgect/internal/modules/file/handler.go`
- Modify: `rims-goProgect/internal/modules/file/handler_audit_test.go`
- Modify: `rims-goProgect/internal/modules/user/handler.go`
- Modify: `rims-goProgect/internal/modules/user/handler_audit_test.go`
- Modify: `rims-goProgect/internal/modules/document/service.go`
- Modify: `rims-goProgect/internal/modules/document/service_concurrency_test.go`

- [ ] **Step 1: Write failing audit tests**

Add tests:
- `TestProductHandlerAuditsCreateAndDelete`
- `TestProductHandlerAuditsNonStdCreateUpdateAndDelete`
- `TestWarehouseHandlerAuditsCRUD`
- `TestFileHandlerAuditsUpload`
- `TestUserHandlerAuditsPasswordChangeAndReset`
- `TestDocumentServiceAuditsCreateInsideTransaction`
- `TestDocumentServiceAuditsConfirmStocktake`
- `TestDocumentServiceAuditsSettleStocktakeInsideTransaction`

Run:

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/modules/product ./internal/modules/warehouse ./internal/modules/file ./internal/modules/user ./internal/modules/document -run 'Audits|Audit' -count=1"
```

Expected: FAIL for the newly added cases.

- [ ] **Step 2: Add handler-level success audit**

Add best-effort success audit for:
- product create/delete and non-std create/update/delete
- warehouse create/update/delete
- file upload
- password change/reset

Do not log password values. Details should include IDs, type codes, filenames, warehouse IDs, and status changes only.

- [ ] **Step 3: Add document transaction-bound audit**

In `DocumentService.Create`, `ConfirmStocktake`, and `SettleStocktake`, write audit entries through the existing injected audit logger inside the same transaction where the status or inventory changes occur.

Use:
- `audit.ActionCreate`, `audit.ResourceDocument`
- `audit.ActionConfirm`, `audit.ResourceDocument`
- `audit.ActionSettle`, `audit.ResourceDocument`

Include `docNo`, `warehouseID`, `docType`, and before/after status snapshots.

- [ ] **Step 4: Run audit package tests**

Run:

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/modules/product ./internal/modules/warehouse ./internal/modules/file ./internal/modules/user ./internal/modules/document -count=1"
```

Expected: PASS.

---

## Task 4: Add Explicit Migration and Cleanup Commands

**Files:**
- Create: `rims-goProgect/internal/migration/runner.go`
- Create: `rims-goProgect/internal/migration/runner_test.go`
- Create: `rims-goProgect/cmd/migrate/main.go`
- Create: `rims-goProgect/internal/maintenance/cleanup.go`
- Create: `rims-goProgect/internal/maintenance/cleanup_test.go`
- Create: `rims-goProgect/cmd/cleanup/main.go`
- Modify: `rims-goProgect/internal/config/config.go`
- Modify: `.gitignore` and/or `rims-goProgect/.gitignore`
- Modify: project README/docs that describe migrations and maintenance

- [ ] **Step 1: Write failing migration runner tests**

Add tests that use `sqlmock` or a lightweight fake executor to verify:
- migration files are sorted by filename
- `schema_migrations` is created
- already-applied matching checksum files are skipped
- checksum mismatch returns an error

Run:

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/migration -count=1"
```

Expected: FAIL because the package does not exist.

- [ ] **Step 2: Implement `cmd/migrate up`**

Implement a migration runner that:
- loads config with `config.Load`
- connects through existing `db.New`
- reads `MIGRATIONS_DIR`, default `./migrations`
- creates `schema_migrations(version text primary key, checksum text not null, applied_at timestamptz not null default now())`
- executes each pending `.sql` file in filename order
- stores SHA-256 checksum
- rejects changed checksums for already-applied versions

Do not run this automatically from app startup.

- [ ] **Step 3: Write failing cleanup tests**

Add tests for cleanup SQL construction and retention rules:
- expired idempotency keys are hard-deleted
- soft-deleted file objects older than retention are selected for object cleanup
- audit cleanup is disabled when `AUDIT_LOG_RETENTION_DAYS=0`

Run:

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/maintenance -count=1"
```

Expected: FAIL because the package does not exist.

- [ ] **Step 4: Implement explicit cleanup command**

Implement `cmd/cleanup` as a one-shot command. Defaults:
- `IDEMPOTENCY_KEY_TTL_HOURS=24`
- `FILE_DELETED_RETENTION_DAYS=30`
- `AUDIT_LOG_RETENTION_DAYS=0`
- `CLEANUP_BATCH_SIZE=1000`

Keep audit deletion disabled by default. File cleanup should remove storage objects for old soft-deleted file rows before hard-deleting metadata, or retain metadata if storage deletion fails.

- [ ] **Step 5: Ignore generated local artifacts without hiding `.env`**

Ensure ignore rules cover:
- `rims-goProgect/uploads/`
- `**/__pycache__/`
- `*.pyc`

Ensure no new `.env`, `.env.*`, or `*.env` ignore rule is added. If an existing project ignore file already hides `.env`, remove that line.

- [ ] **Step 6: Run maintenance verification**

Run:

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/migration ./internal/maintenance ./cmd/migrate ./cmd/cleanup -count=1"
```

Expected: PASS.

---

## Final Verification

- [ ] Run WSL full tests:

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./... -count=1"
```

- [ ] Run WSL build:

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go build ./..."
```

- [ ] Run git checks:

```powershell
git status --short
git diff --check
```

- [ ] Review all changed files for SPDX headers where source files were created.

