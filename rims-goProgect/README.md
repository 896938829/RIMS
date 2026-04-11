# rims-go

零售端库存管理系统后端服务 / Retail Inventory Management System Backend Service

## 技术栈 / Tech Stack

- **Go 1.25** + **Gin** (HTTP) + **GORM** (ORM) + **PostgreSQL**
- **JWT** (golang-jwt/v5) 认证鉴权 / Authentication
- **Swagger** (swaggo) API 文档 / API Documentation
- **Viper** 配置管理 / Configuration
- **Docker Compose** 本地依赖 / Local Dependencies

## 架构概览 / Architecture Overview

```
客户端 Client
    │
    ▼
┌─────────────────────────────────────────────────────┐
│  Gin HTTP Server                                    │
│  ┌───────────────────────────────────────────────┐  │
│  │ Middleware Chain 中间件链                       │  │
│  │ Recovery → RequestID → Logger → CORS           │  │
│  │   → JWTAuth → WarehouseScope → Permission     │  │
│  └───────────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────────┐  │
│  │ Modules 业务模块                               │  │
│  │ ┌─────────┐ ┌───────────┐ ┌─────────────────┐│  │
│  │ │  User   │ │ Warehouse │ │    Product       ││  │
│  │ │ 用户认证 │ │ 仓库权限   │ │   商品库存       ││  │
│  │ └─────────┘ └───────────┘ └─────────────────┘│  │
│  │ ┌─────────┐ ┌───────────┐ ┌────────┐┌──────┐│  │
│  │ │Document │ │  Report   │ │  File  ││Audit ││  │
│  │ │单据流水  │ │  报表分析  │ │ 文件附件 ││审计   ││  │
│  │ └─────────┘ └───────────┘ └────────┘└──────┘│  │
│  └───────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────┘
    │
    ▼
 PostgreSQL
```

## 分层架构 / Layered Architecture

每个业务模块遵循统一分层 / Each module follows a consistent layered pattern:

```
Handler  (接口层 / HTTP Layer)        ← 参数绑定、Swagger 注解
   │                                    Request binding, Swagger annotations
   ▼
Service  (业务层 / Business Layer)    ← 业务规则、事务编排
   │                                    Business rules, transaction orchestration
   ▼
Repository (数据层 / Data Layer)      ← 接口定义 + GORM 实现
                                        Interface + GORM implementation
```

## 目录结构 / Project Layout

```
rims-goProgect/
├── cmd/server/main.go              # 入口 / Entry point
├── internal/
│   ├── app/
│   │   ├── app.go                  # 启动引导 / Bootstrap
│   │   └── router.go               # 路由注册、模块组装 / Route registration
│   ├── auth/
│   │   └── jwt.go                  # JWT 令牌服务 / Token service
│   ├── config/
│   │   └── config.go               # 环境配置加载 / Config loading
│   ├── db/
│   │   ├── db.go                   # GORM 数据库连接 / DB connection
│   │   └── tx.go                   # 事务上下文传播 / Transaction propagation
│   ├── middleware/
│   │   ├── jwt.go                  # JWT 认证中间件 / Auth middleware
│   │   ├── requestid.go            # 请求追踪 ID / Request trace ID
│   │   ├── logger.go               # 结构化请求日志 / Request logging
│   │   └── cors.go                 # 跨域支持 / CORS
│   ├── types/
│   │   ├── response.go             # 统一响应格式 / Unified response
│   │   ├── errors.go               # 业务错误码 / Business error codes
│   │   ├── pagination.go           # 分页 / Pagination
│   │   ├── base_model.go           # GORM 基础模型 / Base model
│   │   └── context.go              # Context 辅助函数 / Context helpers
│   └── modules/
│       ├── user/                   # ✅ 用户与认证 / User & Auth
│       ├── warehouse/              # ✅ 仓库与权限 / Warehouse & Permission
│       ├── product/                # ✅ 商品与库存 / Product & Inventory
│       ├── document/               # ✅ 单据与流水 / Documents & Transactions
│       ├── report/                 # ✅ 报表分析 / Reports & Analytics
│       ├── file/                   # ✅ 文件附件 / File & Attachment
│       └── audit/                  # ✅ 审计日志 / Audit & Log
├── migrations/                     # SQL 迁移脚本 / SQL migrations
├── docs/                           # Swagger 生成文件 / Generated Swagger docs
├── deploy/                         # Docker Compose 等 / Deployment configs
└── go.mod
```

