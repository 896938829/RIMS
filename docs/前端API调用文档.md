# RIMS 前端 API 调用文档

本文档基于 rims-py-validator 验证器实跑通过的全部 17 个业务流程（截至 2026-04-17）反向整理，所有端点均经过黑盒冒烟 + 权限探测 + 边界错误验证。

- 后端语言: Go 1.25 + Gin
- 基础路径: `http://<host>:8080/api/v1`
- Swagger UI: `http://<host>:8080/swagger/index.html`
- 静态文件: `http://<host>:8080/uploads/<year>/<month>/<hash>.<ext>`（仅 public 类型）
- 健康检查: `GET /healthz` → `{ "status": "ok" }`

---

## 1. 通用约定

### 1.1 鉴权

除 `POST /auth/login`、`GET /uploads/*`、`GET /swagger/*`、`GET /healthz` 外，所有接口需在请求头携带 JWT：

```
Authorization: Bearer <token>
```

Token 通过登录接口获取，过期时间由服务端 `JWT_EXPIRE_HOURS` 配置。

### 1.2 仓库隔离

以下路由组经过 `WarehouseScope` 中间件，需提供仓库上下文：

- `/inventory/**`、`/non-std-inventory/**`
- `/documents/**`、`/transactions`
- `/reports/**`

指定方式（优先级从高到低）：

1. 请求头 `X-Warehouse-ID: <id>`
2. 若未传，使用当前用户的默认仓库
3. 若用户未绑定任何仓库，返回 `403 请先选择仓库`

`/users`、`/warehouses`、`/roles`、`/permissions`、`/products`、`/files`、`/audit/**` **不**经过 WarehouseScope。

### 1.3 响应信封

**所有接口统一返回下列结构**（204 除外）：

```json
{
  "code": 0,
  "message": "success",
  "data": <业务数据>,
  "traceId": "1cc3bbc831b30dca6464ae498dfbd79c"
}
```

- `code=0` 表示业务成功；非 0 时 `data` 通常为 null，`message` 含用户可读错误文本。
- `traceId` 可在服务端 `audit_logs` 中反查全链路。

### 1.4 分页

列表接口统一使用查询参数：

| 参数 | 类型 | 默认 | 说明 |
|---|---|---|---|
| `page` | int | 1 | 页码，从 1 起 |
| `pageSize` | int | 20 | 每页条数，上限 100 |
| `keyword` | string | - | 仅部分接口支持，按 code/no 模糊匹配 |

分页响应：

```json
{
  "list": [ ... ],
  "total": 123,
  "page": 1,
  "pageSize": 20
}
```

### 1.5 业务错误码

| code | HTTP | 含义 | 触发场景 |
|---|---|---|---|
| 0 | 200/201/204 | success | 正常 |
| 10001 | 401 | 认证失败 | token 缺失/过期/无效 |
| 10002 | 403 | 权限不足 | 非 admin 访问 admin 接口；跨仓访问 |
| 10003 | 400 | 参数校验失败 | body/query 字段不合法 |
| 10004 | 404 | 资源不存在 | ID 不存在或已软删除 |
| 10005 | 409 | 资源重复 | 唯一键冲突（如 username、product.code） |
| 20001 | 422 | 库存不足 | 销售/调拨出库超出现有库存 |
| 20002 | 422 | 状态不合法 | 对已完成单据再次 complete、有库存商品删除等 |
| 20003 | 409 | 幂等/重复提交 | 预留 |
| 50000 | 500 | 系统异常 | DB/IO 故障 |

### 1.6 角色权限矩阵

| 操作 | admin | user（普通） |
|---|---|---|
| 登录 / 查看自己信息 / 改自己密码 / 切仓库 | ✅ | ✅ |
| 查看 / 创建 / 完成 销售单、退货单 | ✅ | ✅ |
| 浏览商品、条码查询、标准库存、报表（屏蔽成本字段） | ✅ | ✅ |
| 用户/角色/权限 CRUD、重置他人密码 | ✅ | ❌ 403 |
| 仓库 CRUD、绑定用户 | ✅ | ❌ 403 |
| 商品 CRUD、查看 costPrice | ✅ | ❌ 403（写）/ 屏蔽（读） |
| 非标库存 CRUD + 转标 | ✅ | ❌ 403 |
| 入库单、调拨单、盘点单、非标转换单 create/complete | ✅ | ❌ 403 |
| 审计日志查询 | ✅ | ❌ 403 |
| 成本价、利润、库存总值、costPrice 字段 | ✅ 可见 | 自动屏蔽（字段不返回） |
| 下载他人私有文件 | ✅ | ❌ 403（仅上传者/admin） |

