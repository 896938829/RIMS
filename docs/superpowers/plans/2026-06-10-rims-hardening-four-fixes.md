# RIMS Hardening Four Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix four remaining high-priority hardening items in order: database integrity constraints, idempotency, file business ACL, and audit coverage.

**Architecture:** Each item is implemented as a separate, testable phase with its own regression tests and commit. Database guarantees go first so later service work sits on a stronger data model; idempotency comes before ACL/audit because it changes route wiring; ACL comes before audit so audit entries can report final permission-aware behavior.

**Tech Stack:** Go 1.25, Gin, GORM, PostgreSQL 16, raw SQL migrations, WSL Ubuntu 22.04 commands via `wsl -e bash -c "..."`.

---

## Execution Rules

- Run all Go, Docker, and test commands inside WSL from `/mnt/e/My Work/RIMS/rims-goProgect`.
- Do not revert existing dirty files. The current worktree already contains prior security/concurrency fixes.
- Use TDD for every behavior change: write the failing test, run it red, implement the minimum fix, run it green.
- After each phase, run:

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./..."
```

- After each phase, run a whitespace check from the workspace root:

```powershell
git diff --check
```

---

## File Structure

### Phase 1: Database Integrity Constraints

- Create: `rims-goProgect/migrations/000008_inventory_constraints.sql`
- Create: `rims-goProgect/tests/deploy/migration_constraints_test.go`

### Phase 2: Idempotency

- Create: `rims-goProgect/migrations/000009_idempotency_keys.sql`
- Create: `rims-goProgect/internal/idempotency/model.go`
- Create: `rims-goProgect/internal/idempotency/repository.go`
- Create: `rims-goProgect/internal/idempotency/service.go`
- Create: `rims-goProgect/internal/idempotency/service_test.go`
- Create: `rims-goProgect/internal/middleware/idempotency.go`
- Create: `rims-goProgect/internal/middleware/idempotency_test.go`
- Modify: `rims-goProgect/internal/app/app.go`
- Modify: `rims-goProgect/internal/app/router.go`
- Modify: `rims-goProgect/internal/modules/document/routes.go`
- Modify: `rims-goProgect/internal/modules/product/routes.go`
- Modify: `rims-goProgect/internal/modules/file/routes.go`

### Phase 3: File Business ACL

- Create: `rims-goProgect/internal/app/file_acl.go`
- Create: `rims-goProgect/internal/app/file_acl_test.go`
- Create: `rims-goProgect/internal/modules/file/service_acl_test.go`
- Modify: `rims-goProgect/internal/modules/file/service.go`
- Modify: `rims-goProgect/internal/modules/file/handler.go`
- Modify: `rims-goProgect/internal/app/router.go`

### Phase 4: Audit Coverage

- Create: `rims-goProgect/internal/modules/audit/testutil_test.go`
- Create or extend: handler/service tests in `user`, `warehouse`, `product`, and `file`
- Modify: `rims-goProgect/internal/modules/user/handler.go`
- Modify: `rims-goProgect/internal/modules/warehouse/handler.go`
- Modify: `rims-goProgect/internal/modules/warehouse/service.go`
- Modify: `rims-goProgect/internal/modules/product/handler.go`
- Modify: `rims-goProgect/internal/modules/file/handler.go`
- Modify: `rims-goProgect/internal/app/router.go`

---

## Phase 1: Database Integrity Constraints

### Task 1.1: Write Failing Migration Constraint Test

**Files:**
- Create: `rims-goProgect/tests/deploy/migration_constraints_test.go`

- [ ] **Step 1: Add the test**

```go
// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package deploy_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func migrationRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func TestInventoryConstraintMigrationExists(t *testing.T) {
	root := migrationRepoRoot(t)
	path := filepath.Join(root, "rims-goProgect", "migrations", "000008_inventory_constraints.sql")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read inventory constraints migration: %v", err)
	}
	sql := strings.ToLower(string(b))

	required := []string{
		"chk_inventories_quantity_non_negative",
		"chk_inventories_locked_qty_non_negative",
		"chk_inventories_alert_threshold_non_negative",
		"chk_non_std_inventories_quantity_non_negative",
		"chk_non_std_inventories_converted_qty_non_negative",
		"chk_non_std_inventories_converted_qty_lte_quantity",
		"chk_document_lines_quantity_non_negative",
		"validate constraint",
	}
	for _, token := range required {
		if !strings.Contains(sql, token) {
			t.Fatalf("expected migration to contain %q", token)
		}
	}
}
```

- [ ] **Step 2: Run the test red**

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./tests/deploy/..."
```

