<!-- SPDX-License-Identifier: AGPL-3.0-or-later -->
<!-- Copyright (c) 2026 ShangBin Wang -->

# RIMS Business Module Internal Diagrams

Common visual style for all diagrams:
- 16:9 horizontal technical architecture diagram
- Dark slate #0F172A background with subtle grid #1E293B
- JetBrains Mono / SF Mono style text
- Rounded rectangles, 8px radius, clean thin borders
- Orthogonal arrows, no arrows crossing through nodes
- Use English ASCII labels for readability
- No real brand logos, no emojis, no 3D, no decorative blobs
- Include a tiny legend: Blue = HTTP entry, Emerald = service logic, Violet = data model/repository, Rose = auth/audit/security, Amber = cross-module dependency

## 1. User & RBAC

Title: "RIMS Module Internals - User & RBAC"
Subtitle: "Authentication, user CRUD, roles, permissions"

Nodes:
- Routes: /auth, /users, /roles, /permissions
- Handler: Login, User CRUD, Password, Role CRUD
- Services: UserService, RoleService
- Repositories: UserRepository, RoleRepository
- Models: users, roles, permissions, role_permissions
- Security: JWT TokenService, Permission Middleware
- Audit: login, user writes, role writes, password changes

Flows:
- POST /auth/login -> Handler.Login -> UserService.Login -> UserRepository.FindByUsername -> JWT TokenService -> LoginResponse
- /users protected routes -> JWTAuth -> Permission Middleware for admin writes -> UserService -> UserRepository -> users table
- /roles protected routes -> RoleService -> RoleRepository -> roles + permissions
- Handler -> AuditService for write events

## 2. Warehouse

Title: "RIMS Module Internals - Warehouse"
Subtitle: "Warehouse CRUD, user binding, default and current warehouse"

Nodes:
- Routes: /warehouses, /users/me/warehouses
- Handler: Warehouse CRUD, BindUsers, SwitchCurrentWarehouse
- Service: WarehouseService
- Repositories: WarehouseRepository, UserWarehouseRepository
- Models: warehouses, user_warehouses
- Middleware: JWTAuth, Permission Middleware, WarehouseScope consumer
- Audit: warehouse CRUD and user binding events

Flows:
- Warehouse CRUD -> permission checks -> WarehouseService -> WarehouseRepository -> warehouses
- BindUsers / UnbindUser -> WarehouseService transaction -> UserWarehouseRepository -> user_warehouses
- MyWarehouses / SetDefault / SwitchCurrent -> UserWarehouseRepository
- WarehouseScope middleware reads X-Warehouse-ID or default warehouse -> validates access
- Handler -> AuditService for write events

## 3. Product & Inventory

Title: "RIMS Module Internals - Product & Inventory"
Subtitle: "Catalog, standard stock, non-standard stock, conversion"

Nodes:
- Routes: /products, /inventory, /non-std-inventory
- Handler: Product CRUD, Inventory update, NonStd CRUD, Convert
- Service: ProductService
- Repositories: ProductRepository, InventoryRepository, NonStdInventoryRepository
- Models: products, inventories, non_std_inventories
- Middleware: JWTAuth, WarehouseScope, Permission Middleware, Idempotency for convert
- Audit: product, inventory, non-standard write events
- Concurrency: row locks for inventory and non-standard conversion

Flows:
- Product CRUD -> ProductService -> ProductRepository -> products
- Inventory update -> WarehouseScope -> ProductService -> InventoryRepository.GetForUpdate -> inventories
- NonStd CRUD -> permission protected -> NonStdInventoryRepository
- ConvertNonStd -> Idempotency -> txRunner -> NonStdInventory row lock -> Inventory row lock -> update both
- Handler -> AuditService for write events

## 4. Document

Title: "RIMS Module Internals - Document Engine"
Subtitle: "Inbound, sales, return, transfer, stocktake, conversion"

