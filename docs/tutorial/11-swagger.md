# 第 11 章 · Swagger 文档

> 本章目标：
> 1. 学会读和写 swaggo 注解
> 2. 理解**泛型外壳** `types.Response{data=XxxResponse}` 的语法
> 3. 掌握重新生成 Swagger 的命令

## 11.1 Swagger 是什么

Swagger（现在叫 OpenAPI）是一份**机器可读的 API 描述**。写得好，前端可以自动生成 TypeScript 客户端、Postman 可以导入、测试可以跑自动化。

本项目用 [swaggo/swag](https://github.com/swaggo/swag) —— 通过**扫描 Go 注释**生成 `docs/swagger.json` 和 `docs/swagger.yaml`。然后 [swaggo/gin-swagger](https://github.com/swaggo/gin-swagger) 在 `/swagger/index.html` 挂一个 UI 界面供人浏览。

## 11.2 两级注解

### 全局注解 · `internal/app/app.go`

打开 [internal/app/app.go:21-27](../../rims-goProgect/internal/app/app.go#L21-L27)：

```go
// @title RIMS API
// @version 1.0
// @description 零售端库存管理系统 API
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
```

| 注解 | 含义 |
|---|---|
| `@title` / `@version` / `@description` | API 基本元数据 |
| `@BasePath` | 所有路径的前缀（这里是 `/`） |
| `@securityDefinitions.apikey BearerAuth` | 声明一种鉴权方式，名字叫 `BearerAuth` |
| `@in header` / `@name Authorization` | 这种鉴权通过 `Authorization` header 传 |

### 方法级注解 · handler 函数

每个 handler 头部写：

```go
// Login godoc                             ← 规矩：函数名 + " godoc"
// @Summary 用户登录                        ← 一句话说明
// @Tags 认证                               ← 分组标签（前端自动归类）
// @Accept json                             ← 请求体 MIME
// @Produce json                            ← 响应体 MIME
// @Param payload body LoginRequest true "登录凭证"   ← 参数
// @Success 200 {object} types.Response{data=LoginResponse}
// @Failure 401 {object} types.Response
// @Router /api/v1/auth/login [post]
func (h *Handler) Login(c *gin.Context) { ... }
```

## 11.3 `@Param` 五种位置

```
@Param <name> <in> <type> <required> "<description>" <extra>
```

`in` 可以是：

| in | 来源 | 例子 |
|---|---|---|
| `path` | URL 路径段 | `@Param id path int true "用户ID"` |
| `query` | URL 查询串 | `@Param page query int false "页码" default(1)` |
| `header` | 请求头 | `@Param X-Warehouse-ID header int false "仓库ID"` |
| `body` | 请求体（JSON） | `@Param payload body LoginRequest true "登录凭证"` |
| `formData` | 表单 / 文件上传 | `@Param file formData file true "文件"` |

## 11.4 `@Success` / `@Failure` 语法

```
@Success <httpStatus> {<containerType>} <dataType> "<description>"
```

- `containerType`: `object`（单个）、`array`（数组）
- `dataType`: Go 类型名（带包前缀）

### 常见写法

```go
// 单个对象
@Success 200 {object} UserResponse

// 数组
@Success 200 {array} RoleResponse

// 包在 types.Response 里
@Success 200 {object} types.Response{data=UserResponse}

// data 是数组
@Success 200 {object} types.Response{data=[]PermissionResponse}

// data 是分页结构
@Success 200 {object} types.Response{data=types.PageResult}

// 只要响应壳，不关心 data
@Failure 404 {object} types.Response
```

## 11.5 泛型外壳 `{data=XxxResponse}` —— 本项目的核心技巧

项目所有 API 的响应都统一包在 `types.Response` 里：

```go
type Response struct {
    Code    int         `json:"code"`
    Message string      `json:"message"`
    Data    interface{} `json:"data,omitempty"`   // ← 前端关心的实际内容
    TraceID string      `json:"traceId,omitempty"`
}
```

问题：`Data` 是 `interface{}` —— Swagger 不知道里面是什么。前端看文档只看到 `data: {}` 空对象，没用。

解法：swaggo 支持**模板语法**覆盖字段：

```go
@Success 200 {object} types.Response{data=LoginResponse}
```

意思是："**响应体是 `types.Response`，但它的 `data` 字段是 `LoginResponse` 类型**"。Swagger 生成的 schema 就会把 `data` 标成正确类型，前端看到的文档是：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "token": "string",
    "expiresAt": 0,
    "user": { "id": 0, "username": "string", ... }
  },
  "traceId": "string"
}
```

### 要让泛型生效，生成命令必须带 `--parseDependency`

```bash
swag init -g internal/app/app.go --parseDependency --parseInternal -o docs
```

- `--parseDependency`：解析被 import 的第三方和本项目包
- `--parseInternal`：**必须**——因为业务类型都在 `internal/` 下，默认 swag 会跳过

## 11.6 `@Security` · 需要鉴权的接口

```go
// @Security BearerAuth
```

引用的是全局 `@securityDefinitions.apikey BearerAuth`。Swagger UI 里这些接口前会显示一个🔒图标，用户可以点 "Authorize" 输入 token 后在 UI 里试接口。

公开接口（如 `/auth/login`）不写这行。

## 11.7 重新生成命令 · 常见翻车

记在 [CLAUDE.md](../../CLAUDE.md)：

```bash
cd rims-goProgect
swag init -g internal/app/app.go --parseDependency --parseInternal -o docs
```

### 为什么是 `-g internal/app/app.go` 而不是 `cmd/server/main.go`？

全局注解（`@title`、`@BasePath` 等）写在 `internal/app/app.go` 里不是 `main.go` 里。`-g` 告诉 swag "从哪个文件开始解析全局注解"。如果指错，生成的 `swagger.json` 里 title 就是默认的 "Swagger Example API"。

### 翻车场景速查

| 症状 | 原因 |
|---|---|
| `data` 字段是空 `{}` | 漏了 `--parseDependency` 或 `--parseInternal` |
| 类型 `LoginResponse` 找不到 | 漏了 `--parseInternal`（类型在 `internal/`） |
| 新加的 handler 没出现 | 注释里 `// @Router` 路径拼错了；或忘记 `swag init` 重新生成 |
| 生成后 `/swagger/index.html` 404 | `go.mod` 里 swagger 依赖可能没装 |