Expected: `FAIL` because `000008_inventory_constraints.sql` does not exist.

### Task 1.2: Add Constraint Migration

**Files:**
- Create: `rims-goProgect/migrations/000008_inventory_constraints.sql`

- [ ] **Step 1: Add the migration**

```sql
-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Copyright (c) 2026 ShangBin Wang

ALTER TABLE inventories
  ADD CONSTRAINT chk_inventories_quantity_non_negative CHECK (quantity >= 0) NOT VALID,
  ADD CONSTRAINT chk_inventories_locked_qty_non_negative CHECK (locked_qty >= 0) NOT VALID,
  ADD CONSTRAINT chk_inventories_alert_threshold_non_negative CHECK (alert_threshold >= 0) NOT VALID;

ALTER TABLE non_std_inventories
  ADD CONSTRAINT chk_non_std_inventories_quantity_non_negative CHECK (quantity >= 0) NOT VALID,
  ADD CONSTRAINT chk_non_std_inventories_converted_qty_non_negative CHECK (converted_qty >= 0) NOT VALID,
  ADD CONSTRAINT chk_non_std_inventories_converted_qty_lte_quantity CHECK (converted_qty <= quantity) NOT VALID;

ALTER TABLE document_lines
  ADD CONSTRAINT chk_document_lines_quantity_non_negative CHECK (quantity >= 0) NOT VALID;

ALTER TABLE inventories VALIDATE CONSTRAINT chk_inventories_quantity_non_negative;
ALTER TABLE inventories VALIDATE CONSTRAINT chk_inventories_locked_qty_non_negative;
ALTER TABLE inventories VALIDATE CONSTRAINT chk_inventories_alert_threshold_non_negative;

ALTER TABLE non_std_inventories VALIDATE CONSTRAINT chk_non_std_inventories_quantity_non_negative;
ALTER TABLE non_std_inventories VALIDATE CONSTRAINT chk_non_std_inventories_converted_qty_non_negative;
ALTER TABLE non_std_inventories VALIDATE CONSTRAINT chk_non_std_inventories_converted_qty_lte_quantity;

ALTER TABLE document_lines VALIDATE CONSTRAINT chk_document_lines_quantity_non_negative;
```

- [ ] **Step 2: Run the test green**

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./tests/deploy/..."
```

Expected: `ok rims-go/tests/deploy`.

- [ ] **Step 3: Run full verification**

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./..."
```

Expected: all packages pass.

- [ ] **Step 4: Commit phase 1**

```bash
git add rims-goProgect/migrations/000008_inventory_constraints.sql rims-goProgect/tests/deploy/migration_constraints_test.go
git commit -m "chore: add inventory integrity constraints"
```

---

## Phase 2: Idempotency

### Task 2.1: Add Idempotency Migration and Static Test

**Files:**
- Create: `rims-goProgect/migrations/000009_idempotency_keys.sql`
- Extend: `rims-goProgect/tests/deploy/migration_constraints_test.go`

- [ ] **Step 1: Extend the migration test**

Add this test to `migration_constraints_test.go`:

```go
func TestIdempotencyMigrationExists(t *testing.T) {
	root := migrationRepoRoot(t)
	path := filepath.Join(root, "rims-goProgect", "migrations", "000009_idempotency_keys.sql")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read idempotency migration: %v", err)
	}
	sql := strings.ToLower(string(b))

	required := []string{
		"create table if not exists idempotency_keys",
		"idempotency_key",
		"request_hash",
		"response_body",
		"state",
		"idx_idempotency_user_scope_key",
		"unique",
	}
	for _, token := range required {
		if !strings.Contains(sql, token) {
			t.Fatalf("expected idempotency migration to contain %q", token)
		}
	}
}
```