---

## 2. 认证模块

### 2.1 登录 `POST /auth/login`

**公开**，请求体：

```json
{ "username": "admin", "password": "admin123" }
```

响应 `data`：

```json
{
  "token": "eyJhbGciOi...",
  "expiresAt": 1713456000,
  "user": {
    "id": 1,
    "username": "admin",
    "realName": "系统管理员",
    "roleCode": "admin",
    "roleName": "管理员"
  }
}
```

登录成功/失败均落审计日志。错误：`10001` 用户名或密码错误。

---

## 3. 用户 / 角色 / 权限

> 写操作**全部 admin-only**。非 admin 访问返回 `10002 / 403`。

### 3.1 用户

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| POST | `/users` | admin | 创建用户 |
| GET | `/users` | auth | 分页列表（支持 `keyword`） |
| GET | `/users/:id` | auth | 用户详情 |
| PUT | `/users/:id` | admin | 更新 realName/phone/email/roleId/status |
| DELETE | `/users/:id` | admin | 软删除 |
| GET | `/users/me` | auth | 当前用户信息 |
| PUT | `/users/me/password` | auth | 修改自己密码（需 `oldPassword` + `newPassword`） |
| PUT | `/users/:id/password` | admin | 管理员重置他人密码 |
| GET | `/users/me/warehouses` | auth | 当前用户可见仓库列表 |
| PUT | `/users/me/warehouses/default` | auth | 设置默认仓库 |
| PUT | `/users/me/warehouses/current` | auth | 切换当前仓库（响应头影响后续 `X-Warehouse-ID` 解析） |

**CreateUserRequest**

```json
{
  "username": "alice",           // 3-64
  "password": "Pwd@12345",       // 6-72
  "realName": "张三",             // 可选，<=64
  "phone": "13800000000",        // 可选，<=20
  "email": "a@b.com",            // 可选，email 格式
  "roleId": 2                    // 必填
}
```

**UpdateUserRequest**（所有字段可选，PATCH 语义）

```json
{ "realName": "新名", "phone": "...", "email": "...", "roleId": 3, "status": 1 }
```

**UserResponse**

```json
{
  "id": 2, "username": "alice", "realName": "张三",
  "phone": "...", "email": "...",
  "roleId": 2, "roleCode": "user", "roleName": "普通用户",
  "status": 1,
  "createdAt": "2026-04-17T06:00:00Z", "updatedAt": "..."
}
```

### 3.2 角色

| 方法 | 路径 | 权限 |
|---|---|---|
| POST | `/roles` | admin |
| GET | `/roles` | auth（返回全部，无分页） |
| GET | `/roles/:id` | auth |
| PUT | `/roles/:id` | admin |
| DELETE | `/roles/:id` | admin |
| PUT | `/roles/:id/permissions` | admin — `{ "permissionIds": [1,2,3] }` |

### 3.3 权限

| 方法 | 路径 | 权限 |
|---|---|---|
| GET | `/permissions` | auth — 返回全部权限清单 |

---

## 4. 仓库模块

> 仓库管理 / 绑定用户为 **admin-only**；查询接口普通用户可见但只返回自己有权限的仓库。

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| POST | `/warehouses` | admin | 创建仓库 |
| GET | `/warehouses` | auth | 分页列表（admin 全量，普通用户仅绑定仓库） |
| GET | `/warehouses/:id` | auth | 仓库详情 |
| PUT | `/warehouses/:id` | admin | 更新 |
| DELETE | `/warehouses/:id` | admin | 软删除（存在业务数据时拒绝） |
| POST | `/warehouses/:id/users` | admin | 绑定用户 — `{ "userIds": [2,3] }` |
| GET | `/warehouses/:id/users` | admin | 仓库绑定用户列表 |
| DELETE | `/warehouses/:id/users/:userId` | admin | 解绑单个用户 |

**CreateWarehouseRequest**