## 11.8 本地访问 Swagger UI

启动服务后：

```
http://127.0.0.1:8080/swagger/index.html
```

界面上可以：

- 按 Tag 折叠查看所有接口
- 点 "Authorize" 输入 Bearer token
- 点接口 → "Try it out" → 直接发请求，看真实响应

对前端同学和自测都很友好。

## 11.9 swaggo 注解速查表

| 注解 | 位置 | 用途 |
|---|---|---|
| `@title` | 全局 | API 标题 |
| `@version` | 全局 | 版本号 |
| `@description` | 全局 | 描述 |
| `@BasePath` | 全局 | 路径前缀 |
| `@securityDefinitions.apikey` | 全局 | 声明鉴权类型 |
| `@Summary` | 方法 | 一句话标题 |
| `@Description` | 方法 | 详细描述（可多行） |
| `@Tags` | 方法 | 分组（中文也可） |
| `@Accept` / `@Produce` | 方法 | MIME |
| `@Param` | 方法 | 入参 |
| `@Success` / `@Failure` | 方法 | 出参 |
| `@Router` | 方法 | 路径 + HTTP 方法 |
| `@Security` | 方法 | 引用全局鉴权 |

## 11.10 动手试试

1. 打开 Swagger UI，找到 `POST /api/v1/auth/login`，直接试一发登录。把返回的 token 复制到右上角 "Authorize"。
2. 再找 `GET /api/v1/users/me`，直接 "Try it out"，应该看到当前用户信息。
3. 给 `Handler.Login` 加一行 `@Failure 429 {object} types.Response` 然后重新 `swag init`，观察 `docs/swagger.json` 里对应段落是否多了 429。
4. 如果你前一章练习里加了 `ErrRateLimit`，把它加到某个 handler 的注解里，重新生成。

---

上一章 ← [10-错误处理](./10-error-handling.md) | 下一章 → [12-动手练习](./12-exercises.md)