> ✅ 已实现 / Implemented　　🔲 待实现 / Planned

> 所有业务模块均已落地。审计试点接入 `user.Login` + `document.CompleteDocument` 两处，其余写操作的审计接入见后续增强。
> All business modules are implemented. Audit pilot is wired into `user.Login` + `document.CompleteDocument`; remaining retrofit sites are listed under follow-up enhancements.

## 模块说明 / Module Details

| 模块 Module | 职责 Responsibility | 状态 Status |
|---|---|---|
| **user** 用户认证 | 登录、用户 CRUD、角色权限管理 / Login, user CRUD, role & permission management | ✅ |
| **warehouse** 仓库权限 | 仓库管理、用户仓库绑定、仓库范围校验 / Warehouse CRUD, user binding, scope validation | ✅ |
| **product** 商品库存 | 商品档案、标准/非标库存、库存预警、非标转标准 / Product catalog, standard/non-standard inventory, alerts, non-std conversion | ✅ |
| **document** 单据流水 | 入库/销售/退货/调拨/盘点/转换单、库存流水 / Inbound, sales, return, transfer, stocktaking, conversion orders & inventory transactions | ✅ |
| **report** 报表分析 | 销售统计、趋势、排行、库存概况、周转率、滞销预警 / Sales stats, trend, ranking, inventory overview, turnover rate, slow-moving alerts | ✅ |
| **file** 文件附件 | 文件上传/下载/删除、元数据管理、本地磁盘存储（可插拔 Storage 接口） / File upload/download/delete, metadata management, local-disk storage (pluggable Storage interface) | ✅ |
| **audit** 审计日志 | 审计日志写入（在业务事务内原子提交）、按用户/仓库/资源/动作/单据号/时间范围查询、管理员only / Audit log writes (atomic with the business transaction), query by user / warehouse / resource / action / docNo / time range, admin-only | ✅ |

## 模块内部结构 / Module Internal Structure

每个模块包含以下文件 / Each module contains:

| 文件 File | 职责 Responsibility |
|---|---|
| `model.go` | GORM 数据实体 / Data entities |
| `dto.go` | 请求/响应结构体 / Request/Response DTOs |
| `repository.go` | 数据访问接口 + 实现 / Data access interface + implementation |
| `service.go` | 业务逻辑 / Business logic |
| `handler.go` | HTTP 处理器 + Swagger 注解 / HTTP handlers + Swagger |
| `routes.go` | 路由自注册 / Route self-registration |

## 跨模块交互 / Cross-Module Interaction

- **依赖反转 / Dependency Inversion**: 消费方定义接口，提供方实现，在 `app.go` 中组装
  Consumer defines interface, provider implements, wired in `app.go`
- **事务传播 / Transaction Propagation**: 通过 `context.Context` 传递事务，`db.RunInTx()` / `db.FromCtx()` 实现跨 Repository 原子操作
  Transaction passed via `context.Context`, enabling atomic operations across repositories
- **审计注入 / Audit Injection**: `AuditLogger` 接口注入到需要审计的 Service 中
  `AuditLogger` interface injected into services that need auditing

## 快速开始 / Quick Start

所有命令在 WSL 中执行 / All commands run inside WSL:

```bash
# 1. 配置环境变量 / Configure environment
#    编辑项目根目录 .env 文件 / Edit .env in workspace root

# 2. 启动 PostgreSQL / Start PostgreSQL
docker compose --project-directory . -f deploy/docker-compose.yml up -d

# 3. 运行服务 / Run the server
cd rims-goProgect && go run ./cmd/server

# 4. 打开 Swagger UI / Open Swagger UI
#    http://127.0.0.1:8080/swagger/index.html
```

