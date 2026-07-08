# RIMS M8 Backend Integration Defect Summary

> Scope: This document records backend defects found during the M8 frontend-backend integration pass on 2026-07-07 and 2026-07-08. It is intended as the backend-side handoff record before the next backend target loop starts.

## Executive Summary

- Integration status: Phase A-F backend blockers are repaired and verified.
- Open P0/P1 backend defects: 0.
- Repaired backend defects: 4.
- Verification baseline: backend targeted tests pass, backend full test/build pass, and frontend smoke passes against the repaired backend.
- Frontend source plan: `E:\My Work\rims-frontend\docs\superpowers\plans\2026-07-07-rims-frontend-backend-integration.md`
- Frontend execution record: `E:\My Work\rims-frontend\docs\superpowers\plans\2026-07-07-rims-m8-integration-session-1.md`

## Verification Baseline

Run from `E:\My Work\RIMS\rims-goProgect`:

```powershell
wsl -e bash -lc 'cd "/mnt/e/My Work/RIMS/rims-goProgect" && ~/local/go/bin/go test ./internal/modules/user ./internal/modules/warehouse ./internal/modules/product'
wsl -e bash -lc 'cd "/mnt/e/My Work/RIMS/rims-goProgect" && ~/local/go/bin/go test ./...'
wsl -e bash -lc 'cd "/mnt/e/My Work/RIMS/rims-goProgect" && ~/local/go/bin/go build ./...'
```

Expected result:

- Targeted module tests pass.
- Full backend test suite passes.
- Full backend build passes.

Run from `E:\My Work\rims-frontend` while the repaired backend is running:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\rims_smoke.ps1
```

Expected result:

- Frontend analyze passes.
- Frontend test suite passes.
- API smoke passes against `http://localhost:8080`.

## Defect Ledger

| ID | Phase | Priority | Status | Defect |
| --- | --- | --- | --- | --- |
| M8-B-001 | Phase B | P1 | Fixed and verified | Current warehouse selection was not restored after refresh/login |
| M8-B-002 | Phase B/F | P1 | Fixed and verified | Normal users could list roles and permissions |
| M8-C-001 | Phase C | P1 | Fixed and verified | Inventory keyword search returned filtered total with unfiltered rows |
| M8-F-001 | Phase F | P1 | Fixed and verified | Warehouse deletion was allowed while active user bindings existed |

## M8-B-001: Current Warehouse Restore Contract Missing

**Observed behavior**

- `PUT /api/v1/users/me/warehouses/current` could return the selected warehouse.
- A later `GET /api/v1/users/me` and `GET /api/v1/users/me/warehouses` did not expose a reliable current-warehouse marker.
- `GET /api/v1/users/me/warehouses` still marked the old warehouse as `isDefault=true`, so frontend session restore could revert the user to the previous warehouse after refresh.

**Root cause**

- The switch endpoint changed the runtime selection response but did not persist the selection into the user-warehouse binding state returned by session restoration APIs.

**Backend repair**

- `internal/modules/warehouse/service.go`
  - `SwitchCurrentWarehouse` now clears previous default bindings for the user and marks the selected warehouse as the default binding.
- `internal/modules/warehouse/routes_permission_test.go`
  - Added `TestSwitchCurrentWarehousePersistsDefaultWarehouse`.

**Verification**

- Switching to warehouse 2 makes `GET /api/v1/users/me/warehouses` return warehouse 2 with `isDefault: true` and warehouse 1 with `isDefault: false`.
- Switching back to warehouse 1 restores warehouse 1 with `isDefault: true`.
- Targeted warehouse tests pass.

**Optimization item**

- Decide the public contract explicitly: either document that `isDefault` is also the current warehouse marker, or add explicit fields such as `isCurrent` and `currentWarehouseId` to avoid overloading default-vs-current semantics.

## M8-B-002: Role And Permission Read Authorization Leak

**Observed behavior**

- A normal operator received `403` from `GET /api/v1/users`, which was correct.
- The same operator received `200` from `GET /api/v1/roles` and `GET /api/v1/permissions`, which exposed role and permission metadata.

**Root cause**

