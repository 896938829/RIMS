# 第 5 章 · 中间件链

> 本章目标：
> 1. 理解 Gin 中间件本质上就是一个闭包
> 2. 走一遍本项目的 6 层中间件（Recovery、RequestID、Logger、CORS、JWTAuth、WarehouseScope）
> 3. 看懂中间件和业务 handler 共享数据的标准做法（`c.Set` / `c.Get`）

## 5.1 什么是 Gin 中间件

Gin 的中间件就是一个函数，签名：

```go
type HandlerFunc func(c *gin.Context)
```

这和普通 handler 的签名**一模一样**——Gin 内部把中间件和 handler 串成一个切片，一个个执行。区别在约定：

- **中间件**：通常调 `c.Next()` 让后面的处理继续；可以 `c.Abort()` 提前终止
- **handler**：通常在链末尾，不需要 `c.Next()`

## 5.2 项目的中间件全貌

打开 [internal/app/router.go](../../rims-goProgect/internal/app/router.go) 的 `buildRouter`：

```go
r := gin.New()
r.Use(gin.Recovery())                       // ① 兜底 panic
r.Use(middleware.RequestID())               // ② 请求唯一 ID
r.Use(middleware.Logger())                  // ③ 访问日志
r.Use(middleware.CORS(cfg.CORSOrigins))     // ④ 跨域

// 业务路由组里再叠
users := rg.Group("/users")
users.Use(authMw)                           // ⑤ JWT 鉴权
// warehouseScope 只用在需要仓库作用域的路由组
products.Use(authMw, whScope)               // ⑤+⑥
```

对应流程图：

```mermaid
flowchart LR
    Req[HTTP 请求] --> Rec[gin.Recovery]
    Rec --> RID[RequestID]
    RID --> Log[Logger]
    Log --> Cors[CORS]
    Cors -->|公开| Public[/healthz<br>/auth/login]
    Cors -->|需登录| JWT[JWTAuth]
    JWT -->|需选仓| WH[WarehouseScope]
    JWT --> H1[Handler]
    WH --> H2[Handler]
```

每层都可以：

- 在请求进来时做预处理（`c.Set(...)`）
- 调 `c.Next()` 把控制权交给后面
- `c.Next()` 返回后做后处理（例如 Logger 是在 `c.Next()` 之后计算 latency）

## 5.3 `gin.Recovery()` · 兜底 panic

Gin 标准库自带，不在项目 middleware/ 下。作用：**任何 handler 里 panic 时，它会用 `recover()` 捕获、打日志、返回 500**，让进程不崩。

所有 Web 服务的**第一个中间件**都应该是 Recovery。

## 5.4 `RequestID` · 请求追踪

打开 [internal/middleware/requestid.go](../../rims-goProgect/internal/middleware/requestid.go)：

```go
func RequestID() gin.HandlerFunc {
    return func(c *gin.Context) {
        id := c.GetHeader("X-Request-ID")
        if id == "" {
            id = generateID()  // 16 字节随机 hex
        }
        c.Set(types.CtxKeyTraceID, id)
        c.Header("X-Request-ID", id)
        c.Next()
    }
}
```

**做三件事**：

1. 如果上游（如网关、前端）传了 `X-Request-ID`，就复用；否则自己生成 16 字节随机 hex
2. 写进 gin 上下文（后面 Logger 和 Response 会读）
3. 回写到响应 header（前端可以拿到 trace ID 反馈问题）

### 外层函数的意义

注意 `RequestID()` 是一个**返回 HandlerFunc 的函数**，而不是 `func RequestID(c *gin.Context)`。为什么多套一层？

- 这种写法允许传**配置参数**：看 `middleware.CORS(cfg.CORSOrigins)`，它在外层收参数，返回带参数的闭包。
- 所有中间件工厂都统一成 `func() gin.HandlerFunc` 风格，形成约定。
- 闭包里可以缓存一些只需计算一次的东西（例如 CORS 里的 `allowedOrigins` 切片）。

这是 **Go 里的一等函数 + 闭包**的典型用法。

## 5.5 `Logger` · 访问日志

打开 [internal/middleware/logger.go](../../rims-goProgect/internal/middleware/logger.go)：