## API 接口 / API Endpoints

所有接口基于 `/api/v1` / All endpoints under `/api/v1`:

### 认证 Auth (公开 / Public)

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/auth/login` | 用户登录 / Login |

### 用户 Users (需认证 / Auth Required)

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/users/me` | 当前用户信息 / Current user info |
| PUT | `/api/v1/users/me/password` | 修改密码 / Change password |
| GET | `/api/v1/users/me/warehouses` | 我的仓库列表 / My warehouses |
| PUT | `/api/v1/users/me/warehouses/default` | 设置默认仓库 / Set default warehouse |
| PUT | `/api/v1/users/me/warehouses/current` | 切换当前仓库 / Switch current warehouse |
| POST | `/api/v1/users` | 创建用户 / Create user |
| GET | `/api/v1/users` | 用户列表 / List users |
| GET | `/api/v1/users/:id` | 用户详情 / Get user |
| PUT | `/api/v1/users/:id` | 更新用户 / Update user |
| DELETE | `/api/v1/users/:id` | 删除用户 / Delete user |
| PUT | `/api/v1/users/:id/password` | 重置密码 / Reset password (admin) |

### 角色与权限 Roles & Permissions (需认证 / Auth Required)

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/roles` | 创建角色 / Create role |
| GET | `/api/v1/roles` | 角色列表 / List roles |
| GET | `/api/v1/roles/:id` | 角色详情 / Get role |
| PUT | `/api/v1/roles/:id` | 更新角色 / Update role |
| DELETE | `/api/v1/roles/:id` | 删除角色 / Delete role |
| PUT | `/api/v1/roles/:id/permissions` | 分配权限 / Assign permissions |
| GET | `/api/v1/permissions` | 权限列表 / List permissions |

### 商品 Products (需认证 / Auth Required)

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/products` | 创建商品 / Create product (admin) |
| GET | `/api/v1/products` | 商品列表 / List products |
| GET | `/api/v1/products/:id` | 商品详情 / Get product |
| GET | `/api/v1/products/barcode/:barcode` | 条码查询 / Get product by barcode |
| PUT | `/api/v1/products/:id` | 更新商品 / Update product (admin) |
| DELETE | `/api/v1/products/:id` | 删除商品 / Delete product (admin) |

### 标准库存 Inventory (需认证+仓库范围 / Auth + Warehouse Scope)

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/inventory` | 库存列表 / List inventory |
| GET | `/api/v1/inventory/alerts` | 库存预警 / Low stock alerts |
| GET | `/api/v1/inventory/:id` | 库存详情 / Get inventory |
| PUT | `/api/v1/inventory/:id` | 更新库存设置 / Update inventory settings (admin) |

### 非标库存 Non-Std Inventory (需认证+仓库范围+管理员 / Auth + Warehouse Scope + Admin)

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/non-std-inventory` | 创建非标库存 / Create non-std inventory |
| GET | `/api/v1/non-std-inventory` | 非标库存列表 / List non-std inventory |
| GET | `/api/v1/non-std-inventory/:id` | 非标库存详情 / Get non-std inventory |
| PUT | `/api/v1/non-std-inventory/:id` | 更新非标库存 / Update non-std inventory |
| DELETE | `/api/v1/non-std-inventory/:id` | 删除非标库存 / Delete non-std inventory |
| POST | `/api/v1/non-std-inventory/:id/convert` | 非标转标准 / Convert to standard inventory |