- `internal/modules/user/routes.go` protected role and permission list/detail routes with authentication only. The routes did not require role or permission management permissions.

**Backend repair**

- `internal/modules/user/routes.go`
  - `GET /api/v1/roles` now requires `role:list`.
  - `GET /api/v1/roles/:id` now requires `role:read`.
  - `GET /api/v1/permissions` now requires `permission:list`.
- `migrations/000013_role_read_permission_seed.sql`
  - Seeds `role:list`, `role:read`, and `permission:list`.
  - Grants those permissions to the admin role.
- `internal/modules/user/handler_role_auth_test.go`
  - Added `TestRoleAndPermissionReadRoutesRequirePermission`.

**Verification**

- Normal operator:
  - `GET /api/v1/roles` returns `403`.
  - `GET /api/v1/permissions` returns `403`.
- Admin:
  - `GET /api/v1/roles` returns `200`.
  - `GET /api/v1/permissions` returns `200`.
- Targeted user module tests pass.

**Optimization item**

- Audit every authenticated management route and create a route-permission matrix. The matrix should make auth-only routes intentional rather than accidental.

## M8-C-001: Inventory Keyword Search Total/List Divergence

**Observed behavior**

- `GET /api/v1/inventory?keyword=sale_1776407432915&page=1&pageSize=20` returned `total: 1`.
- The same response contained many unrelated rows in `data.list`.

**Root cause**

- `internal/modules/product/repository.go` applied keyword joins and filters to the query used by `Count`.
- The later `Find` query was rebuilt from a fresh base query and only retained `warehouse_id`, so the response rows were not filtered by keyword.

**Backend repair**

- `internal/modules/product/repository.go`
  - `inventoryRepo.ListByWarehouse` now reuses the filtered query for both count and row loading.
- `internal/modules/product/repository_test.go`
  - Added `TestInventoryRepositoryListByWarehouseAppliesKeywordToRowsAndTotal`.

**Verification**

- The same keyword request returns `total=1`.
- `data.list.length=1`.
- The only returned product code is `sale_1776407432915`.
- Targeted product module tests pass.

**Optimization item**

- Standardize list-query construction so `Count` and paged `Find` cannot diverge across inventory, products, warehouses, non-standard inventory, transactions, and reports.

## M8-F-001: Warehouse Deletion Allowed With Active User Bindings

**Observed behavior**

- A warehouse could be created, bound to a user, and then deleted.
- The bound user could still log in, but `GET /api/v1/users/me/warehouses` returned an empty warehouse list.

**Root cause**

- `WarehouseService.Delete` soft-deleted the warehouse and deleted all user bindings for that warehouse without first rejecting active bindings.

**Backend repair**

- `internal/modules/warehouse/service.go`
  - `Delete` now checks active user bindings and returns `ErrInvalidState("仓库已绑定用户，无法删除")` when bindings exist.
- `internal/modules/warehouse/routes_permission_test.go`
  - Added `TestDeleteWarehouseRejectsActiveUserBindings`.

**Verification**

- Deleting a warehouse with active bindings returns `422`.
- The response message is `仓库已绑定用户，无法删除`.
- After unbinding users, deleting the same warehouse returns `204`.
- Targeted warehouse tests pass.

**Optimization item**

- Move deletion-conflict checks into clear repository helpers such as `CountActiveBindingsByWarehouseID`, then apply the same explicit conflict policy to products, users, inventory-affecting records, and configuration entities.

## Backend Files Changed During Repair

- `internal/modules/product/repository.go`
- `internal/modules/product/repository_test.go`
- `internal/modules/user/handler_role_auth_test.go`
- `internal/modules/user/routes.go`
- `internal/modules/warehouse/routes_permission_test.go`
- `internal/modules/warehouse/service.go`
- `migrations/000013_role_read_permission_seed.sql`

## Handoff Recommendation

Start the backend target loop from the companion plan:

```text
docs/superpowers/plans/2026-07-08-rims-backend-repair-optimization-plan.md
```

Use this defect summary as the factual baseline. The companion plan should improve the contracts and guardrails around these repaired defects rather than reopen the already verified fixes.