```go
func Logger() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()

        c.Next()  // ← 把控制权交给后面的中间件 + handler

        latency := time.Since(start)
        log.Printf("[%s] %s %s | %d | %v | user=%d",
            types.GetTraceID(c),
            c.Request.Method,
            c.Request.URL.Path,
            c.Writer.Status(),
            latency,
            types.GetUserID(c),
        )
    }
}
```

**关键点**：`c.Next()` 是同步的。它里面会执行**后续所有中间件和 handler**，然后才返回。所以 `c.Next()` 下面的代码都是"后处理"。

输出样子：

```
[abc123def456] GET /api/v1/users | 200 | 12.3ms | user=1
```

## 5.6 `CORS` · 跨域

打开 [internal/middleware/cors.go](../../rims-goProgect/internal/middleware/cors.go)。逻辑：

1. 启动时把 `CORS_ORIGINS="http://a.com,http://b.com"` 按逗号切成数组
2. 请求来时比对 `Origin` header
3. 如果命中（或配置了 `*`），放行并写 CORS 响应头
4. `OPTIONS` 预检请求直接 204

生产环境**不要**用 `*`——给具体域名清单更安全。

## 5.7 `JWTAuth` · 鉴权

打开 [internal/middleware/jwt.go](../../rims-goProgect/internal/middleware/jwt.go)：

```go
func JWTAuth(tokenSvc *auth.TokenService) gin.HandlerFunc {
    return func(c *gin.Context) {
        authHeader := c.GetHeader("Authorization")
        if authHeader == "" {
            types.Fail(c, 401, types.ErrAuth("缺少认证头"))
            c.Abort()
            return
        }

        parts := strings.SplitN(authHeader, " ", 2)
        if len(parts) != 2 || parts[0] != "Bearer" || parts[1] == "" {
            types.Fail(c, 401, types.ErrAuth("认证头格式错误"))
            c.Abort()
            return
        }

        claims, err := tokenSvc.ParseToken(parts[1])
        if err != nil {
            types.Fail(c, 401, types.ErrAuth("无效的令牌"))
            c.Abort()
            return
        }

        c.Set(types.CtxKeyUserID, claims.UserID)
        c.Set(types.CtxKeyUsername, claims.Username)
        c.Set(types.CtxKeyRoleID, claims.RoleID)
        c.Set(types.CtxKeyRoleCode, claims.RoleCode)
        c.Next()
    }
}
```

**做三件事**：

1. 校验 `Authorization: Bearer xxxxx` 格式
2. 调 `TokenService.ParseToken` 验签、解 Claims
3. 把用户身份塞进 gin 上下文 → 后续 handler 能用 `types.GetUserID(c)` 取

### 失败路径一定要 `c.Abort()`

`c.Abort()` 把 gin 的 handler 链切断，后续中间件和 handler 都不会执行。**但 `return` 不等于 `c.Abort()`**——它只退出当前中间件，Gin 还会调用后面的。所以失败时要既 `types.Fail(...)` 写响应、又 `c.Abort()` 切链、最后 `return` 退函数。三连组合是 Go Web 开发里非常常见的模式，新手容易漏掉 `c.Abort()`。

### 依赖注入方式

```go
authMw := middleware.JWTAuth(tokenSvc)
```

`tokenSvc` 是在 `router.go` 里创建的——中间件不直接读 `config.JWTSecret`，而是接受一个已经构造好的服务。这样测试时可以传**假的** `tokenSvc`。

## 5.8 `WarehouseScope` · 仓库作用域

打开 [internal/middleware/warehouse.go](../../rims-goProgect/internal/middleware/warehouse.go)：

```go
type WarehouseAccess interface {
    GetDefaultWarehouseID(ctx context.Context, userID uint) (uint, error)
    HasAccess(ctx context.Context, userID, warehouseID uint) (bool, error)
}

func WarehouseScope(checker WarehouseAccess) gin.HandlerFunc {
    return func(c *gin.Context) {
        userID := types.GetUserID(c)
        if userID == 0 {
            types.Fail(c, 401, types.ErrAuth("未认证")); c.Abort(); return
        }

        warehouseID := parseWarehouseHeader(c)
        if warehouseID == 0 {
            defaultID, err := checker.GetDefaultWarehouseID(c.Request.Context(), userID)
            if err != nil || defaultID == 0 {
                types.Fail(c, 403, types.ErrValidation("请先选择仓库")); c.Abort(); return
            }
            warehouseID = defaultID
        }

        ok, err := checker.HasAccess(c.Request.Context(), userID, warehouseID)
        if err != nil || !ok {
            types.Fail(c, 403, types.ErrForbidden()); c.Abort(); return
        }

        c.Set(types.CtxKeyWarehouseID, warehouseID)
        c.Next()
    }
}
```

