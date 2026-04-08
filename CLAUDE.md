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
| `model.go` | GORM entities (embed `types.BaseModel` or `types.AuditableModel`) |
| `dto.go` | Request/response structs with `binding` validation tags |
| `repository.go` | Data access interface + GORM implementation; all methods take `context.Context` for transaction propagation |
| `service.go` | Business logic; depends on repository interfaces (mockable) |
| `handler.go` | Thin HTTP handlers with Swagger annotations; binds params → calls service → returns unified response |
| `routes.go` | `RegisterRoutes()` function for self-registering routes on a `gin.RouterGroup` |

**Current modules**: `user` (auth, user CRUD, role & permission management). Planned: `warehouse`, `product`, `document`, `report`, `file`, `audit`.

### Shared Infrastructure

- `internal/types/` — Shared types used across all modules:
  - `response.go`: Unified response envelope (`OK`, `Fail`, `FailFromError`, `OKWithPage`)
  - `errors.go`: `AppError` type with business error codes (10001–50000) and `HTTPStatus()` mapping
  - `pagination.go`: `PageRequest` / `PageResult` for paginated queries
  - `base_model.go`: `BaseModel` (ID, timestamps, soft delete) and `AuditableModel` (+ CreatedBy/UpdatedBy)
  - `context.go`: Helpers to extract `userID`, `roleCode`, `warehouseID`, `traceID` from gin.Context
- `internal/auth/` — JWT `TokenService` (GenerateToken, ParseToken); Claims carry `UserID`, `Username`, `RoleID`, `RoleCode`
- `internal/db/` — GORM connection (`db.go`) + transaction propagation (`tx.go`: `RunInTx`, `FromCtx`)
- `internal/middleware/` — `JWTAuth`, `RequestID`, `Logger`, `CORS`; planned: `Warehouse`, `Permission`, `Idempotency`
- `internal/config/` — Viper-based config struct

### Cross-Module Interaction

Modules interact through **interfaces defined by the consumer** (dependency inversion), wired in `internal/app/router.go` (the composition root). For example, the document module will define an `InventoryOperator` interface that the product module's service implements.

**Transaction propagation**: `db.RunInTx(ctx, gormDB, func(txCtx) error { ... })` stores the `*gorm.DB` transaction in context. All repositories use `db.FromCtx(ctx, r.db)` to automatically participate in the active transaction.

### Middleware Chain

```
Recovery → RequestID → Logger → CORS → [public routes] → JWTAuth → [protected routes]
```

JWT middleware sets `userID`, `username`, `roleID`, `roleCode` in gin.Context. Future middleware: `Warehouse` (warehouse scope validation via `X-Warehouse-ID` header), `Permission` (RBAC check), `Idempotency` (write dedup).

**API routes**: All under `/api/v1`. Public: `POST /auth/login`. Protected: `/users`, `/roles`, `/permissions` CRUD.

**Swagger UI**: available at `/swagger/index.html` when the server is running.

**SQL migrations**: `rims-goProgect/migrations/` contains raw SQL. `000001_init.sql` creates users/roles/permissions tables and seeds default admin. GORM AutoMigrate also runs at startup for convenience.

**Docker**: `deploy/docker-compose.yml` runs PostgreSQL 16, reads env vars from workspace root `.env`.