Nodes:
- Routes: /documents, /transactions
- Handler: Create, List, Complete, ConfirmStocktake, SettleStocktake
- Service: DocumentService
- Repositories: DocumentRepository, DocumentLineRepository, InventoryTransactionRepository
- Cross-module repos: InventoryRepository, ProductRepository, NonStdInventoryRepository
- Models: documents, document_lines, inventory_transactions
- Middleware: JWTAuth, WarehouseScope, Idempotency for create/complete
- Audit: create, complete, confirm, settle inside transaction
- Concurrency: document row lock, inventory row locks, deterministic transfer lock order

Flows:
- CreateDocument -> txRunner -> generate doc no -> documents + document_lines -> audit create
- CompleteDocument -> txRunner -> DocumentRepository.GetByIDForUpdate -> validate status/type -> apply inventory movement -> inventory_transactions -> audit complete
- Stocktake: recording -> confirmed -> settled -> inventory adjustment -> transaction log
- Transfer -> lock source/destination inventory in deterministic order -> out transaction + in transaction
- Transactions list -> InventoryTransactionRepository

## 5. Report

Title: "RIMS Module Internals - Report"
Subtitle: "Read-only analytics over documents, transactions and inventory"

Nodes:
- Routes: /reports/sales/*, /reports/inventory/*
- Handler: SalesStats, SalesTrend, Ranking, InventoryOverview, Turnover, SlowMoving
- Service: ReportService
- Repository: ReportRepository
- Data sources: documents, document_lines, inventory_transactions, inventories, products
- Middleware: JWTAuth, WarehouseScope
- Security: admin field gating for cost/profit/value
- Safeguards: max 366 day range, bucket and metric whitelist

Flows:
- Report routes -> WarehouseScope -> ReportService validation -> ReportRepository raw aggregate SQL
- Sales stats/trend/ranking -> documents + lines + products
- Inventory overview/turnover/slow-moving -> inventory_transactions + inventories + products
- isAdmin flag controls cost/profit/stock value fields

## 6. File Attachment

Title: "RIMS Module Internals - File Attachment"
Subtitle: "Upload, metadata, ACL, public product images"

Nodes:
- Routes: /files, /files/upload, /files/:id/download
- Public route: /uploads/*
- Handler: Upload, List, Get, Download, Delete
- Service: FileService
- Repository: FileRepository
- Storage: LocalStorage
- ACL: BusinessAccessChecker, fileAccessChecker
- Model: file_attachments
- Types: product_image, doc_attachment, import_template, export_result, other
- Safeguards: max size, allowed extension, MIME sniff, SHA-256, product_image requires businessId
- Audit: upload success events

Flows:
- Upload -> Idempotency -> FileService validation -> ACL create check -> stream buffer + hash + MIME -> LocalStorage.Save -> FileRepository.Create
- product_image -> public FileURL -> /uploads/*
- private files -> /api/v1/files/:id/download -> ACL read check -> LocalStorage.Open
- Delete -> ACL delete check -> soft delete metadata, object retained for cleanup

## 7. Audit

Title: "RIMS Module Internals - Audit"
Subtitle: "Append-only business audit log"

Nodes:
- Routes: /audit/logs, /audit/logs/:id
- Handler: List, Get
- Service: AuditService
- Repository: AuditRepository
- Model: audit_logs JSONB details
- Producer modules: User, Warehouse, Product, Document, File
- Context: traceId, userId, username, roleCode, warehouseId, IP, User-Agent
- Permission: audit:read
- Transaction behavior: db.FromCtx participates in outer business transaction

Flows:
- Producer module writes -> AuditService.Log -> AuditRepository.Create -> audit_logs
- In transaction: audit record commits or rolls back with business write
- Outside transaction: best-effort write
- Admin audit query -> JWTAuth -> Permission Middleware audit:read -> AuditService.List/Get -> filters by user, warehouse, resource, action, docNo, traceId, time, result
