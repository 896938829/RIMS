# 第 3 章 · 配置与数据库

> 本章目标：
> 1. 看懂 Viper 如何把 `.env` + 环境变量合并成一个 `Config` 结构体
> 2. 理解 GORM 的 DSN 与 `gorm.Open` 的最小用法
> 3. 知道 "12-factor app" 的配置原则及项目是怎么落地的

## 3.1 Config 结构体是"仪表板"

打开 [internal/config/config.go](../../rims-goProgect/internal/config/config.go)。`Config` 就是一块扁平结构：

```go
type Config struct {
    AppEnv  string
    AppPort string

    // Database
    DBHost        string
    DBPort        string
    DBUser        string
    DBPassword    string
    DBName        string
    DBSSLMode     string
    DBAutoMigrate bool

    // JWT
    JWTSecret      string
    JWTExpireHours int

    // File upload
    UploadDir   string
    MaxUploadMB int
    AllowedExts string

    // ... Log / HTTP server / CORS
}
```

**为什么不用全局变量 `os.Getenv("DB_HOST")` 随取随用？**

把所有配置集中到一个结构体，然后把它作为参数传给需要的人（`db.New(cfg)`、`buildRouter(cfg, ...)`），好处：

- **可测试**：测试时构造一个假的 `Config{}` 传进去即可，不用改环境变量
- **可发现**：IDE 里按 `.` 就能列出所有配置项
- **类型安全**：`JWTExpireHours int` 比 `strconv.Atoi(os.Getenv("JWT_EXPIRE_HOURS"))` 安全多了

这就是**依赖注入**最朴素的形式——把依赖做成参数而不是全局变量。

## 3.2 `config.Load()` 里发生了什么

```go
func Load() (Config, error) {
    v := viper.New()
    v.SetConfigType("env")      // 把 .env 当成 key=value 格式
    v.SetConfigName(".env")
    v.AddConfigPath(".")        // 优先搜当前目录
    v.AddConfigPath("..")       // 再搜父目录
    v.AutomaticEnv()            // 允许环境变量覆盖
    v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
    _ = v.ReadInConfig()        // 找不到 .env 不报错（env 变量还在）

    // 默认值
    v.SetDefault("APP_PORT", "8080")
    v.SetDefault("DB_HOST", "127.0.0.1")
    // ...

    cfg := Config{
        AppPort:    v.GetString("APP_PORT"),
        DBPassword: v.GetString("DB_PASSWORD"),
        // ...
    }

    // 必填校验
    if cfg.DBPassword == "" {
        return Config{}, fmt.Errorf("DB_PASSWORD is required")
    }
    if cfg.JWTSecret == "" {
        return Config{}, fmt.Errorf("JWT_SECRET is required")
    }
    return cfg, nil
}
```

### 优先级顺序

Viper 按"**环境变量 > .env 文件 > 默认值**"的顺序取值。也就是：

```mermaid
flowchart LR
    A[代码里 SetDefault] --> B[.env 文件覆盖]
    B --> C[环境变量覆盖]
    C --> D[cfg.GetString]
```

举例：
- 代码里 `v.SetDefault("APP_PORT", "8080")`
- `.env` 里 `APP_PORT=9000` → 实际值 `9000`
- Shell 里 `export APP_PORT=9090` → 实际值 `9090`

这就是 **12-factor app** 的 "Store config in the environment" 原则——部署环境通过环境变量覆盖，不用改代码。

### `SetEnvKeyReplacer` 做什么

`v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))` 告诉 Viper：访问嵌套 key 时，把点换成下划线。比如 `v.GetString("db.host")` 会读环境变量 `DB_HOST`。本项目 key 都是扁平的（`DB_HOST`），这行其实可写可不写，但留着是 Viper 的惯用法。

## 3.3 `.env` 长什么样

项目根目录已经有一份 `.env`（早期开发期提交到仓库是项目的刻意决策，**生产环境必须换**）：

