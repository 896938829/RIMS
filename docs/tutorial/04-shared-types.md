# 第 4 章 · 共享基础设施

> 本章目标：
> 1. 掌握 `internal/types/` 里的 5 个核心文件
> 2. 理解 Go 的**结构体嵌入**（embedding）——替代继承的方式
> 3. 看懂 `AppError` 是怎么做成"可错误类型断言"的
> 4. 熟悉 JWT 服务的最小实现

`internal/types/` 和 `internal/auth/` 是整个项目的**公共词汇表**——任何模块都会用到。花 30 分钟把这一章吃透，后面阅读模块代码会轻松很多。

## 4.1 `base_model.go` · 结构体嵌入（embedding）

打开 [internal/types/base_model.go](../../rims-goProgect/internal/types/base_model.go)：

```go
type BaseModel struct {
    ID        uint           `gorm:"primaryKey" json:"id"`
    CreatedAt time.Time      `json:"createdAt"`
    UpdatedAt time.Time      `json:"updatedAt"`
    DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type AuditableModel struct {
    BaseModel
    CreatedBy uint `gorm:"not null;default:0" json:"createdBy"`
    UpdatedBy uint `gorm:"not null;default:0" json:"updatedBy"`
}
```

### 嵌入 vs 继承

Go 没有继承。上面那段 `AuditableModel` 把 `BaseModel` **作为一个匿名字段**塞进自己——这叫**结构体嵌入**。效果是：

```go
var m AuditableModel
m.ID            // 可以直接访问 BaseModel 的字段，就像它是 AuditableModel 自己的
m.CreatedAt     // 同上
m.BaseModel.ID  // 也可以这么写，等价
```

**嵌入不是继承**——Go 里没有 "AuditableModel 是一个 BaseModel" 的类型关系。它只是**字段和方法的提升**（promotion）。

### 使用方式

其他模块这样用：

```go
// user/model.go
type User struct {
    types.BaseModel          // ← 嵌入，自动得到 ID / CreatedAt / UpdatedAt / DeletedAt
    Username string `gorm:"uniqueIndex"`
    // ...
}

// product/model.go
type Product struct {
    types.AuditableModel     // ← 嵌入更胖的版本，额外带 CreatedBy / UpdatedBy
    Code string
    // ...
}
```

**选型原则**：**系统数据**（用户、角色、权限）用 `BaseModel`；**业务数据**（商品、库存、单据）用 `AuditableModel`，因为业务数据需要记录"谁创建/谁更新"。

### `gorm.DeletedAt` 是软删除

`gorm.DeletedAt` 是 GORM 提供的特殊类型。当你 `db.Delete(&u)` 时，GORM 不会真删记录，而是把 `DeletedAt` 填上当前时间。之后所有 `Find` / `First` 都会自动加上 `WHERE deleted_at IS NULL`。

想真删？用 `Unscoped()`：`db.Unscoped().Delete(&u)`。

## 4.2 `context.go` · 上下文读写的类型安全封装

打开 [internal/types/context.go](../../rims-goProgect/internal/types/context.go)：

```go
const (
    CtxKeyUserID      = "userID"
    CtxKeyUsername    = "username"
    CtxKeyRoleID      = "roleID"
    CtxKeyRoleCode    = "roleCode"
    CtxKeyWarehouseID = "warehouseID"
    CtxKeyTraceID     = "traceID"
)

func GetUserID(c *gin.Context) uint {
    v, _ := c.Get(CtxKeyUserID)
    id, _ := v.(uint)
    return id
}

// 其他 Get* 同理

func IsAdmin(c *gin.Context) bool {
    return GetRoleCode(c) == "admin"
}
```

### 为什么要封装？

JWT 中间件会 `c.Set("userID", claims.UserID)` 把用户 ID 塞进 gin 的上下文。下游任何 handler 都能 `c.Get("userID")` 取出来。问题是：

1. `"userID"` 是**魔法字符串**——拼错了编译器不会报错，只会在运行时拿到 nil。
2. `c.Get()` 返回 `(any, bool)`——每次都要类型断言 `v.(uint)`，代码很啰嗦。

所以项目抽了两层：

- **常量 `CtxKeyUserID`** —— 统一 key，IDE 能自动补全
- **`GetUserID(c)` 函数** —— 封装 `c.Get` + 类型断言，返回干净的 `uint`

**设计要点**：取不到时默认返回零值（`uint` 的零值是 `0`、`string` 的零值是 `""`）。handler 再去判断 `if userID == 0`。这是 Go 的"**避免抛异常，返回零值**"风格。

## 4.3 `errors.go` · AppError 与业务错误码

打开 [internal/types/errors.go](../../rims-goProgect/internal/types/errors.go)：

### 错误码表

