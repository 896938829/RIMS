# 第 12 章 · 动手练习

> 把学到的东西变成肌肉记忆。每题带参考答案，做完再看。

两档难度：

- **L1 读懂题** · 不改代码，只需在项目里定位 / 推理
- **L2 小改动题** · 改 1~3 个文件，能编译、能跑

（按协商结果，本教程**不含 L3 新模块题**。学有余力可自行按[第 6 章 6.10 节](./06-module-pattern.md) checklist 挑战"新增 supplier 模块"。）

---

## L1 · 读懂题

### L1-1 · 画出创建仓库的调用栈

任务：用户发一个 `POST /api/v1/warehouses` 请求创建仓库。写出从 HTTP 进来到 DB 落库的完整调用栈（中间件 + handler + service + repo）。

要求：标出每一步发生在哪个文件、大约在哪一行。

<details>
<summary>参考答案</summary>

```
HTTP POST /api/v1/warehouses
│
├─ gin.Recovery()                                 ← router.go:32
├─ middleware.RequestID()                         ← requestid.go:17
├─ middleware.Logger()                            ← logger.go:16
├─ middleware.CORS(cfg.CORSOrigins)               ← cors.go:13
│
├─ api := r.Group("/api/v1")                      ← router.go:117
├─ warehouse.RegisterRoutes(api, handler, authMw) ← warehouse/routes.go:Register
├─ POST /warehouses → handler.CreateWarehouse
│
├─ middleware.JWTAuth(tokenSvc)                   ← jwt.go:16（由 routes.go 挂在 /warehouses 组上）
│
├─ warehouse.Handler.CreateWarehouse              ← warehouse/handler.go
│    ├─ types.IsAdmin(c) 权限校验
│    ├─ c.ShouldBindJSON(&req)
│    └─ warehouseSvc.Create(ctx, req)             ← warehouse/service.go
│         └─ warehouseRepo.Create(ctx, &w)        ← warehouse/repository.go
│              └─ db.FromCtx(ctx, gormDB).Create(&w)
│                   └─ INSERT INTO warehouses ... ← PostgreSQL
│
└─ types.OKCreated(c, resp)                       ← response.go:32
```

几个要点：
- `/warehouses` 经过了 `authMw`，没走 `WarehouseScope`（创建仓库的人还没有"所属仓库"概念）。
- 因为是纯新建，**没有**事务——所有 repo 调用共享 ctx 但没 tx。
- 管理员检查在 handler 里（`types.IsAdmin(c)`），不在 service。

</details>

---

### L1-2 · 找出没用 `db.FromCtx` 的 repo 方法

任务：通读所有模块的 `repository.go`，找一处直接用 `r.gormDB` 而不是 `r.getDB(ctx)` 的 repo 方法。如果全都用了 `getDB`，说明为什么这是好现象。

<details>
<summary>参考答案</summary>

**做法**：`Grep` 一下：

```bash
# 在项目里搜 "gormDB." 看哪些不是在 getDB 里
rg "gormDB\." rims-goProgect/internal/modules --type go
```

预期：搜到的几乎都是 `r.getDB(ctx).XXX` 里的 `r.gormDB`（作为 `FromCtx` 的 fallback 参数），以及 `NewXxxRepository(gormDB *gorm.DB) { ... r.gormDB = gormDB }` 的构造函数。**没有**裸 `r.gormDB.Create()` 直接调 GORM 的情况。

**为什么这是好现象**：

- 如果某方法用 `r.gormDB.Where(...)` 而不是 `r.getDB(ctx).Where(...)`，就**绕过了事务**——该方法被包在 `RunInTx` 里时，它的 SQL 会在事务**外**执行，破坏原子性。
- 项目严格约定"所有 repo 方法第一行 `d := r.getDB(ctx)`"或直接链式 `r.getDB(ctx).Xxx()`，code review 会把这当必查项。
- 作为新人，加方法时**照抄邻居**即可——不需要自己推导。

</details>

---

### L1-3 · `UpdateUserRequest` 里为什么全是指针

任务：打开 [user/dto.go](../../rims-goProgect/internal/modules/user/dto.go) 看到 `UpdateUserRequest` 所有字段都是 `*string` / `*uint` / `*int8`。说出用指针和不用指针的差别。

<details>
<summary>参考答案</summary>

- 不用指针：前端不传 `realName` 字段时，Go 里 `req.RealName == ""`——你无法区分"前端没传"和"前端明确传了空字符串"。
- 用指针：前端不传时 `req.RealName == nil`；传 `""` 时 `req.RealName != nil && *req.RealName == ""`。**区分出来了**。

这对 PATCH 语义至关重要：你只想更新用户明确指定的字段，没传的要**保持原值**。service 代码：