### 四步解析

1. **读 header `X-Warehouse-ID`** —— 前端切换仓库时带上这个头
2. **回落到用户默认仓库** —— 如果没带头，就查这个用户绑定的默认仓库
3. **权限校验** —— 这个用户有没有访问这个仓库的权限（管理员可访问多仓，普通用户只能访问自己的那个）
4. **写进上下文** —— 下游 handler 用 `types.GetWarehouseID(c)` 取值

### 消费者定义的接口

注意 `WarehouseAccess` 是**在 middleware 包里定义**的，不是在 warehouse 模块定义的。这叫**消费者定义接口**（Consumer-Defined Interface）——"我需要什么方法，我自己声明"。

`warehouse.UserWarehouseRepository` 结构性地满足了这个接口，所以 `router.go` 里这行能直接传进去：

```go
whScope := middleware.WarehouseScope(userWarehouseRepo)
```

这是 Go **结构性类型系统**的强大之处——不用像 Java 那样显式 `implements`。**第 9 章**会专门讲这一模式。

## 5.9 路由组里的中间件叠加

```go
api := r.Group("/api/v1")
user.RegisterRoutes(api, userHandler, authMw)         // 只要 JWT
product.RegisterRoutes(api, productHandler, authMw, whScope)  // JWT + 仓库作用域
```

模块内部又把 authMw 挂到更细的路由组：

```go
// user/routes.go
auth := rg.Group("/auth")
auth.POST("/login", handler.Login)                    // 公开

users := rg.Group("/users")
users.Use(authMw)                                     // 组级别挂中间件
users.POST("", handler.CreateUser)
users.GET("", handler.ListUsers)
```

`Group().Use()` 是**组级中间件**——对该组内所有路由都生效。这样 `/api/v1/auth/login` 不需要 JWT，而 `/api/v1/users/*` 都需要。

## 5.10 中间件写入 gin 上下文的 key 一览

| Key 常量 | 写入方 | 读取方 |
|---|---|---|
| `CtxKeyTraceID` | RequestID 中间件 | Logger、`types.OK/Fail` 写响应 |
| `CtxKeyUserID` | JWTAuth 中间件 | 所有 protected handler |
| `CtxKeyUsername` | JWTAuth 中间件 | audit 模块 |
| `CtxKeyRoleID` / `CtxKeyRoleCode` | JWTAuth 中间件 | `types.IsAdmin(c)` |
| `CtxKeyWarehouseID` | WarehouseScope 中间件 | inventory / document / report handler |

## 5.11 常见疑问

**Q: `c.Set` 存的值是全局的还是请求级的？**
A: 请求级。每个请求有自己的 `*gin.Context`，互不干扰。

**Q: 中间件里能读请求 body 吗？**
A: 可以，但只能读一次（`c.Request.Body` 是 `io.ReadCloser`）。如果要让后面 handler 还能读，需要 `c.Request.Body = io.NopCloser(bytes.NewBuffer(data))` 重置。本项目没这个需求。

**Q: 为什么 `WarehouseScope` 不写成全局中间件？**
A: 因为 `/auth/login`、`/users/me` 这些接口根本不需要仓库概念。全局挂会导致登录都要先选仓库，死锁。

## 5.12 动手试试

1. 不带 `Authorization` 头请求 `/api/v1/users/me`：
   ```bash
   curl -i http://127.0.0.1:8080/api/v1/users/me
   ```
   应该看到 401 + `{"code":10001,"message":"缺少认证头",...}`。

2. 带上错误格式的 header：
   ```bash
   curl -i -H "Authorization: Token abc" http://127.0.0.1:8080/api/v1/users/me
   ```
   期待 401 `"认证头格式错误"`。

3. 在 `Logger` 里增加一行 `log.Printf("user-agent=%s", c.Request.UserAgent())`，重启服务观察日志。

---

上一章 ← [04-共享基础设施](./04-shared-types.md) | 下一章 → [06-六文件模块模式 ★](./06-module-pattern.md)