```go
const (
    ErrCodeOK                = 0
    ErrCodeAuthFailed        = 10001
    ErrCodePermissionDenied  = 10002
    ErrCodeValidation        = 10003
    ErrCodeNotFound          = 10004
    ErrCodeDuplicate         = 10005
    ErrCodeInsufficientStock = 20001
    ErrCodeInvalidState      = 20002
    ErrCodeDuplicateSubmit   = 20003
    ErrCodeSystemError       = 50000
)
```

**分段约定**：
- `10xxx` → 通用错误（认证、权限、参数、重复）
- `20xxx` → 业务错误（库存不足、状态非法）
- `50xxx` → 系统异常

前端按错误码做国际化或者特定提示，比返回一个 HTTP 400 + 字符串更灵活。

### `AppError` 结构体

```go
type AppError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
    Err     error  `json:"-"`   // 原始错误，不回传给前端
}

func (e *AppError) Error() string {
    if e.Err != nil {
        return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Err)
    }
    return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
    return e.Err
}
```

**三个要点**：

1. **`Error() string` 方法** —— Go 里任何实现了 `Error() string` 方法的类型都是 `error`。`*AppError` 自然满足 `error` 接口。
2. **`Unwrap() error` 方法** —— 让 `errors.Is(someErr, target)` 和 `errors.As(someErr, &appErr)` 能递归探测。
3. **`Err error` 字段带 `json:"-"`** —— 返回给前端时不泄露内部错误详情，但日志里 `fmt.Errorf("...: %w", err)` 仍能看到完整链。

### `HTTPStatus()` 映射

```go
func (e *AppError) HTTPStatus() int {
    switch e.Code {
    case ErrCodeAuthFailed:            return http.StatusUnauthorized     // 401
    case ErrCodePermissionDenied:      return http.StatusForbidden        // 403
    case ErrCodeValidation:            return http.StatusBadRequest       // 400
    case ErrCodeNotFound:              return http.StatusNotFound         // 404
    case ErrCodeDuplicate,
         ErrCodeDuplicateSubmit:       return http.StatusConflict         // 409
    case ErrCodeInsufficientStock,
         ErrCodeInvalidState:          return http.StatusUnprocessableEntity // 422
    default:                           return http.StatusInternalServerError // 500
    }
}
```

**业务错误码 ↔ HTTP 状态码解耦**：业务层不关心 HTTP，handler 层需要 HTTP 时调 `.HTTPStatus()`。

### 构造函数

```go
func ErrAuth(msg string) *AppError { ... }
func ErrForbidden() *AppError { ... }
func ErrValidation(msg string) *AppError { ... }
func ErrNotFound(entity string) *AppError { ... }
func ErrDuplicate(msg string) *AppError { ... }
func ErrInsufficientStock() *AppError { ... }
func ErrInvalidState(msg string) *AppError { ... }
func ErrSystem(err error) *AppError { ... }
```

**风格**：service 层不要手工 `&AppError{Code: ..., Message: ...}`，**一律走构造函数**。这样未来统一改文案或加日志钩子只需改一处。

## 4.4 `response.go` · 统一响应外壳

打开 [internal/types/response.go](../../rims-goProgect/internal/types/response.go)：

```go
type Response struct {
    Code    int         `json:"code"`
    Message string      `json:"message"`
    Data    interface{} `json:"data,omitempty"`
    TraceID string      `json:"traceId,omitempty"`
}

func OK(c *gin.Context, data interface{}) {
    c.JSON(http.StatusOK, Response{
        Code:    ErrCodeOK,
        Message: "success",
        Data:    data,
        TraceID: GetTraceID(c),
    })
}

func OKCreated(c *gin.Context, data interface{}) { /* 201 */ }
func OKWithPage(c *gin.Context, page PageResult) { /* 200 + 分页结构 */ }
func OKNoContent(c *gin.Context)                   { /* 204 */ }

func Fail(c *gin.Context, httpStatus int, appErr *AppError) {
    c.JSON(httpStatus, Response{
        Code:    appErr.Code,
        Message: appErr.Message,
        TraceID: GetTraceID(c),
    })
}

func FailFromError(c *gin.Context, err error) {
    var appErr *AppError
    if errors.As(err, &appErr) {
        Fail(c, appErr.HTTPStatus(), appErr)
        return
    }
    Fail(c, http.StatusInternalServerError, ErrSystem(err))
}
```

### 前端看到的 JSON

成功：
```json
{ "code": 0, "message": "success", "data": { ... }, "traceId": "abc123" }
```

失败：
```json
{ "code": 10004, "message": "用户不存在", "traceId": "abc123" }
```

所有接口都这一个壳，前端写 axios 拦截器时非常省心——看 `code !== 0` 就是错误。

### `FailFromError` · 核心魔法

```go
var appErr *AppError
if errors.As(err, &appErr) {
    Fail(c, appErr.HTTPStatus(), appErr)
    return
}
```

