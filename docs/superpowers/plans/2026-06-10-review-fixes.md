# Review Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the confirmed inventory, authentication, startup, and data-boundary issues found in the Claude Review pass.

**Architecture:** Keep changes inside existing module boundaries. Add narrowly scoped regression tests before production changes, use existing `types.AppError` constructors, and preserve repository/service abstractions.

**Tech Stack:** Go 1.25, Gin, GORM, PostgreSQL-oriented behavior, existing repository interfaces and WSL-based Go commands.

---

### Task 1: Stocktake Settlement Integrity

**Files:**
- Modify: `rims-goProgect/internal/modules/document/service.go`
- Test: `rims-goProgect/internal/modules/document/service_concurrency_test.go`

- [x] Add failing tests for stocktake settlement rejecting stale `SystemQty`.
- [x] Add failing tests for rejecting negative post-settlement inventory instead of truncating.
- [x] In `SettleStocktake`, after locking current inventory, reject settlement when `beforeQty != line.SystemQty`.
- [x] Reject `beforeQty + line.DiffQty < 0` with `types.ErrInvalidState`.
- [x] Verify `go test ./internal/modules/document/...`.

### Task 2: Auth, Permission, And User Race Safety

**Files:**
- Modify: `rims-goProgect/internal/middleware/jwt.go`
- Modify: `rims-goProgect/internal/app/router.go`
- Modify: `rims-goProgect/internal/modules/user/service.go`
- Modify: `rims-goProgect/internal/modules/user/repository.go`
- Test: `rims-goProgect/internal/middleware/*_test.go`
- Test: `rims-goProgect/internal/modules/user/*_test.go`

- [x] Add failing middleware tests proving disabled/deleted users are rejected after JWT parsing.
- [x] Add a narrow user status provider interface to JWT middleware and wire `userRepo` in `buildRouter`.
- [x] Refresh role identity from DB when accepting a request, so stale token role data does not authorize deleted/changed roles.
- [x] Add failing service test for unique-constraint duplicate username errors returning `types.ErrDuplicate`.
- [x] Convert database unique constraint failures on create to `ErrDuplicate`.
- [x] Add repository test or SQL assertion proving `HasPermission` ignores soft-deleted roles.
- [x] Verify `go test ./internal/middleware/... ./internal/modules/user/...`.

### Task 3: Startup And Boundary Hardening

**Files:**
- Modify: `rims-goProgect/internal/app/app.go`
- Modify: `rims-goProgect/internal/modules/audit/dto.go`
- Modify: `rims-goProgect/internal/modules/product/service.go`
- Test: `rims-goProgect/internal/app/*_test.go`
- Test: `rims-goProgect/internal/modules/audit/*_test.go`

- [x] Add failing audit test showing truncation preserves valid UTF-8 and rune boundaries.
- [x] Implement UTF-8-safe truncation by rune count.
- [x] Extract server start/shutdown behavior into a testable helper and add graceful SIGINT/SIGTERM shutdown.
- [x] Add a concise comment in `ConvertNonStd` explaining advisory lock plus `FOR UPDATE` intent.
- [x] Verify `go test ./internal/app/... ./internal/modules/audit/... ./internal/modules/product/...`.

### Task 4: Integration Verification

**Files:**
- Review all modified files.

- [x] Run `go test ./...`.
- [x] Run `go build ./...`.
- [x] Confirm no unrelated user changes were reverted.
- [x] Report items intentionally not changed: warehouse default ordering, `ListByIDs` soft-delete filtering, idempotency writer full optional interfaces, AutoMigrate raw indexes.