```go
if req.RealName != nil {
    u.RealName = strings.TrimSpace(*req.RealName)
}
```

只有非 nil 才去改，nil 跳过。

</details>

---

### L1-4 · `document.Complete` 里每一处 `txCtx` 必要性

任务：打开 [document/service.go Complete](../../rims-goProgect/internal/modules/document/service.go#L179)。数一下闭包里用到 `txCtx` 的地方。如果某一处误写成外层 `ctx`，会发生什么？

<details>
<summary>参考答案</summary>

闭包里每次 repo 调用都传 `txCtx`：

1. `s.docRepo.GetByID(txCtx, id)` — 只读，即使漏传影响小（但会脱离事务连接）
2. `s.lineRepo.ListByDocumentID(txCtx, doc.ID)` — 只读同上
3. `s.executeXxx(txCtx, ...)` — 里面会 update 库存，**必须**是 tx
4. `s.docRepo.Update(txCtx, doc)` — update documents，**必须**是 tx
5. `s.audit.Log(txCtx, ...)` — insert audit_logs，**必须**是 tx

如果第 4 处误写 `ctx`：单据状态更新会**立即 commit**（因为非事务连接）；随后如果第 5 处审计写入失败 → 整个 `RunInTx` 返回 error → tx 回滚 → **但已经 commit 的单据状态改不回去了**。这就造成"单据已完成 / 审计未记录"的幽灵状态，违反合规要求。

所以练习的结论：**事务里的所有 DB 调用必须用同一个 `txCtx`**。这件事靠人工 review 保证，编译器不会帮你。

</details>

---

### L1-5 · `user.Handler` 为什么要依赖 `audit.AuditLogger` 接口？

任务：解释为什么 `user.Handler` 自己定义了一个 `AuditLogger` 接口，而不是直接依赖 `*audit.AuditService`。

<details>
<summary>参考答案</summary>

主要三点（详见[第 9 章](./09-cross-cutting.md)）：

1. **防循环依赖**：如果 audit 模块将来也想依赖 user 模块（比如做用户行为统计），双向依赖会导致 import cycle 编译错误。各自定义接口切断了这条。
2. **接口最小化**：user handler 只调 `Log` 一个方法，接口就只声明这一个，不引入 `AuditRepository` 那一摞无关方法。
3. **易测试**：单元测试 `user.Handler` 时可以造一个 `fakeAudit struct{}` 实现 `Log` 方法即可，不需要拖出整个 audit 模块的依赖链。

这在 Go 社区称 **Consumer-Defined Interface** 或 "Accept interfaces, return structs"。

</details>

---

## L2 · 小改动题

### L2-1 · 给 `user.CreateUser` 加审计日志

任务：模仿 `user.Login` 里的 `auditLogin` 助手，给创建用户的接口也加上审计。要求：成功失败都记。

**提示**：
- 在 `user/handler.go` 里加一个 `auditCreateUser` 方法，仿照 `auditLogin`
- Action 用 `audit.ActionCreate`（自查 audit 包里的常量）
- Resource 用 `audit.ResourceUser`
- 在 `CreateUser` handler 里调

<details>
<summary>参考答案</summary>

```go
// 在 handler.go 里加

func (h *Handler) auditCreateUser(c *gin.Context, req CreateUserRequest, resp *UserResponse, createErr error) {
    entry := audit.Entry{
        Actor: audit.Actor{
            UserID:    types.GetUserID(c),
            Username:  types.GetUsername(c),
            RoleCode:  types.GetRoleCode(c),
            TraceID:   types.GetTraceID(c),
            IPAddress: c.ClientIP(),
            UserAgent: c.Request.UserAgent(),
        },
        Action:      audit.ActionCreate,
        Resource:    audit.ResourceUser,
        Description: "创建用户 " + req.Username,
    }
    if createErr != nil {
        entry.Result = audit.ResultFailure
        var appErr *types.AppError
        if errors.As(createErr, &appErr) {
            entry.ErrorCode = appErr.Code
            entry.ErrorMsg = appErr.Message
        } else {
            entry.ErrorMsg = createErr.Error()
        }
    } else if resp != nil {
        entry.Result = audit.ResultSuccess
        newID := resp.ID
        entry.ResourceID = &newID
        entry.After = map[string]any{
            "username": resp.Username,
            "roleCode": resp.RoleCode,
        }
    }
    _ = h.auditSvc.Log(c.Request.Context(), entry)
}

// 然后改 CreateUser：
func (h *Handler) CreateUser(c *gin.Context) {
    if !types.IsAdmin(c) { types.FailFromError(c, types.ErrForbidden()); return }
    var req CreateUserRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
        return
    }
    resp, err := h.userSvc.Create(c.Request.Context(), req)

    h.auditCreateUser(c, req, resp, err)   // ← 新增这行

    if err != nil { types.FailFromError(c, err); return }
    types.OKCreated(c, resp)
}
```

**注意事项**：
- best-effort（`_ =`），因为创建用户**不在事务里**，审计失败回滚不了
- 不要把 `req.Password` 写进审计！密码字段要过滤
- 确认 audit 包里 `ActionCreate` 和 `ResourceUser` 常量存在（若不存在需要先在 audit 包里定义）

</details>

---

### L2-2 · 在 `types/response.go` 加 `OKAccepted`（202）

任务：HTTP 202 `Accepted` 常用于"已接收、处理中"的异步场景。照 `OKCreated` 的样子加一个 `OKAccepted(c, data)`。

<details>
<summary>参考答案</summary>

改 [types/response.go](../../rims-goProgect/internal/types/response.go)：

```go
// OKAccepted sends a 202 response with data, for async operations
// that have been accepted but not yet completed.
func OKAccepted(c *gin.Context, data interface{}) {
    c.JSON(http.StatusAccepted, Response{
        Code:    ErrCodeOK,
        Message: "accepted",
        Data:    data,
        TraceID: GetTraceID(c),
    })
}
```

**验证**：
```bash
cd rims-goProgect
go build ./...    # 应该无报错
```

（因为没人调用它，编译是唯一检查。真用时会在某个异步 handler 调用它。）

</details>

---

### L2-3 · 给用户列表 API 加"仅管理员"保护

任务：目前 [user/handler.go ListUsers](../../rims-goProgect/internal/modules/user/handler.go#L143) 没有 `types.IsAdmin(c)` 检查，普通用户也能看所有用户。加上这道关。

<details>
<summary>参考答案</summary>

在 `ListUsers` 开头加：

```go
func (h *Handler) ListUsers(c *gin.Context) {
    if !types.IsAdmin(c) {
        types.FailFromError(c, types.ErrForbidden())
        return
    }
    var page types.PageRequest
    // ... 原逻辑
}
```

**思考题**：为什么这种检查写在 handler 里而不是中间件？

项目约定：**rbac 更适合在 handler 做，因为同一个路由上不同 HTTP 方法权限可能不同（GET 允许、POST 不允许），中间件层次表达不够精细。**

（如果你想系统化权限控制，可以将来加一个 `RequireAdmin()` 中间件作为语法糖——但本项目目前还没到这个规模。）

</details>

---

### L2-4 · 给 `PageRequest` 加默认最大 `PageSize`

任务：目前 `PageRequest.PageSize` 被 `binding:"min=1,max=100"` 限在 100。但 `Defaults()` 里没有对 `PageSize > 100` 的兜底——其实 binding 会先于 `Defaults` 失败，这里是为了防御式编程。改 `Defaults`，让它把 `> 100` 的值也截断为 100。

<details>
<summary>参考答案</summary>

改 [types/pagination.go](../../rims-goProgect/internal/types/pagination.go)：

```go
func (p *PageRequest) Defaults() {
    if p.Page <= 0 {
        p.Page = 1
    }
    if p.PageSize <= 0 {
        p.PageSize = 20
    }
    if p.PageSize > 100 {   // ← 新增
        p.PageSize = 100
    }
}
```

**验证**：

```bash
cd rims-goProgect
go build ./...
```

**延伸思考**：既然 binding 已经限 `max=100`，这次改动有意义吗？

有。考虑 service 被**非 handler 场景**调用（比如内部任务、CLI 工具），这些场景不走 Gin 的 binding。保留 `Defaults()` 兜底，service 永远拿到合法值。

</details>

---

## 完成之后

恭喜！你现在应该：

- 能在这个项目里快速定位任何功能的实现位置
- 看懂任何新加的模块
- 有能力按六文件模板扩展自己的模块
- 理解项目中反复出现的 Go 模式（接口定义、错误处理、事务传播、上下文）

**下一步建议**：

1. 动手新增一个 `supplier` 模块（按[第 6 章 6.10 节](./06-module-pattern.md) checklist），从 migration 到 Swagger 全跑通。
2. 读一读 `document` 模块的 `executeSales` / `executeInbound` 等内部方法，看更复杂的事务怎么组织。
3. 看 `report` 模块——它没有 `model.go`（只读不写），演示了另一种模块形态。
4. 熟悉完项目后，反过头去看官方 [Effective Go](https://go.dev/doc/effective_go) 和 [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)——你会发现本项目的很多约定就是出自这两篇。

---

上一章 ← [11-Swagger 文档](./11-swagger.md) | 回首页 → [00-目录](./00-index.md)