- [ ] **Step 2: Run test red**

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./tests/deploy/..."
```

Expected: `FAIL` because `000009_idempotency_keys.sql` does not exist.

- [ ] **Step 3: Add the migration**

```sql
-- SPDX-License-Identifier: AGPL-3.0-or-later
-- Copyright (c) 2026 ShangBin Wang

CREATE TABLE IF NOT EXISTS idempotency_keys (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    user_id BIGINT NOT NULL,
    scope VARCHAR(128) NOT NULL,
    idempotency_key VARCHAR(128) NOT NULL,
    request_hash CHAR(64) NOT NULL,
    state VARCHAR(16) NOT NULL DEFAULT 'processing',
    status_code INT NOT NULL DEFAULT 0,
    response_body JSONB NOT NULL DEFAULT '{}'::jsonb,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_idempotency_user_scope_key
    ON idempotency_keys(user_id, scope, idempotency_key)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_idempotency_expires_at
    ON idempotency_keys(expires_at)
    WHERE deleted_at IS NULL;
```

- [ ] **Step 4: Run deploy tests green**

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./tests/deploy/..."
```

Expected: `ok rims-go/tests/deploy`.

### Task 2.2: Implement Idempotency Service with Unit Tests

**Files:**
- Create: `rims-goProgect/internal/idempotency/model.go`
- Create: `rims-goProgect/internal/idempotency/repository.go`
- Create: `rims-goProgect/internal/idempotency/service.go`
- Create: `rims-goProgect/internal/idempotency/service_test.go`
- Modify: `rims-goProgect/internal/app/app.go`

- [ ] **Step 1: Write service tests**

Create `service_test.go` with a fake repository and these behaviors:

```go
func TestBeginReservesNewKey(t *testing.T) {
	store := newMemoryRepo()
	svc := NewService(store, time.Hour)

	result, err := svc.Begin(context.Background(), BeginRequest{
		UserID: 1, Scope: "POST /api/v1/documents", Key: "abc", RequestHash: "hash-a",
	})
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}
	if result.Kind != DecisionProceed {
		t.Fatalf("expected proceed, got %v", result.Kind)
	}
}

func TestBeginReplaysCompletedKeyWithSameHash(t *testing.T) {
	store := newMemoryRepo()
	svc := NewService(store, time.Hour)
	_, _ = svc.Begin(context.Background(), BeginRequest{UserID: 1, Scope: "s", Key: "abc", RequestHash: "hash-a"})
	_ = svc.Complete(context.Background(), CompleteRequest{
		UserID: 1, Scope: "s", Key: "abc", StatusCode: 201, ResponseBody: []byte(`{"code":0}`),
	})

	result, err := svc.Begin(context.Background(), BeginRequest{UserID: 1, Scope: "s", Key: "abc", RequestHash: "hash-a"})
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}
	if result.Kind != DecisionReplay || result.StatusCode != 201 || string(result.ResponseBody) != `{"code":0}` {
		t.Fatalf("expected replay result, got %+v", result)
	}
}

func TestBeginRejectsSameKeyWithDifferentHash(t *testing.T) {
	store := newMemoryRepo()
	svc := NewService(store, time.Hour)
	_, _ = svc.Begin(context.Background(), BeginRequest{UserID: 1, Scope: "s", Key: "abc", RequestHash: "hash-a"})

	_, err := svc.Begin(context.Background(), BeginRequest{UserID: 1, Scope: "s", Key: "abc", RequestHash: "hash-b"})
	if err == nil {
		t.Fatal("expected duplicate key with different request hash to fail")
	}
}

func TestBeginReportsProcessingDuplicate(t *testing.T) {
	store := newMemoryRepo()
	svc := NewService(store, time.Hour)
	_, _ = svc.Begin(context.Background(), BeginRequest{UserID: 1, Scope: "s", Key: "abc", RequestHash: "hash-a"})

	result, err := svc.Begin(context.Background(), BeginRequest{UserID: 1, Scope: "s", Key: "abc", RequestHash: "hash-a"})
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}
	if result.Kind != DecisionProcessing {
		t.Fatalf("expected processing, got %v", result.Kind)
	}
}
```

- [ ] **Step 2: Run tests red**

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/idempotency/..."
```

Expected: `FAIL` because package and types are not implemented.

- [ ] **Step 3: Add idempotency model**

```go
// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package idempotency

import (
	"time"

	"rims-go/internal/types"
)

const (
	StateProcessing = "processing"
	StateCompleted  = "completed"
)

type Record struct {
	types.BaseModel
	UserID         uint      `gorm:"not null;uniqueIndex:idx_idempotency_user_scope_key,priority:1"`
	Scope          string    `gorm:"size:128;not null;uniqueIndex:idx_idempotency_user_scope_key,priority:2"`
	IdempotencyKey string    `gorm:"column:idempotency_key;size:128;not null;uniqueIndex:idx_idempotency_user_scope_key,priority:3"`
	RequestHash    string    `gorm:"size:64;not null"`
	State          string    `gorm:"size:16;not null;default:'processing'"`
	StatusCode     int       `gorm:"not null;default:0"`
	ResponseBody   string    `gorm:"type:jsonb;not null;default:'{}'"`
	ExpiresAt      time.Time `gorm:"not null;index"`
}

func (Record) TableName() string { return "idempotency_keys" }
```

- [ ] **Step 4: Add service contracts and logic**

Implement `Begin`, `Complete`, and `Release` in `service.go`:

```go
type DecisionKind int

const (
	DecisionProceed DecisionKind = iota + 1
	DecisionReplay
	DecisionProcessing
)

type BeginRequest struct {
	UserID      uint
	Scope       string
	Key         string
	RequestHash string
}

type BeginResult struct {
	Kind         DecisionKind
	StatusCode   int
	ResponseBody []byte
}

type CompleteRequest struct {
	UserID       uint
	Scope        string
	Key          string
	StatusCode   int
	ResponseBody []byte
}
```

Rules:
- New key creates `StateProcessing` and returns `DecisionProceed`.
- Same key and same hash with `StateCompleted` returns `DecisionReplay`.
- Same key and same hash with `StateProcessing` returns `DecisionProcessing`.
- Same key and different hash returns `types.ErrValidation("Idempotency-Key已用于不同请求")`.
- `Complete` stores status and JSON response for successful handler responses.
- `Release` soft-deletes or removes a processing record when the handler returns `5xx`.

- [ ] **Step 5: Add repository implementation**

Use GORM methods:
- `Get(ctx, userID, scope, key) (*Record, error)`
- `Create(ctx, record) error`
- `Complete(ctx, userID, scope, key, statusCode int, responseBody string) error`
- `DeleteProcessing(ctx, userID, scope, key) error`

Use `db.FromCtx(ctx, r.gormDB)` in every method.

- [ ] **Step 6: Register model in AutoMigrate**

Modify `internal/app/app.go` AutoMigrate list to include:

```go
&idempotency.Record{},
```

- [ ] **Step 7: Run idempotency tests green**

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/idempotency/..."
```

Expected: `ok rims-go/internal/idempotency`.

### Task 2.3: Add Idempotency Middleware and Wire Routes

**Files:**
- Create: `rims-goProgect/internal/middleware/idempotency.go`
- Create: `rims-goProgect/internal/middleware/idempotency_test.go`
- Modify: `rims-goProgect/internal/app/router.go`
- Modify: `rims-goProgect/internal/modules/document/routes.go`
- Modify: `rims-goProgect/internal/modules/product/routes.go`
- Modify: `rims-goProgect/internal/modules/file/routes.go`

- [ ] **Step 1: Write middleware tests**

Test these cases:

```go
func TestIdempotencyMiddlewarePassesWithoutHeader(t *testing.T)
func TestIdempotencyMiddlewareCachesSuccessfulResponse(t *testing.T)
func TestIdempotencyMiddlewareReplaysCompletedResponse(t *testing.T)
func TestIdempotencyMiddlewareRejectsDifferentBodyForSameKey(t *testing.T)
func TestIdempotencyMiddlewareReturnsConflictForProcessingDuplicate(t *testing.T)
```

Use `gin.CreateTestContext`, a fake idempotency service, and a handler returning `201` with body `{"code":0,"data":{"id":1}}`.

- [ ] **Step 2: Run middleware tests red**

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/middleware/..."
```

Expected: `FAIL` because `Idempotency` middleware is not implemented.

- [ ] **Step 3: Implement middleware**

Middleware behavior:
- If `Idempotency-Key` header is empty, call `c.Next()`.
- Build scope as `c.Request.Method + " " + c.FullPath()`.
- Compute request hash from method, full path, user ID, warehouse ID, and request body bytes.
- Restore `c.Request.Body` after hashing.
- On `DecisionReplay`, write cached status/body and abort.
- On `DecisionProcessing`, return `409` with `types.ErrInvalidState("请求正在处理中")`.
- Wrap `gin.ResponseWriter` to capture response body.
- Cache only `2xx` responses; release key for `5xx`.

- [ ] **Step 4: Wire services in router**

In `internal/app/router.go`:

```go
idempotencyRepo := idempotency.NewRepository(gormDB)
idempotencySvc := idempotency.NewService(idempotencyRepo, 24*time.Hour)
idempotencyMw := middleware.Idempotency(idempotencySvc, cfg.MaxUploadMB)
```

Pass `idempotencyMw` to document, product, and file route registration.

- [ ] **Step 5: Apply middleware only to mutation routes**

In document routes:

```go
docs.POST("", idempotencyMw, handler.CreateDocument)
docs.POST("/:id/complete", idempotencyMw, handler.CompleteDocument)
```

In product routes:

```go
nonStd.POST("/:id/convert", idempotencyMw, handler.ConvertNonStd)
```

In file routes:

```go
files.POST("/upload", idempotencyMw, handler.Upload)
```

- [ ] **Step 6: Run focused and full tests**

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/idempotency/... ./internal/middleware/... ./internal/modules/document/... ./internal/modules/product/... ./internal/modules/file/..."
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./..."
```

Expected: all packages pass.

- [ ] **Step 7: Commit phase 2**

```bash
git add rims-goProgect/migrations/000009_idempotency_keys.sql rims-goProgect/internal/idempotency rims-goProgect/internal/middleware/idempotency.go rims-goProgect/internal/middleware/idempotency_test.go rims-goProgect/internal/app rims-goProgect/internal/modules/document/routes.go rims-goProgect/internal/modules/product/routes.go rims-goProgect/internal/modules/file/routes.go rims-goProgect/tests/deploy/migration_constraints_test.go
git commit -m "feat: add idempotency protection for critical writes"
```

---

## Phase 3: File Business ACL

### Task 3.1: Add File ACL Service Tests

**Files:**
- Create: `rims-goProgect/internal/modules/file/service_acl_test.go`

- [ ] **Step 1: Write failing ACL tests**

Create tests for:

```go
func TestDeleteAllowsBusinessAccessWhenUploaderDiffers(t *testing.T)
func TestDeleteRejectsBusinessAccessDenied(t *testing.T)
func TestOpenForDownloadRejectsPrivateFileWithoutBusinessAccess(t *testing.T)
func TestOpenForDownloadAllowsAdmin(t *testing.T)
```

Use a fake checker:

```go
type fileAccessCheckerStub struct {
	allowed bool
	calls   int
}

func (s *fileAccessCheckerStub) CanAccessFile(ctx context.Context, actor FileActor, f *FileAttachment, action FileAction) (bool, error) {
	s.calls++
	return s.allowed, nil
}
```

- [ ] **Step 2: Run file tests red**

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/modules/file/..."
```

Expected: `FAIL` because `FileActor`, `FileAction`, and checker-aware methods do not exist.

### Task 3.2: Implement File ACL Contract

**Files:**
- Modify: `rims-goProgect/internal/modules/file/service.go`
- Modify: `rims-goProgect/internal/modules/file/handler.go`

- [ ] **Step 1: Add ACL types**

Add to `service.go`:

```go
type FileAction string

const (
	FileActionRead   FileAction = "read"
	FileActionDelete FileAction = "delete"
)

type FileActor struct {
	UserID  uint
	IsAdmin bool
}

type BusinessAccessChecker interface {
	CanAccessFile(ctx context.Context, actor FileActor, f *FileAttachment, action FileAction) (bool, error)
}

type ownerOrAdminChecker struct{}

func (ownerOrAdminChecker) CanAccessFile(ctx context.Context, actor FileActor, f *FileAttachment, action FileAction) (bool, error) {
	return actor.IsAdmin || f.CreatedBy == actor.UserID, nil
}
```

- [ ] **Step 2: Extend `NewFileService`**

Change constructor to accept a checker:

```go
func NewFileService(repo FileRepository, storage Storage, maxUploadMB int, allowedExts string, downloadURLFormat string, checker BusinessAccessChecker) *FileService
```

If `checker == nil`, use `ownerOrAdminChecker{}`.

- [ ] **Step 3: Centralize authorization in service**

Add:

```go
func (s *FileService) authorize(ctx context.Context, actor FileActor, f *FileAttachment, action FileAction) error {
	if actor.IsAdmin || f.CreatedBy == actor.UserID {
		return nil
	}
	ok, err := s.access.CanAccessFile(ctx, actor, f, action)
	if err != nil {
		return types.ErrSystem(err)
	}
	if !ok {
		return types.ErrForbidden()
	}
	return nil
}
```

Change `Delete` and `OpenForDownload` signatures:

```go
func (s *FileService) Delete(ctx context.Context, id uint, actor FileActor) error
func (s *FileService) OpenForDownload(ctx context.Context, id uint, actor FileActor) (io.ReadCloser, *FileAttachment, error)
```

For public files, `OpenForDownload` can allow read without ACL. Private files must call `authorize`.

- [ ] **Step 4: Update handler**

In `Download` and `Delete`, construct:

```go
actor := FileActor{UserID: types.GetUserID(c), IsAdmin: types.IsAdmin(c)}
```

Remove handler-local ACL that only checks uploader/admin.

- [ ] **Step 5: Run file tests green**

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/modules/file/..."
```

Expected: `ok rims-go/internal/modules/file`.

### Task 3.3: Implement Business ACL Checker in App Composition

**Files:**
- Create: `rims-goProgect/internal/app/file_acl.go`
- Create: `rims-goProgect/internal/app/file_acl_test.go`
- Modify: `rims-goProgect/internal/app/router.go`

- [ ] **Step 1: Add app-level checker tests**

Test cases:

```go
func TestFileACLAllowsDocumentAttachmentWhenUserHasDocumentWarehouse(t *testing.T)
func TestFileACLRejectsDocumentAttachmentWhenUserLacksDocumentWarehouse(t *testing.T)
func TestFileACLFallsBackToOwnerOrAdminForUnscopedBusinessTypes(t *testing.T)
```

- [ ] **Step 2: Add checker implementation**

Create `internal/app/file_acl.go`:

```go
type fileAccessChecker struct {
	docRepo document.DocumentRepository
	whRepo  warehouse.UserWarehouseRepository
}

func (c fileAccessChecker) CanAccessFile(ctx context.Context, actor file.FileActor, f *file.FileAttachment, action file.FileAction) (bool, error) {
	if actor.IsAdmin || f.CreatedBy == actor.UserID {
		return true, nil
	}
	if f.BusinessID == nil {
		return false, nil
	}
	switch f.BusinessType {
	case file.BusinessTypeDocAttachment:
		doc, err := c.docRepo.GetByID(ctx, *f.BusinessID)
		if err != nil {
			return false, err
		}
		return c.whRepo.HasAccess(ctx, actor.UserID, doc.WarehouseID)
	case file.BusinessTypeProductImage:
		return action == file.FileActionRead, nil
	default:
		return false, nil
	}
}
```

- [ ] **Step 3: Inject checker**

In `router.go`, change file service construction:

```go
fileACL := fileAccessChecker{docRepo: docRepo, whRepo: userWarehouseRepo}
fileSvc := file.NewFileService(
	fileRepo, localStorage,
	cfg.MaxUploadMB, cfg.AllowedExts,
	"/api/v1/files/%d/download",
	fileACL,
)
```

- [ ] **Step 4: Run focused and full tests**

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/modules/file/... ./internal/app/..."
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./..."
```

Expected: all packages pass.

- [ ] **Step 5: Commit phase 3**

```bash
git add rims-goProgect/internal/modules/file rims-goProgect/internal/app/file_acl.go rims-goProgect/internal/app/file_acl_test.go rims-goProgect/internal/app/router.go
git commit -m "fix: enforce business ACL for private files"
```

---

## Phase 4: Audit Coverage

### Task 4.1: Add Shared Audit Test Stub

**Files:**
- Create: `rims-goProgect/internal/modules/audit/testutil_test.go`

- [ ] **Step 1: Add test helper**

```go
// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package audit

import "context"

type LoggerStub struct {
	Entries []Entry
	Err     error
}

func (s *LoggerStub) Log(ctx context.Context, e Entry) error {
	s.Entries = append(s.Entries, e)
	return s.Err
}
```

This helper is only visible to `audit` package tests. For other packages, create local `auditLoggerStub` types to avoid exporting test-only API.

### Task 4.2: Audit User and Role Management

**Files:**
- Modify: `rims-goProgect/internal/modules/user/handler.go`
- Extend or create: `rims-goProgect/internal/modules/user/handler_audit_test.go`

- [ ] **Step 1: Write failing tests**

Cover these successful operations:

```go
func TestCreateUserWritesAuditLog(t *testing.T)
func TestUpdateUserWritesAuditLog(t *testing.T)
func TestDeleteUserWritesAuditLog(t *testing.T)
func TestAssignPermissionsWritesAuditLog(t *testing.T)
```

Expected entries:
- `ActionCreate`, `ResourceUser`
- `ActionUpdate`, `ResourceUser`
- `ActionDelete`, `ResourceUser`
- `ActionAssign`, `ResourcePermission`

- [ ] **Step 2: Implement handler-side audit logging**

After each service call succeeds, log with:

```go
_ = h.auditSvc.Log(c.Request.Context(), audit.Entry{
	Actor:      audit.ActorFromContext(c),
	Action:     audit.ActionUpdate,
	Resource:   audit.ResourceUser,
	ResourceID: &id,
	After: map[string]any{
		"request": req,
	},
})
```

For failure paths where the service returns an `AppError`, log `ResultFailure`, `ErrorCode`, and `ErrorMsg` for role/permission writes.

- [ ] **Step 3: Run user tests**

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/modules/user/..."
```

Expected: `ok rims-go/internal/modules/user`.

### Task 4.3: Audit Warehouse Binding Changes

**Files:**
- Modify: `rims-goProgect/internal/modules/warehouse/handler.go`
- Modify if needed: `rims-goProgect/internal/app/router.go`
- Create: `rims-goProgect/internal/modules/warehouse/handler_audit_test.go`

- [ ] **Step 1: Write failing tests**

Cover:

```go
func TestBindUserWritesAuditLog(t *testing.T)
func TestUnbindUserWritesAuditLog(t *testing.T)
func TestSetDefaultWarehouseWritesAuditLog(t *testing.T)
```

Expected entries:
- `ActionBind`, `ResourceUserWarehouse`
- `ActionUnbind`, `ResourceUserWarehouse`
- `ActionUpdate`, `ResourceUserWarehouse`

- [ ] **Step 2: Inject audit logger into warehouse handler**

Constructor shape:

```go
func NewHandler(warehouseSvc *WarehouseService, auditSvc AuditLogger) *Handler
```

Define local narrow interface:

```go
type AuditLogger interface {
	Log(ctx context.Context, e audit.Entry) error
}
```

- [ ] **Step 3: Log after successful writes**

Use `audit.ActorFromContext(c)` and include `warehouseID`, `targetUserID`, and request fields in `After`.

- [ ] **Step 4: Run warehouse tests**

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/modules/warehouse/..."
```

Expected: `ok rims-go/internal/modules/warehouse`.

### Task 4.4: Audit Product Cost and Inventory Settings

**Files:**
- Modify: `rims-goProgect/internal/modules/product/handler.go`
- Create: `rims-goProgect/internal/modules/product/handler_audit_test.go`

- [ ] **Step 1: Write failing tests**

Cover:

```go
func TestUpdateProductCostWritesAuditLog(t *testing.T)
func TestUpdateInventorySettingsWritesAuditLog(t *testing.T)
func TestConvertNonStdWritesAuditLog(t *testing.T)
```

Expected entries:
- `ActionUpdate`, `ResourceProduct`, details include `costPrice` when present.
- `ActionUpdate`, `ResourceInventory`, details include `alertThreshold` and `status`.
- `ActionConvert`, `ResourceNonStdInventory`, details include `productID` and `quantity`.

- [ ] **Step 2: Inject audit logger into product handler**

Constructor shape:

```go
func NewHandler(productSvc *ProductService, auditSvc AuditLogger) *Handler
```

- [ ] **Step 3: Log successful writes**

Use actor and request fields:

```go
_ = h.auditSvc.Log(c.Request.Context(), audit.Entry{
	Actor:      audit.ActorFromContext(c),
	Action:     audit.ActionUpdate,
	Resource:   audit.ResourceInventory,
	ResourceID: &id,
	After: map[string]any{
		"alertThreshold": req.AlertThreshold,
		"status":         req.Status,
	},
})
```

- [ ] **Step 4: Run product tests**

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/modules/product/..."
```

Expected: `ok rims-go/internal/modules/product`.

### Task 4.5: Audit File Delete

**Files:**
- Modify: `rims-goProgect/internal/modules/file/handler.go`
- Create: `rims-goProgect/internal/modules/file/handler_audit_test.go`
- Modify: `rims-goProgect/internal/app/router.go`

- [ ] **Step 1: Write failing test**

```go
func TestDeleteFileWritesAuditLog(t *testing.T)
```

Expected entry:
- `ActionDelete`
- `ResourceFile`
- `ResourceID` set to file ID

- [ ] **Step 2: Inject audit logger into file handler**

Constructor shape:

```go
func NewHandler(svc *FileService, auditSvc AuditLogger) *Handler
```

- [ ] **Step 3: Log after successful delete**

```go
_ = h.auditSvc.Log(c.Request.Context(), audit.Entry{
	Actor:      audit.ActorFromContext(c),
	Action:     audit.ActionDelete,
	Resource:   audit.ResourceFile,
	ResourceID: &id,
})
```

- [ ] **Step 4: Run file tests**

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./internal/modules/file/..."
```

Expected: `ok rims-go/internal/modules/file`.

### Task 4.6: Final Audit Wiring and Verification

**Files:**
- Modify: `rims-goProgect/internal/app/router.go`

- [ ] **Step 1: Update composition root**

Pass `auditSvc` into:

```go
warehouse.NewHandler(warehouseSvc, auditSvc)
product.NewHandler(productSvc, auditSvc)
file.NewHandler(fileSvc, auditSvc)
```

Keep `user.NewHandler(userSvc, roleSvc, auditSvc)` as already wired.

- [ ] **Step 2: Run full verification**

```bash
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/gofmt -w internal/modules/user/handler.go internal/modules/warehouse/handler.go internal/modules/product/handler.go internal/modules/file/handler.go internal/app/router.go"
wsl -e bash -c "cd '/mnt/e/My Work/RIMS/rims-goProgect' && ~/local/go/bin/go test ./..."
git diff --check
```

Expected:
- `go test ./...` exits `0`.
- `git diff --check` exits `0`; CRLF warnings are acceptable.

- [ ] **Step 3: Commit phase 4**

```bash
git add rims-goProgect/internal/modules/user rims-goProgect/internal/modules/warehouse rims-goProgect/internal/modules/product rims-goProgect/internal/modules/file rims-goProgect/internal/app/router.go
git commit -m "feat: expand audit coverage for high-risk writes"
```

---

## Self-Review

- Spec coverage: the plan covers all four requested items in order: DB constraints, idempotency, file ACL, audit coverage.
- Placeholder scan: no task relies on an unspecified future choice; each phase names exact files, tests, implementation shape, and verification commands.
- Type consistency: idempotency uses `Record`, `BeginRequest`, `BeginResult`, `DecisionKind`; file ACL uses `FileActor`, `FileAction`, `BusinessAccessChecker`; audit entries use existing `audit.Entry`, `audit.ActorFromContext`, action/resource constants.
- Risk control: every phase ends with focused tests and `go test ./...`; each phase can be committed independently.