`errors.As` 会**沿着错误链 Unwrap**，找到第一个能赋给 `*AppError` 的节点。不管你的 service 层返回的是原始 `*AppError` 还是被 `fmt.Errorf("xxx: %w", appErr)` 包了几层，这行都能把 `*AppError` 揪出来。

如果整条链都不是 `*AppError`——比如没被转换的原始 `gorm.ErrRecordNotFound`——就兜底返回 500 系统异常。

## 4.5 `pagination.go` · 分页

打开 [internal/types/pagination.go](../../rims-goProgect/internal/types/pagination.go)：

```go
type PageRequest struct {
    Page     int    `form:"page" binding:"omitempty,min=1"`
    PageSize int    `form:"pageSize" binding:"omitempty,min=1,max=100"`
    Sort     string `form:"sort"`
    Keyword  string `form:"keyword"`
}

func (p *PageRequest) Defaults() {
    if p.Page <= 0 { p.Page = 1 }
    if p.PageSize <= 0 { p.PageSize = 20 }
}

func (p *PageRequest) Offset() int {
    p.Defaults()
    return (p.Page - 1) * p.PageSize
}

type PageResult struct {
    List     interface{} `json:"list"`
    Total    int64       `json:"total"`
    Page     int         `json:"page"`
    PageSize int         `json:"pageSize"`
}

func NewPageResult(req PageRequest, list interface{}, total int64) PageResult { ... }
```

**用法三部曲**：

```go
// handler.go
var page types.PageRequest
c.ShouldBindQuery(&page)             // 绑定 ?page=2&pageSize=20
result, _ := svc.List(ctx, page)
types.OKWithPage(c, result)

// service.go
users, total, _ := repo.List(ctx, page)
return types.NewPageResult(page, users, total), nil

// repository.go
page.Defaults()
d.Count(&total)                      // 先数总数
d.Offset(page.Offset()).Limit(page.PageSize).Find(&users)  // 再取当前页
```

**`binding:"min=1,max=100"`** 由 Gin 调 go-playground/validator 校验。`PageSize=200` 会直接返回 400。

## 4.6 `internal/auth/jwt.go` · JWT 服务

打开 [internal/auth/jwt.go](../../rims-goProgect/internal/auth/jwt.go)：

```go
type Claims struct {
    UserID   uint   `json:"uid"`
    Username string `json:"username"`
    RoleID   uint   `json:"rid"`
    RoleCode string `json:"role"`
    jwt.RegisteredClaims   // ← 嵌入库自带的 iat/exp 等标准字段
}

type TokenService struct {
    secretKey []byte
    expireDur time.Duration
}

func NewTokenService(secret string, expireHours int) *TokenService { ... }

func (s *TokenService) GenerateToken(userID uint, username string, roleID uint, roleCode string) (string, int64, error) {
    claims := Claims{
        UserID: userID, Username: username, RoleID: roleID, RoleCode: roleCode,
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.expireDur)),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
        },
    }
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString(s.secretKey)
}

func (s *TokenService) ParseToken(tokenString string) (*Claims, error) {
    token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
        if t.Method != jwt.SigningMethodHS256 {
            return nil, fmt.Errorf("unexpected signing method: %s", t.Method.Alg())
        }
        return s.secretKey, nil
    })
    // ...
    return claims, nil
}
```

### 三个要点

1. **HMAC-SHA256** (`HS256`) 对称签名——签和验用同一个密钥 `secretKey`。部署时 `JWT_SECRET` 是必填。
2. **`ParseWithClaims` 的回调** 是一个**函数作为参数**的写法（Go 里函数是一等公民）。里面先确认签名算法没被篡改，再返回密钥。
3. **`Claims` 嵌入 `jwt.RegisteredClaims`** —— 又是结构体嵌入，复用库的标准字段（`exp`, `iat` 等），自己只加业务字段（`uid`, `rid`, ...）。

### Token 长这样

```
eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1aWQiOjEsInJvbGUiOiJhZG1pbiIsImV4cCI6MTcxMDAwMDAwMH0.xxxxx
```

三段 base64 用 `.` 分隔：header / payload / signature。payload 里就是你的 `Claims`。**base64 不是加密**——任何人都能解出 payload 看到 `uid`、`role`。签名只防篡改。所以**不要把敏感信息写进 Claims**。

## 4.7 动手试试

1. 打开 [types/errors.go](../../rims-goProgect/internal/types/errors.go)，给自己写一个 `ErrRateLimit(msg string) *AppError`，错误码 `30001`，HTTP 429。**注意**：要同时改 `HTTPStatus()` 方法。
2. 给 `BaseModel` 加一个新字段 `Version int` 用作乐观锁（不用真改，写在纸上想清楚：它会影响哪些文件？答案：`BaseModel` 所在文件，**不需**要改任何模块代码，因为所有 model 都嵌入了 `BaseModel`——这就是嵌入的威力）。

---

上一章 ← [03-配置与数据库](./03-config-db.md) | 下一章 → [05-中间件链](./05-middleware.md)