```
DB_PASSWORD=dev_password
JWT_SECRET=dev-secret-change-me-in-prod
DB_AUTO_MIGRATE=true
# ... 其余都用 Load() 里的默认值
```

**注意**：真实项目里 `.env` 应放到 `.gitignore`。本项目因为还在极早期、只有开发凭据，刻意提交便于新人一键起跑。生产部署时通过 CI/CD 或 k8s secret 注入环境变量覆盖。

## 3.4 从 `Config` 到 `*gorm.DB`

打开 [internal/db/db.go](../../rims-goProgect/internal/db/db.go)：

```go
func BuildDSN(cfg config.Config) string {
    return fmt.Sprintf(
        "host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
        cfg.DBHost, cfg.DBPort, cfg.DBUser,
        cfg.DBPassword, cfg.DBName, cfg.DBSSLMode,
    )
}

func New(cfg config.Config) (*gorm.DB, error) {
    dsn := BuildDSN(cfg)
    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
    if err != nil {
        return nil, fmt.Errorf("connect postgres: %w", err)
    }
    return db, nil
}
```

### DSN 是什么

DSN = **Data Source Name**，告诉驱动连哪个库。PostgreSQL 的 DSN 支持两种风格：

```
// 键值对风格（本项目用的）
host=127.0.0.1 port=5432 user=app password=xxx dbname=appdb sslmode=disable

// URL 风格（等价）
postgres://app:xxx@127.0.0.1:5432/appdb?sslmode=disable
```

### `gorm.Open` 返回什么

`*gorm.DB` 是 GORM 的"数据库句柄"。整个项目所有 repo 都持有它。但它**不是一个连接**——底下是标准库 `database/sql` 的连接池，GORM 按需从池中借出连接。

### 为什么 `gorm.Config{}` 是空的

这里我们没设置任何 GORM 选项。常见可配置项：

- `NamingStrategy` — 自定义表名/列名规则（本项目用 GORM 默认的 snake_case + 复数）
- `Logger` — 自定义 SQL 日志格式
- `PrepareStmt` — 是否预编译 SQL 语句

初学阶段用默认配置就够了。

## 3.5 GORM 的 `TableName()` 约定

默认情况下，GORM 把结构体名转换成下划线、复数当表名：

```go
type User struct { ... }
// 默认表名 → "users"

type NonStdInventory struct { ... }
// 默认表名 → "non_std_inventories"  ← 注意是复数！
```

本项目为了明确化，每个 model 都显式写了：

```go
func (User) TableName() string { return "users" }
```

这是 GORM 识别的特殊方法。**参数是值接收者**（`(User)` 而不是 `(*User)` 或 `(u User)`），因为 GORM 反射调用时不依赖实例字段。

## 3.6 常见疑问

**Q: 配置文件改了要重启服务吗？**
A: 要。Viper 支持 `WatchConfig()` 热重载，但本项目没启用。生产实践里配置热更的边界 case 太多，一般不建议。

**Q: `.env` 里可以写多行字符串吗？**
A: 可以用引号包起来：`CORS_ORIGINS="http://a.com, http://b.com"`。

**Q: 为什么 `DB_PASSWORD` 和 `JWT_SECRET` 是必填校验，其他不是？**
A: 因为其他项都有合理默认值，这两个是"没默认值的秘密"——漏填意味着有安全漏洞，必须把进程崩在启动时而不是运行时。

## 3.7 动手试试

1. 把 `.env` 里的 `DB_PASSWORD=...` 删掉，重新 `go run ./cmd/server`。期待看到：
   ```
   server exited with error: load config: DB_PASSWORD is required
   exit status 1
   ```
2. 恢复密码，改 `APP_PORT=9090`，重启。看看服务是否监听在 9090。
3. 不改 `.env`，直接 `APP_PORT=9091 go run ./cmd/server`（环境变量临时覆盖）。观察是不是跑在 9091。

---

上一章 ← [02-启动流程](./02-bootstrap.md) | 下一章 → [04-共享基础设施](./04-shared-types.md)