```json
{
  "code": "WH002",                 // 必填 2-32
  "name": "上海仓",                 // 必填 <=128
  "status": 1,                     // 可选 0/1
  "address": "...", "contactPerson": "...", "contactPhone": "..."
}
```

---

## 5. 商品 / 库存

### 5.1 商品 `/products`

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| POST | `/products` | admin | 创建商品 |
| GET | `/products` | auth | 分页（keyword 匹配 code/name/barcode） |
| GET | `/products/:id` | auth | 详情。**admin 返回 `costPrice`，普通用户字段缺失** |
| PUT | `/products/:id` | admin | 更新 |
| DELETE | `/products/:id` | admin | 软删。**有库存/流水时返回 20002** |
| GET | `/products/barcode/:barcode` | auth | 按条码查询，404 = 未找到 |

**CreateProductRequest**

```json
{
  "code": "SKU001",        // 必填
  "name": "商品A",          // 必填
  "unit": "件",             // 必填
  "category": "日用品",
  "spec": "500ml",
  "barcode": "6901234567890",
  "retailPrice": 50.0,
  "costPrice": 25.0,       // admin 下发
  "imageUrl": "",
  "status": 1
}
```

### 5.2 标准库存 `/inventory`（warehouse-scoped）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/inventory` | 分页列表（当前仓库） |
| GET | `/inventory/alerts` | 低于 `alertThreshold` 的预警列表 |
| GET | `/inventory/:id` | 单条详情 |
| PUT | `/inventory/:id` | 设置 `alertThreshold` / `status`（数量由单据驱动，无法直接改） |

**库存数量只能通过单据流转改变**，业务层严禁直接加减 `quantity` 字段。

### 5.3 非标库存 `/non-std-inventory`（warehouse-scoped，admin-only）

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/non-std-inventory` | 创建一条非标记录（装箱/集合/暂存） |
| GET | `/non-std-inventory` | 分页列表 |
| GET | `/non-std-inventory/:id` | 详情（含 `remainingQty`） |
| PUT | `/non-std-inventory/:id` | 更新描述 / 数量 / 状态 |
| DELETE | `/non-std-inventory/:id` | 软删 |
| POST | `/non-std-inventory/:id/convert` | 转标：`{ "productId": <id>, "quantity": <n> }`。部分转换 status=2，全转 status=3 |

状态值：`0=禁用, 1=可用, 2=部分转换, 3=已转完`。

---

## 6. 业务单据

> 所有单据路由经过 **warehouse-scoped**。文档明细行 `costPrice` 对非 admin **自动屏蔽为 0**（GetDocument 侧处理）。

### 6.1 单据类型

| docType | 名称 | 创建权限 | complete 权限 | 号前缀 |
|---|---|---|---|---|
| 1 | 入库单 | admin | admin | `RK` |
| 2 | 销售单 | 全员 | 全员 | `XS` |
| 3 | 退货单 | 全员 | 全员 | `TH` |
| 4 | 调拨单 | admin | admin | `DB` |
| 5 | 盘点单 | admin | 特殊流程 | `PD` |
| 6 | 非标转换单 | admin | admin | `ZH` |

状态：普通单据 `1=草稿, 2=已完成`；盘点单 `1=盘点中 → 2=差异已确认 → 3=已结转`。

### 6.2 端点

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/documents` | 创建草稿单据 |
| GET | `/documents` | 分页列表，可按 `docType` 过滤 |
| GET | `/documents/:id` | 详情（含 `lines` 数组；非 admin 屏蔽 costPrice） |
| POST | `/documents/:id/complete` | 完成单据（执行库存变更），事务内写审计 |
| POST | `/documents/:id/confirm` | **admin**，盘点：`盘点中 → 差异已确认` |
| POST | `/documents/:id/settle` | **admin**，盘点：`差异已确认 → 已结转`（应用库存差异） |
| GET | `/transactions` | 当前仓库库存流水（支持 `keyword=docNo`） |

### 6.3 CreateDocumentRequest

