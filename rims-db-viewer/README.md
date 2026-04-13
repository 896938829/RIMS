# RIMS DB Viewer

轻量级数据库查看工具，用于 RIMS 项目开发调试。基于 Flask 的 Web 界面，直连 PostgreSQL，提供表浏览、结构查看、SQL 查询和数据导出功能。

## 快速启动

### 前置条件

- Python 3.10+
- PostgreSQL 正在运行（通过 `deploy/docker-compose.yml` 启动）
- 项目根目录存在 `.env` 文件（含数据库连接配置）

### 安装与运行

```bash
# 1. 确保 PostgreSQL 已启动
cd <项目根目录>
docker compose -f deploy/docker-compose.yml up -d

# 2. 安装依赖
cd rims-db-viewer
pip install -r requirements.txt

# 3. 启动服务
python app.py
```

浏览器打开 **http://127.0.0.1:5001**

Windows 和 WSL 均可运行。数据库连接信息自动从项目根目录的 `.env` 文件读取。

## 功能说明

### 表列表（首页）

访问 `/` 查看数据库中所有表及预估行数。点击表名进入数据浏览页面。

### 表数据浏览

访问 `/table/<表名>` 浏览表数据，支持：

- **分页** — 每页 50 行，底部翻页导航
- **筛选** — 每列提供输入框，支持模糊匹配（不区分大小写）
- **排序** — 点击列头切换升序/降序
- **导出** — 右上角「Export CSV」按钮，导出当前筛选条件下的数据

筛选和排序条件通过 URL 参数传递，翻页时自动保持。

### 表结构

访问 `/table/<表名>/structure` 查看：

- 列定义：列名、数据类型、是否可空、默认值、字符长度
- 索引列表：索引名称及定义

### SQL 查询

访问 `/query` 执行自定义 SQL 查询：

- 仅允许 `SELECT` 和 `WITH` 语句（其他语句会被拒绝）
- 查询超时限制 10 秒
- 页面显示最多 1000 行结果
- 支持将查询结果导出为 CSV（最多 10000 行）
- 显示查询执行耗时

### CSV 导出

两种导出方式：

| 场景 | 入口 | 行数上限 |
|------|------|----------|
| 表数据导出 | 表浏览页右上角「Export CSV」 | 10,000 行 |
| 查询结果导出 | SQL 查询页「Export CSV」按钮 | 10,000 行 |

## 路由一览

| 方法 | 路径 | 功能 |
|------|------|------|
| GET | `/` | 表列表 |
| GET | `/table/<name>` | 表数据浏览 |
| GET | `/table/<name>/structure` | 表结构 |
| GET | `/table/<name>/export` | 导出表数据 CSV |
| GET | `/query` | SQL 查询页面 |
| POST | `/query` | 执行 SQL 查询 |
| POST | `/query/export` | 导出查询结果 CSV |

## 配置

程序从项目根目录（`rims-db-viewer/` 的上级目录）的 `.env` 文件读取以下配置：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `DB_HOST` | 127.0.0.1 | 数据库地址 |
| `DB_PORT` | 5432 | 数据库端口 |
| `DB_USER` | app | 数据库用户 |
| `DB_PASSWORD` | （必填） | 数据库密码 |
| `DB_NAME` | appdb | 数据库名 |
| `DB_SSLMODE` | disable | SSL 模式 |

## 安全说明

- SQL 查询仅允许只读 `SELECT`/`WITH` 语句
- 查询执行超时 10 秒
- 表名参数经过白名单校验，防止 SQL 注入
- 结果行数有上限（浏览 1000 行，导出 10000 行）
- 默认监听 `0.0.0.0:5001`，仅供本地开发使用

## 技术栈

- **Flask 3.x** — Web 框架（服务端渲染）
- **Jinja2 + Bootstrap 5** — 页面模板（CDN 加载）
- **psycopg2** — PostgreSQL 驱动（连接池）
- **python-dotenv** — 环境变量加载

## 目录结构

```
rims-db-viewer/
├── app.py              # Flask 入口 + 所有路由
├── db.py               # PostgreSQL 连接池 + 查询函数
├── requirements.txt    # Python 依赖
├── templates/
│   ├── base.html       # 基础布局
│   ├── index.html      # 表列表
│   ├── table.html      # 表数据浏览
│   ├── structure.html  # 表结构
│   └── query.html      # SQL 查询
└── static/
    └── style.css       # 自定义样式
```

## License

AGPL-3.0-or-later — see [LICENSE](../LICENSE)