### 仓库 Warehouses (需认证 / Auth Required)

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/warehouses` | 创建仓库 / Create warehouse (admin) |
| GET | `/api/v1/warehouses` | 仓库列表 / List warehouses |
| GET | `/api/v1/warehouses/:id` | 仓库详情 / Get warehouse |
| PUT | `/api/v1/warehouses/:id` | 更新仓库 / Update warehouse (admin) |
| DELETE | `/api/v1/warehouses/:id` | 删除仓库 / Delete warehouse (admin) |
| POST | `/api/v1/warehouses/:id/users` | 绑定用户 / Bind users (admin) |
| DELETE | `/api/v1/warehouses/:id/users/:userId` | 解绑用户 / Unbind user (admin) |
| GET | `/api/v1/warehouses/:id/users` | 仓库用户列表 / List warehouse users |

### 单据 Documents (需认证+仓库范围 / Auth + Warehouse Scope)

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/documents` | 创建单据 / Create document |
| GET | `/api/v1/documents` | 单据列表 / List documents |
| GET | `/api/v1/documents/:id` | 单据详情 / Get document detail |
| POST | `/api/v1/documents/:id/complete` | 完成单据 / Complete document |
| POST | `/api/v1/documents/:id/confirm` | 确认盘点差异 / Confirm stocktake (admin) |
| POST | `/api/v1/documents/:id/settle` | 盘点结转 / Settle stocktake (admin) |

