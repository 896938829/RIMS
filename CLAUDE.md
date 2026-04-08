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

All commands (Go, Docker, tests) must run inside **WSL** (Ubuntu), not Windows. The project directory in WSL is `/mnt/e/My Work/RIMS`. Use `wsl -- bash -c "..."` when executing from a Windows shell. PostgreSQL runs as a Docker container in WSL, and Go services connect via `127.0.0.1:5432` — this only works when Go also runs inside WSL.

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
cd rims-goProgect && go test ./internal/modules/todo/...

# Regenerate Swagger docs (after changing API annotations)
cd rims-goProgect && swag init -g cmd/server/main.go -o docs
```

## Architecture

**Go module**: `rims-go` (Go 1.25, in `rims-goProgect/`)

**Stack**: Gin (HTTP router) + GORM (ORM) + PostgreSQL + JWT auth + Swagger (swaggo)

**Configuration**: Viper reads from `.env` file (workspace root or `rims-goProgect/`) and environment variables. See `.env.example` for all variables. `DB_PASSWORD` and `JWT_SECRET` are required.

**Entry flow**: `cmd/server/main.go` → `internal/app.Run()` which loads config, connects DB, runs auto-migration if `DB_AUTO_MIGRATE=true`, builds the Gin router, and starts the HTTP server.

**Module pattern**: Each domain module lives in `internal/modules/<name>/` with: `model.go` (GORM model), `repository.go` (data access interface + GORM impl), `service.go` (business logic), `handler.go` (Gin HTTP handlers with Swagger annotations). Currently: `todo` (demo CRUD) and `authapi` (demo login).

**Key packages**:
- `internal/auth` — JWT token generation/parsing (HS256, `TokenService`)
- `internal/middleware` — Gin middleware (JWT auth guard, sets `userID`/`username` in context)
- `internal/config` — Viper-based config struct loading from env vars
- `internal/db` — GORM PostgreSQL connection factory

**API routes** (all under `/api/v1`):
- `POST /auth/login` — public, returns JWT
- `/todos` CRUD — protected by JWT middleware

**Swagger UI**: available at `/swagger/index.html` when the server is running.

**SQL migrations**: `rims-goProgect/migrations/` contains raw SQL run by Docker entrypoint. GORM AutoMigrate handles the `todos` table at app startup.

**Docker**: `deploy/docker-compose.yml` runs PostgreSQL 16, reads env vars from workspace root `.env`.