```json
{
  "docType": 1,                    // 必填 1-6
  "toWarehouseId": 0,              // 调拨单必填，其他 0
  "refDocId": 0,                   // 退货单必填：原销售单 ID
  "remark": "备注",
  "lines": [
    {
      "productId": 10,             // 盘点/转换 可填
      "nonStdInvId": 0,            // 转换单必填：源非标库存 ID
      "quantity": 5,                // 非盘点单必须 >0
      "costPrice": 0,
      "retailPrice": 0,
      "actualQty": 0,               // 仅盘点单：实盘数（系统会自动生成 systemQty/diffQty）
      "remark": ""
    }
  ]
}
```

### 6.4 典型流程示例

**入库** (admin-only)：

1. `POST /documents` `docType=1, lines=[{productId, quantity:10}]` → 得到 draft id
2. `POST /documents/:id/complete` → 204；库存 +10，写一条 `direction=1` 流水

**销售** (全员)：

1. `POST /documents` `docType=2, lines=[{productId, quantity:3, retailPrice}]`
2. `POST /documents/:id/complete`；库存不足 → `20001`

**退货** (全员)：

1. 必须 `refDocId` 指向状态为已完成的销售单，否则 `20002`
2. 同商品不能超退原销售量（超出 → `10003`）

**调拨** (admin-only)：

1. `toWarehouseId != warehouseId`，源仓出库、目标仓入库在单事务内完成
2. 源仓库存不足 → `20001`

**盘点** (admin-only, 三阶段)：

1. `POST /documents` `docType=5` 进入"盘点中"
2. `POST /documents/:id/confirm` → "差异已确认"
3. `POST /documents/:id/settle` → "已结转"，将 `diffQty` 应用到各商品库存

**非标转换** (admin-only)：走 `/non-std-inventory/:id/convert` 或 `docType=6` 单据，择一即可。

### 6.5 常见错误

| 场景 | code | HTTP |
|---|---|---|
| 非 admin 创建/完成 admin-only 单据 | 10002 | 403 |
| 重复 complete 已完成单据 | 20002 | 422 |
| 空 `lines` 或 `quantity<=0` | 10003 | 400 |
| 销售/调拨超库存 | 20001 | 422 |
| 退货引用未完成销售单 | 20002 | 422 |

---

## 7. 报表

> Warehouse-scoped，均为 GET。所有时间范围上限 366 天，超出返回 `10003`。**cost/profit/totalValue 字段 admin-only**（非 admin 响应中字段缺失）。

| 方法 | 路径 | 查询参数 | 说明 |
|---|---|---|---|
| GET | `/reports/sales/stats` | `startDate` `endDate` (YYYY-MM-DD) | 销售汇总：revenue/orderCount/skuCount/quantity/[costAmount/grossProfit] |
| GET | `/reports/sales/trend` | +`bucket=day|week|month` | 按时间粒度聚合，返回 `list: [{period, revenue, orderCount, quantity}]` |
| GET | `/reports/sales/ranking` | +`metric=qty|amount` +`limit<=100` | 商品销售排行 |
| GET | `/reports/inventory/overview` | - | skuCount/totalQty/lowStockCount/[totalValue] |
| GET | `/reports/inventory/turnover` | `startDate` `endDate` `limit` | 周转率排行 |
| GET | `/reports/inventory/slow-moving` | `startDate` `endDate` `maxSales` `page` `pageSize` | 滞销清单 |

非法的 `bucket` / `metric` 值会被白名单拦截（防 SQL 注入），返回 `10003`。

---

## 8. 文件附件

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| POST | `/files/upload` | auth | `multipart/form-data`: `file`(必填) + `businessType` + `businessId` |
| GET | `/files` | auth | 列表，filter `businessType` / `businessId` |
| GET | `/files/:id` | auth | 元数据详情（admin 可见 `objectKey`） |
| GET | `/files/:id/download` | auth | **私有文件仅上传者或 admin 可下载（403 其他）** |
| DELETE | `/files/:id` | 上传者或 admin | 软删，对象保留 |

### 8.1 businessType

| 值 | IsPublic | URL 策略 |
|---|---|---|
| `product_image` | ✅ | `/uploads/<objectKey>` 静态访问 |
| `doc_attachment` | ❌ | `/api/v1/files/:id/download` 代理 |
| `import_template` | ❌ | 同上 |
| `export_result` | ❌ | 同上 |
| `other` | ❌ | 同上 |

### 8.2 限制

