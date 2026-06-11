# RIMS Security Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the verified authorization, soft-delete uniqueness, validation, and robustness findings from the Claude Review pass.

**Architecture:** Keep authorization decisions close to existing module boundaries: routes enforce coarse RBAC with `middleware.Permission`, services enforce business invariants such as self-lock and last-admin protection, and cross-module warehouse access is expressed as a narrow consumer-owned interface. SQL migrations and GORM tags should agree so fresh and upgraded databases both behave consistently.

**Tech Stack:** Go 1.25, Gin, GORM, PostgreSQL migrations, WSL Ubuntu 22.04 commands from `/mnt/e/My Work/RIMS/rims-goProgect`.

---

## Fix Inventory

1. High risk: user role escalation via `UpdateUser`.
2. High risk: self-disable and missing last active admin guard.
3. High risk: `GET /users` and `GET /users/:id` lack route permission.
4. High risk: `GET /warehouses/:id` lacks route permission or access check.
5. High risk: transfer completion validates source warehouse only, not `ToWarehouseID`.
6. Medium risk: wildcard CORS reflects arbitrary origin.
7. Medium risk: soft-delete models use naked unique constraints on role code, permission code, username.
8. Medium risk: document line quantity DTO allows `0` even though service rejects it.
9. Low risk: duplicate `CountByUserID` call in warehouse binding.
10. Low risk: `rand.Read` error ignored in request ID generation.
11. Low risk: idempotency duplicate-create fallback hides second lookup failure.
12. Low risk: `internal/db/db.go` missing SPDX header.

## Task A: User RBAC And Admin Safety

**Files:**
- Modify: `rims-goProgect/internal/modules/user/routes.go`
- Modify: `rims-goProgect/internal/modules/user/handler.go`
- Modify: `rims-goProgect/internal/modules/user/service.go`
- Modify: `rims-goProgect/internal/modules/user/repository.go`
- Modify/Test: `rims-goProgect/internal/modules/user/service_test.go`
- Modify/Test: `rims-goProgect/internal/modules/user/handler_role_auth_test.go`

- [ ] Step 1: Add failing route tests showing `GET /api/v1/users` requires `user:list` and `GET /api/v1/users/:id` requires `user:read`.

Run:

```bash
cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/modules/user -run 'TestUserReadRoutesRequirePermission|TestUserReadRoutesAllowRoleWithPermission' -count=1
```

Expected before implementation: tests fail because the routes enter handlers without checking the permission code.

- [ ] Step 2: Add failing service tests for:
  - non-admin actor cannot change `RoleID`;
  - actor cannot set their own `Status` to `0`;
  - disabling or demoting the only active admin returns forbidden/invalid-state and does not call `Update`;
  - deleting the only active admin is rejected.

Run:

```bash
cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/modules/user -run 'TestUserService(Update|Delete).*Admin|TestUserServiceUpdateRejects' -count=1
```

Expected before implementation: tests fail because `Update` has no actor context and no last-admin guard.

- [ ] Step 3: Implement minimal route permission changes:

```go
users.GET("", perm("user:list"), handler.ListUsers)
users.GET("/:id", perm("user:read"), handler.GetUser)
```

- [ ] Step 4: Add `user:list` and `user:read` permissions to the permission seed migration.

- [ ] Step 5: Extend `UserRepository` with an active-admin counter, implemented via a users/roles join with `status = 1` and `deleted_at IS NULL`.

- [ ] Step 6: Change `UserService.Update` and `UserService.Delete` to receive actor user/role context from the handler and enforce:
  - only admins may change `RoleID`;
  - self-disable (`actorUserID == id && Status == 0`) is forbidden;
  - if the target is an active admin, disabling, demoting away from admin, or deleting is forbidden when active admin count is `<= 1`.

- [ ] Step 7: Run package tests.

```bash
cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/modules/user -count=1
```

## Task B: Warehouse Detail And Transfer Target Authorization

**Files:**
- Modify: `rims-goProgect/internal/modules/warehouse/routes.go`
- Modify: `rims-goProgect/internal/modules/warehouse/handler.go`
- Modify: `rims-goProgect/internal/modules/warehouse/service.go`
- Modify/Test: `rims-goProgect/internal/modules/warehouse/routes_permission_test.go`
- Modify: `rims-goProgect/internal/modules/document/service.go`
- Modify: `rims-goProgect/internal/app/router.go`
- Modify/Test: `rims-goProgect/internal/modules/document/service_concurrency_test.go`

- [ ] Step 1: Add failing route tests showing `GET /api/v1/warehouses/:id` requires `warehouse:read`.

Run:

```bash
cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/modules/warehouse -run TestWarehouseReadRouteRequiresPermission -count=1
```

