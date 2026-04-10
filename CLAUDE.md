# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

RIMS (Retail Inventory Management System) — a retail inventory management system. The backend is a Go service located in `rims-goProgect/` (note the typo in the directory name — it's intentional). Product requirement docs are in `docs/` (Chinese language).

Licensed under AGPL-3.0. All source files must include the SPDX header:
```
// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang
```

## Runtime Environment

All commands (Go, Docker, tests) must run inside **WSL** (Ubuntu 22.04), not Windows. The project directory in WSL is `/mnt/e/My Work/RIMS`. Go is installed at `~/local/go/bin/go` in WSL. PostgreSQL runs as a Docker container in WSL, and Go services connect via `127.0.0.1:5432` — this only works when Go also runs inside WSL.

When executing from a Windows shell, use `wsl -e bash -c "..."` (not `wsl -- bash -c`) to avoid PATH expansion issues with Windows paths containing parentheses (e.g. `Program Files (x86)`).

## Common Commands

All Go commands run inside WSL, from `rims-goProgect/`:

```bash
# Start dependencies (PostgreSQL)
docker compose --project-directory . -f deploy/docker-compose.yml up -d

# Run the server (reads .env from workspace root or rims-goProgect/)
cd rims-goProgect && go run ./cmd/server

# Run all tests
cd rims-goProgect && go test ./...

# Run a single package's tests
cd rims-goProgect && go test ./internal/modules/user/...

# Build check
cd rims-goProgect && go build ./...

# Regenerate Swagger docs (after changing API annotations)
cd rims-goProgect && swag init -g cmd/server/main.go -o docs
```

## Architecture

**Go module**: `rims-go` (Go 1.25, in `rims-goProgect/`)

**Stack**: Gin (HTTP) + GORM (ORM) + PostgreSQL + JWT (golang-jwt/v5) + Swagger (swaggo) + Viper (config)

**Configuration**: Viper reads from `.env` file (workspace root or `rims-goProgect/`) and environment variables. `DB_PASSWORD` and `JWT_SECRET` are required. The `.env` file is committed to the repo (early stage, dev-only credentials); do not add it to `.gitignore`.

**Entry flow**: `cmd/server/main.go` → `internal/app.Run()` → loads config, connects DB, runs GORM AutoMigrate if `DB_AUTO_MIGRATE=true`, calls `buildRouter()` in `router.go` to wire all modules, starts HTTP server.

### Module Pattern

Each domain module lives in `internal/modules/<name>/` with a consistent layered structure:

| File | Role |
|---|---|
| `model.go` | GORM entities (embed `types.BaseModel` for system data or `types.AuditableModel` for business data with CreatedBy/UpdatedBy) |
| `dto.go` | Request/response structs with `binding` validation tags; `ToXxxResponse()` converter functions |
| `repository.go` | Data access interface + GORM implementation; all methods take `context.Context` for transaction propagation; use `db.FromCtx(ctx, r.gormDB)` |
| `service.go` | Business logic; depends on repository interfaces (mockable); uses `db.RunInTx` for multi-repo transactions |
| `handler.go` | Thin HTTP handlers with Swagger annotations (Chinese); binds params → calls service → returns via `types.OK`/`FailFromError`; includes local `parseID()` helper |
| `routes.go` | `RegisterRoutes()` function for self-registering routes on a `gin.RouterGroup` |

**Current modules**:
- `user` — auth (login/JWT), user CRUD, role & permission management
- `warehouse` — warehouse CRUD, user-warehouse binding (normal users: 1 warehouse; admins: multiple), default warehouse, warehouse switching, WarehouseScope middleware
- `product` — product catalog (global CRUD, barcode lookup, admin-only cost price), standard inventory (per-warehouse, alert threshold), non-standard inventory (admin-only CRUD, partial/full conversion to standard)

- `document` — business documents (inbound/sales/return/transfer/stocktake/conversion), document lines, inventory transaction log; polymorphic `documents` table with `doc_type` discriminator; `CompleteDocument` executes inventory changes transactionally; stocktake has 3-step flow (recording→confirmed→settled); cross-module: imports `product.InventoryRepository`/`ProductRepository`/`NonStdInventoryRepository` for inventory operations
- `report` — read-only analytics: sales stats, sales trend (day/week/month), product ranking, inventory overview, turnover rate, slow-moving alerts; aggregates over `documents`/`document_lines`/`inventory_transactions`/`inventories`/`products` via raw `.Table(...)` joins (no cross-module model imports); admin-only field gating on cost/profit/stock-value via `*float64 + omitempty`; time-range validation (max 366d); bucket/metric whitelisting to prevent SQL injection; no `model.go` (no new entities)

Planned: `file`, `audit`.

### Shared Infrastructure

- `internal/types/` — Shared types used across all modules:
  - `response.go`: Unified response envelope (`OK`, `Fail`, `FailFromError`, `OKWithPage`, `OKCreated`, `OKNoContent`)
  - `errors.go`: `AppError` type with business error codes (10001–50000) and `HTTPStatus()` mapping; constructors: `ErrAuth`, `ErrForbidden`, `ErrValidation`, `ErrNotFound`, `ErrDuplicate`, `ErrInvalidState`, `ErrSystem`
  - `pagination.go`: `PageRequest` (with `Defaults()`, `Offset()`) / `PageResult` (via `NewPageResult()`) for paginated queries
  - `base_model.go`: `BaseModel` (ID, timestamps, soft delete) and `AuditableModel` (+ CreatedBy/UpdatedBy)
  - `context.go`: Helpers to extract `userID`, `roleCode`, `warehouseID`, `traceID` from gin.Context; `IsAdmin()` check
- `internal/auth/` — JWT `TokenService` (GenerateToken, ParseToken); Claims carry `UserID`, `Username`, `RoleID`, `RoleCode`
- `internal/db/` — GORM connection (`db.go`) + transaction propagation (`tx.go`: `RunInTx`, `FromCtx`)
- `internal/middleware/` — `JWTAuth`, `RequestID`, `Logger`, `CORS`, `WarehouseScope`; planned: `Permission`, `Idempotency`
- `internal/config/` — Viper-based config struct

### Cross-Module Interaction

Modules interact through **interfaces defined by the consumer** (dependency inversion), wired in `internal/app/router.go` (the composition root). To avoid cross-package model imports, use join queries returning flat structs (e.g., `WarehouseUserInfo` in the warehouse repo joins `users` table directly instead of importing the `user` package).

**Transaction propagation**: `db.RunInTx(ctx, gormDB, func(txCtx) error { ... })` stores the `*gorm.DB` transaction in context. All repositories use `db.FromCtx(ctx, r.gormDB)` to automatically participate in the active transaction.

### Middleware Chain

```
Recovery → RequestID → Logger → CORS → [public routes] → JWTAuth → [protected routes]
```

JWT middleware sets `userID`, `username`, `roleID`, `roleCode` in gin.Context. `WarehouseScope` middleware reads `X-Warehouse-ID` header (falls back to user's default warehouse), validates access, and sets `warehouseID` in context — applied to inventory, non-std-inventory, documents, and transactions routes.

**API routes**: All under `/api/v1`. Public: `POST /auth/login`. Protected: `/users`, `/roles`, `/permissions`, `/warehouses` CRUD + user-warehouse binding, `/products` CRUD + barcode lookup, `/inventory` list/alerts/update (warehouse-scoped), `/non-std-inventory` CRUD + convert (warehouse-scoped, admin-only), `/documents` CRUD + complete/confirm/settle (warehouse-scoped), `/transactions` list (warehouse-scoped), `/reports/{sales,inventory}/...` (warehouse-scoped; cost/profit fields admin-only).

**Swagger UI**: available at `/swagger/index.html` when the server is running.

**SQL migrations**: `rims-goProgect/migrations/` contains raw SQL. `000001_init.sql` (users/roles/permissions + seed admin), `000002_warehouse.sql` (warehouses/user_warehouses + seed default warehouse), `000003_product.sql` (products/inventories/non_std_inventories), `000004_document.sql` (documents/document_lines/inventory_transactions), `000005_report_indexes.sql` (compound partial indexes supporting report aggregation queries — warehouse+time+doc_type on inventory_transactions/documents). GORM AutoMigrate also runs at startup for convenience but does NOT create the raw indexes; apply the SQL file manually on new environments.

**Docker**: `deploy/docker-compose.yml` runs PostgreSQL 16, reads env vars from workspace root `.env`.

### Adding a New Module

1. Create `internal/modules/<name>/` with the 6 standard files (model, dto, repository, service, handler, routes)
2. Add SQL migration in `migrations/` (next sequential number)
3. Add models to `AutoMigrate()` in `internal/app/app.go`
4. Wire repos → services → handlers in `internal/app/router.go` and call `<module>.RegisterRoutes()`
5. Admin-only checks go in the handler via `types.IsAdmin(c)`, not in middleware
6. Run `go build ./...` and `go test ./...` to verify