- 服务端根据 `MAX_UPLOAD_MB` 拦截超大文件（默认 10MB）
- 扩展名白名单由 `ALLOWED_EXTS` 配置，非法返回 `10003`
- MIME 通过前 512 字节嗅探，非扩展名驱动
- SHA-256 作为 `fileHash` 写入元数据（暂未启用去重）

### 8.3 FileResponse

```json
{
  "id": 42,
  "businessType": "doc_attachment",
  "businessId": 101,
  "fileUrl": "/api/v1/files/42/download",
  "originalName": "合同.pdf",
  "fileSize": 10240,
  "mimeType": "application/pdf",
  "fileHash": "e3b0c44...",
  "isPublic": false,
  "objectKey": "2026/04/abc.pdf",      // admin-only
  "createdBy": 7,
  "uploadedAt": "2026-04-17T06:00:00Z"
}
```

---

## 9. 审计日志（admin-only）

| 方法 | 路径 | 查询参数 |
|---|---|---|
| GET | `/audit/logs` | `userId` `warehouseId` `resource` `resourceId` `action` `docNo` `result` `startTime`(RFC3339 或 date) `endTime` `keyword` `page` `pageSize` |
| GET | `/audit/logs/:id` | - |

**时间窗口上限 366 天**，超出 `10003`。

**AuditLogResponse** 关键字段：

```json
{
  "id": 9001,
  "traceId": "1cc3bbc8...",
  "userId": 2, "username": "alice", "roleCode": "user",
  "warehouseId": 1,
  "action": "complete",
  "resource": "document",
  "resourceId": 88,
  "docNo": "XS20260417001",
  "description": "完成单据 XS20260417001",
  "details": { "before": {...}, "after": {...} },
  "ipAddress": "127.0.0.1",
  "userAgent": "curl/8.0.1",
  "result": "success",
  "errorCode": 0,
  "errorMsg": "",
  "createdAt": "..."
}
```

审计当前已集成：登录（best-effort）、`/documents/:id/complete`（事务内原子落库）。其他写接口作为待补齐项。

---

## 10. 前端集成建议

1. **全局拦截**：
   - 401 `code=10001` → 跳登录页，清本地 token
   - 403 `code=10002` → 提示"无权限"并回退
   - 422 `code=20001/20002` → 业务弹窗，不要当崩溃
   - 500 `code=50000` → 附上 `traceId` 供运维排查

2. **仓库上下文**：登录成功后调一次 `GET /users/me/warehouses` 缓存可见仓库；每次请求附 `X-Warehouse-ID`，用户主动切换时调 `PUT /users/me/warehouses/current`。

3. **成本价/利润字段**：普通用户响应里这些字段会**自动缺失**（不是 null），前端只需判断 `typeof field === 'undefined'`。

4. **单据完成是幂等守卫在服务端**：重复点击 "完成" 按钮不会重复扣库存，但会返回 `20002`；前端可乐观锁禁用按钮即可。

5. **文件下载**：私有文件直接用 `<a href="/api/v1/files/:id/download">`，记得把 Authorization 头附上（可用 blob fetch）。

6. **分页**：后端 `pageSize` 硬上限 100，前端滚动加载 page 递增即可。

7. **错误信道**：所有业务错误走统一 `code/message`，HTTP 状态只是二级分类，不要仅根据 HTTP 状态做分支。

---

## 11. 已知后端行为（验证器实测）

- **GET /warehouses** 普通用户仅返回其绑定仓库，不会穿透
- **DELETE /products/:id** 存在库存/流水时返回 `20002`，不是 500
- **POST /documents/:id/complete** 在事务内写审计，如审计落库失败整单回滚
- **盘点单** `settle` 执行时若库存已变化或 `diffQty` 会导致负库存，返回 `20002`，需重新盘点后再结转
- **退货单** 超退会按 `原数量 - 已退数量` 精确校验
- **私有文件 ACL** 已在 handler 预检（非 uploader/admin 直接 403，不会开启流）
- **审计时间窗** 与报表时间窗口一致，都是 366 天硬上限

---

_文档来源：rims-py-validator 全量通过的 17 个业务流程（smoke / user / warehouse×2 / product×2 / inventory×2 / document×5 / report×2 / file / audit），以及 rims-goProgect/internal/modules 下的实际 DTO 定义。_
