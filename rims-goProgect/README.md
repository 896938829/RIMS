# rims-go

`rims-go` is the Go backend service for the `rims` workspace.

## Project Layout

- `cmd/server`: service entrypoint
- `internal/app`: application bootstrap logic
- `internal/config`: environment configuration loading
- `configs`: example configuration templates
- `migrations`: SQL schema migrations
- `scripts`: local development helper scripts
- `tests/integration`: integration test placeholder

## Quick Start

1. Copy workspace root `.env.example` to workspace root `.env` and fill values.
2. Start dependencies (such as PostgreSQL) via Docker Compose from workspace root:

```bash
docker compose --project-directory . -f deploy/docker-compose.yml up -d
```
3. Run:

```bash
go run ./cmd/server
```

4. Open Swagger UI:

```text
http://127.0.0.1:8080/swagger/index.html
```

## Generate Swagger Docs

Install tool:

```bash
go install github.com/swaggo/swag/cmd/swag@latest
```

Generate docs:

```bash
swag init -g cmd/server/main.go -o docs
```

## Demo Auth + Protected Todo API

- Login endpoint: `POST /api/v1/auth/login` with:

```json
{"username":"admin","password":"admin123"}
```

- Use response token in header:

```text
Authorization: Bearer <token>
```

- Protected todo endpoints:
  - `POST /api/v1/todos`
  - `GET /api/v1/todos`
  - `GET /api/v1/todos/{id}`
  - `DELETE /api/v1/todos/{id}`

## Environment Variables

See `.env.example` for all supported variables.