### 库存流水 Transactions (需认证+仓库范围 / Auth + Warehouse Scope)

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/transactions` | 库存流水列表 / List inventory transactions |

### 文件附件 Files (需认证 / Auth Required)

> 公开类型 (product_image) 通过 `/uploads/*` 静态路径直接访问；受控类型需经 `/files/:id/download` 代理。
> Public types (product_image) served via `/uploads/*` static path; controlled types must go through `/files/:id/download`.

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/files/upload` | 上传文件 / Upload file (multipart) |
| GET | `/api/v1/files` | 文件列表 / List files |
| GET | `/api/v1/files/:id` | 文件详情 / Get file metadata |
| GET | `/api/v1/files/:id/download` | 下载文件 / Download file |
| DELETE | `/api/v1/files/:id` | 删除文件 (上传人或管理员) / Delete file (uploader or admin) |

### 报表分析 Reports (需认证+仓库范围 / Auth + Warehouse Scope)

> 成本、毛利、库存金额等敏感字段仅管理员可见 / Cost, gross profit, stock value fields are admin-only.

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/reports/sales/stats` | 销售统计 (营收/订单数/SKU数/数量/成本/毛利) / Sales summary |
| GET | `/api/v1/reports/sales/trend` | 销售趋势 (day/week/month) / Sales trend curve |
| GET | `/api/v1/reports/sales/ranking` | 商品销售排行 (qty/amount) / Product ranking |
| GET | `/api/v1/reports/inventory/overview` | 库存概况 / Inventory overview |
| GET | `/api/v1/reports/inventory/turnover` | 库存周转率 / Inventory turnover rate |
| GET | `/api/v1/reports/inventory/slow-moving` | 滞销商品预警 / Slow-moving alerts |

### 审计日志 Audit (需认证+管理员 / Auth + Admin)

> 审计日志为只读查询接口，仅管理员可访问；写入由服务层在业务事务内触发，无对外写接口。
> Audit logs are query-only and admin-only; writes originate from inside the service layer within the business transaction, with no public write endpoint.

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/audit/logs` | 审计日志列表 (按用户/仓库/资源/动作/单据号/时间/结果过滤) / List audit logs (filter by user / warehouse / resource / action / docNo / time / result) |
| GET | `/api/v1/audit/logs/:id` | 审计日志详情 (含 before/after 快照) / Get audit log detail (with before/after snapshot) |

## 统一响应格式 / Unified Response Format

```json
{
  "code": 0,
  "message": "success",
  "data": {},
  "traceId": "a1b2c3d4..."
}
```

### 业务错误码 / Business Error Codes

| 错误码 Code | 说明 Description |
|---|---|
| 0 | 成功 / Success |
| 10001 | 认证失败 / Auth failed |
| 10002 | 权限不足 / Permission denied |
| 10003 | 参数校验失败 / Validation error |
| 10004 | 资源不存在 / Not found |
| 10005 | 重复数据 / Duplicate |
| 20001 | 库存不足 / Insufficient stock |
| 20002 | 状态不允许 / Invalid state |
| 20003 | 重复提交 / Duplicate submission |
| 50000 | 系统异常 / System error |

## 开发命令 / Development Commands

```bash
# 运行测试 / Run tests
cd rims-goProgect && go test ./...

# 编译检查 / Build check
cd rims-goProgect && go build ./...

# 重新生成 Swagger 文档 / Regenerate Swagger docs
cd rims-goProgect && swag init -g cmd/server/main.go -o docs
```

## 环境变量 / Environment Variables

详见 `.env.example` / See `.env.example` for all supported variables.

核心配置 / Key variables:

| 变量 Variable | 说明 Description | 默认值 Default |
|---|---|---|
| `DB_PASSWORD` | 数据库密码 (必填) / DB password (required) | - |
| `JWT_SECRET` | JWT 密钥 (必填) / JWT secret (required) | - |
| `APP_PORT` | 服务端口 / Server port | 8080 |
| `DB_AUTO_MIGRATE` | 自动迁移 / Auto migrate | true |
| `CORS_ORIGINS` | 允许的跨域源 / Allowed origins | * |
| `LOG_LEVEL` | 日志级别 / Log level | info |

## 后续增强 / Follow-up Enhancements

### 文件模块 / File Module
- [ ] 接入对象存储 (MinIO/S3)，替换 `LocalStorage` 实现 / Integrate object storage (MinIO/S3) to replace `LocalStorage`
- [ ] 受控资源下载时增加 `business_id` 关联对象的权限校验 (如单据附件按仓库范围校验) / Add per-resource permission check on download (e.g. scope document attachments by warehouse)
- [ ] 软删对象文件的定时清理任务 / Scheduled cleanup job for soft-deleted object files
- [ ] 文件 hash 去重：上传时命中已有 hash 则复用 `object_key` / File dedup by hash: reuse existing `object_key` on hash match

### 审计模块 / Audit Module

> 当前版本只在 `user.Login` 与 `document.CompleteDocument` 两处试点接入，验证登录安全事件与原子事务回滚两个场景。
> 其余写操作需要在后续 PR 中逐个接入，接入方式：消费方本地定义 `AuditLogger` 接口，构造函数注入 `*audit.AuditService`，在事务内调用 `Log` 并传入 before/after 快照。
> The current release only pilots `user.Login` and `document.CompleteDocument`, covering the login-security and atomic-rollback categories. All other write sites need follow-up PRs — consumer defines a local `AuditLogger` interface, injects `*audit.AuditService` via constructor, and calls `Log` inside the business transaction with before/after snapshots.

待接入的写操作清单 / Pending retrofit sites:

- [ ] **user**: `Create` / `Update` / `Delete` / `ChangePassword` / `ResetPassword`
- [ ] **role & permission**: `Create` / `Update` / `Delete` / `AssignPermissions`
- [ ] **warehouse**: `Create` / `Update` / `Delete` / `BindUser` / `UnbindUser` / `SetDefaultWarehouse` / `SwitchCurrentWarehouse`
- [ ] **product**: `Create` / `Update` / `Delete` (admin-only cost price changes in `details`)
- [ ] **inventory**: `Update` (threshold, status)
- [ ] **non_std_inventory**: `Create` / `Update` / `Delete` / `Convert`
- [ ] **document**: `Create` / `ConfirmStocktake` / `SettleStocktake` / `Complete` 的细分单据类型 (入库/销售/退货/调拨/转换) 单独记录动作
- [ ] **file**: `Upload` / `Delete`

其他增强 / Other enhancements:

- [ ] 保留期与归档策略：按 `created_at` 滚动归档历史审计到冷表，`deleted_at` 配合定时清理 / Retention + archival: roll cold rows off by `created_at`, use `deleted_at` with a scheduled cleanup job
- [ ] `Details` 字段的字段级 diff 辅助函数：避免每个消费方手写 before/after 快照 / Field-diff helper so consumers don't hand-build before/after maps
- [ ] 审计查询导出 (CSV/Excel)，受管理员权限约束 / Audit log export (admin-only CSV/Excel)
- [ ] 更细的异步写路径：对于非关键写操作提供 fire-and-forget 通道，降低热路径延迟 / Optional async path for non-critical writes to keep hot paths fast

## 许可证 / License

AGPL-3.0-or-later. Copyright (c) 2026 ShangBin Wang.