- [ ] Step 2: Add failing document service test showing transfer completion by a scoped admin returns forbidden when the actor lacks access to `doc.ToWarehouseID`.

Run:

```bash
cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/modules/document -run TestCompleteTransferRejectsUnauthorizedTargetWarehouse -count=1
```

- [ ] Step 3: Implement route permission:

```go
warehouses.GET("/:id", perm("warehouse:read"), handler.GetWarehouse)
```

- [ ] Step 4: Add `warehouse:read` to the permission seed migration.

- [ ] Step 5: Add a narrow `WarehouseAccessChecker` interface in `document/service.go`:

```go
type WarehouseAccessChecker interface {
    HasAccess(ctx context.Context, userID, warehouseID uint) (bool, error)
}
```

Wire `userWarehouseRepo` into `NewDocumentService` from `internal/app/router.go`.

- [ ] Step 6: In transfer completion, before inventory writes, call the checker for `doc.ToWarehouseID`. Return `types.ErrForbidden()` when access is false and `types.ErrSystem(err)` when lookup fails.

- [ ] Step 7: Run package tests.

```bash
cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/modules/warehouse ./internal/modules/document ./internal/app -count=1
```

- [ ] Step 8: Reuse the existing `CountByUserID` result inside `BindUsers` so non-admin users are counted once per binding attempt.

## Task C: CORS, Soft-Delete Uniqueness, DTO Validation

**Files:**
- Modify/Test: `rims-goProgect/internal/middleware/cors.go`
- Modify/Test: `rims-goProgect/internal/middleware/*cors*_test.go` or create `cors_test.go`
- Modify: `rims-goProgect/internal/modules/user/model.go`
- Create: `rims-goProgect/migrations/000011_soft_delete_unique_indexes.sql`
- Create: `rims-goProgect/migrations/000012_read_permission_seed.sql`
- Modify: `rims-goProgect/internal/modules/document/dto.go`

- [ ] Step 1: Add failing CORS tests:
  - `CORS("*")` returns `Access-Control-Allow-Origin: *`;
  - a strict allowlist returns the matched origin;
  - a non-matching origin gets no allow-origin header.

Run:

```bash
cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/middleware -run TestCORS -count=1
```

- [ ] Step 2: Implement wildcard behavior so `*` is literal `*`, while explicit origins still echo the matched origin.

- [ ] Step 3: Change GORM tags for `users.username`, `roles.code`, and `permissions.code` to named partial unique indexes with `where:deleted_at IS NULL`.

- [ ] Step 4: Leave previously versioned migrations unchanged to preserve migration checksums. Create `000011_soft_delete_unique_indexes.sql` to migrate existing databases by dropping the old unique constraints/indexes if present and creating partial unique indexes:

```sql
CREATE UNIQUE INDEX IF NOT EXISTS idx_roles_code_active ON roles(code) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_permissions_code_active ON permissions(code) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username_active ON users(username) WHERE deleted_at IS NULL;
```

- [ ] Step 5: Create `000012_read_permission_seed.sql` to seed `user:list`, `user:read`, and `warehouse:read` after the partial permission-code index exists. Use `ON CONFLICT (code) WHERE deleted_at IS NULL DO UPDATE`, and grant the new permissions to the admin role.

- [ ] Step 6: Keep `CreateDocumentLineRequest.Quantity` at `binding:"min=0"` because stocktake lines can legitimately carry `quantity: 0`; rely on doc-type-aware service validation for non-stocktake/non-conversion quantities.

- [ ] Step 7: Run package tests.

```bash
cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/middleware ./internal/modules/user ./internal/modules/document ./internal/migration -count=1
```

## Task D: Low-Risk Robustness

**Files:**
- Modify/Test: `rims-goProgect/internal/middleware/requestid.go`
- Modify/Test: `rims-goProgect/internal/idempotency/service.go`
- Modify/Test: `rims-goProgect/internal/idempotency/service_test.go`
- Modify: `rims-goProgect/internal/db/db.go`

- [ ] Step 1: Update request ID generation to use `io.ReadFull(rand.Reader, b)` and a deterministic fallback such as timestamp plus atomic counter if crypto randomness fails.

- [ ] Step 2: Add idempotency service test proving the duplicate-create fallback returns an error that preserves both create failure and second lookup failure.

- [ ] Step 3: Implement that error with `fmt.Errorf("create idempotency record: %w; load existing record after create conflict: %v", createErr, err)` or equivalent.

- [ ] Step 4: Add SPDX header to `internal/db/db.go`.

- [ ] Step 5: Run focused tests.

```bash
cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/middleware ./internal/idempotency ./internal/db -count=1
```

## Final Verification

Run:

```bash
cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./...
cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go build ./...
```

No git commit is part of this plan unless the user explicitly asks for one.
