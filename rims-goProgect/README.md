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
│  │   → JWTAuth → Warehouse → Permission          │  │
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
│       ├── warehouse/              # 🔲 仓库与权限 / Warehouse & Permission
│       ├── product/                # 🔲 商品与库存 / Product & Inventory
│       ├── document/               # 🔲 单据与流水 / Documents & Transactions
│       ├── report/                 # 🔲 报表分析 / Reports & Analytics
│       ├── file/                   # 🔲 文件附件 / File & Attachment
│       └── audit/                  # 🔲 审计日志 / Audit & Log
├── migrations/                     # SQL 迁移脚本 / SQL migrations
├── docs/                           # Swagger 生成文件 / Generated Swagger docs
├── deploy/                         # Docker Compose 等 / Deployment configs
└── go.mod
```

> ✅ 已实现 / Implemented　　🔲 待实现 / Planned

## 模块说明 / Module Details

| 模块 Module | 职责 Responsibility | 状态 Status |
|---|---|---|
| **user** 用户认证 | 登录、用户 CRUD、角色权限管理 / Login, user CRUD, role & permission management | ✅ |
| **warehouse** 仓库权限 | 仓库管理、用户仓库绑定、仓库范围校验 / Warehouse CRUD, user binding, scope validation | 🔲 |
| **product** 商品库存 | 商品档案、标准/非标库存、库存预警 / Product catalog, standard/non-standard inventory, alerts | 🔲 |
| **document** 单据流水 | 入库/销售/退货/调拨/盘点/转换单 / Inbound, sales, return, transfer, stocktaking, conversion orders | 🔲 |
| **report** 报表分析 | 销售统计、库存分析、排行趋势 / Sales stats, inventory analysis, rankings & trends | 🔲 |
| **file** 文件附件 | 图片上传、附件管理 / Image upload, attachment management | 🔲 |
| **audit** 审计日志 | 操作审计、日志查询 / Operation audit, log queries | 🔲 |

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

## 许可证 / License

AGPL-3.0-or-later. Copyright (c) 2026 ShangBin Wang.
