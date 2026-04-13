# rims-db-viewer 设计文档

## Context

RIMS 项目开发过程中需要直接查看 PostgreSQL 数据库内容，用于调试和数据验证。现有 Go 后端没有数据库浏览功能，pgAdmin 等工具过于重量级。需要一个轻量的、与项目集成的数据库查看工具。

## 概览

独立 Python Flask Web 服务，直连 PostgreSQL，提供表浏览、表结构查看、自定义 SQL 查询、筛选过滤、排序和 CSV 导出功能。仅供本地开发使用。

## 技术栈

- Python 3.10+
- Flask 3.x（服务端渲染）
- Jinja2 模板 + Bootstrap 5（CDN）
- psycopg2-binary（PostgreSQL 驱动）
- python-dotenv（读取 .env 配置）

## 目录结构

```
rims-db-viewer/
├── app.py              # Flask 入口 + 所有路由
├── db.py               # PostgreSQL 连接池管理
├── requirements.txt    # 依赖列表
├── templates/
│   ├── base.html       # 基础布局（Bootstrap 5 CDN、导航栏）
│   ├── index.html      # 首页：表列表 + 行数统计
│   ├── table.html      # 表数据浏览（分页、筛选、排序、导出）
│   ├── structure.html  # 表结构（列名、类型、约束）
│   └── query.html      # 自定义 SQL 查询 + 结果展示 + 导出
└── static/
    └── style.css       # 自定义样式（少量）
```

## 功能详述

### 1. 首页 — 表列表 (`/`)

- 查询 `information_schema.tables` 列出所有 public schema 的用户表
- 每张表显示：表名、预估行数（`pg_class.reltuples`）
- 点击表名跳转到表数据页

### 2. 表数据浏览 (`/table/<name>`)

- 分页展示表数据，默认每页 50 行，支持翻页
- **筛选过滤**：页面顶部显示筛选栏，每列提供文本输入框，生成 `WHERE column::text ILIKE '%keyword%'` 条件；多列条件用 AND 组合
- **排序**：点击列头切换 ASC/DESC 排序
- 筛选和排序参数通过 URL query params 传递，翻页时保持
- **导出 CSV**：按钮触发 `GET /table/<name>/export?<当前筛选参数>`，流式输出 CSV

### 3. 表结构 (`/table/<name>/structure`)

- 查询 `information_schema.columns` 获取列定义
- 显示：列名、数据类型、是否可空、默认值、字符最大长度
- 查询 `pg_indexes` 显示该表的索引列表

### 4. SQL 查询 (`/query`)

- 提供文本域输入 SQL
- 仅允许 `SELECT` 语句（服务端校验：strip 后必须以 `SELECT` 或 `WITH` 开头，拒绝包含 `;` 后跟其他语句）
- 结果以表格展示，上限 1000 行
- **导出 CSV**：`POST /query/export`，导出查询结果（上限 10000 行）
- 显示执行耗时

### 5. CSV 导出

- 表数据导出：`GET /table/<name>/export` — 导出当前筛选条件下的全部数据（上限 10000 行）
- SQL 查询导出：`POST /query/export` — 导出查询结果（上限 10000 行）
- 响应头：`Content-Disposition: attachment; filename=<name>_export.csv`
- 使用 `csv.writer` + Flask `Response` 流式输出

## 安全措施

- SQL 查询仅允许只读 SELECT 语句
- `SET statement_timeout = '10s'` 限制查询执行时间
- 结果行数上限（浏览 1000 行，导出 10000 行）
- 表名参数校验（只允许 `information_schema.tables` 中存在的表名，防注入）
- 默认监听 `127.0.0.1:5001`，仅本地访问
- 仅供开发环境使用，无认证

## 数据库连接

复用项目根目录 `.env` 中的配置：
- `DB_HOST` (127.0.0.1)
- `DB_PORT` (5432)
- `DB_USER` (app)
- `DB_PASSWORD` (app123456)
- `DB_NAME` (appdb)

使用 `psycopg2` 连接池（`psycopg2.pool.SimpleConnectionPool`），最小 1 最大 5 连接。

## 路由汇总

| 方法 | 路径 | 功能 |
|------|------|------|
| GET | `/` | 表列表 |
| GET | `/table/<name>` | 表数据浏览（分页、筛选、排序） |
| GET | `/table/<name>/structure` | 表结构 |
| GET | `/table/<name>/export` | 导出表数据 CSV |
| GET | `/query` | SQL 查询页面 |
| POST | `/query` | 执行 SQL 查询 |
| POST | `/query/export` | 导出查询结果 CSV |

## 验证方式

1. 在 WSL 中启动 PostgreSQL（`docker compose up -d`）
2. `cd rims-db-viewer && pip install -r requirements.txt`
3. `python app.py`（或 `flask run -p 5001`）
4. 浏览器打开 `http://127.0.0.1:5001`
5. 验证：表列表显示所有表 → 点击表查看数据 → 筛选/排序 → 导出 CSV → 执行自定义 SQL 查询
