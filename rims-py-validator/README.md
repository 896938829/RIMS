# rims-py-validator

RIMS 后端的 **"假前端"** 验证工具。使用 Python 按业务流程顺序调用 Go 后端的 HTTP API，
逐步打印请求 / 响应日志并做关键断言，用于在没有真实前端的情况下跑通 PRD
（见 [`../docs/产品需求文档.md`](../docs/产品需求文档.md)）里的端到端流程。

## 项目目标

- **模拟前端**：按 PRD 中描述的业务流程发 HTTP 请求，验证后端行为
- **可观测**：每次 API 调用都在控制台与日志文件同步输出，方便排查
- **增量补齐**：先提供最小可运行骨架（登录 + 冒烟），其余流程以 TODO 形式逐步补全
- **独立**：不依赖 Go 项目，不写数据库，纯黑盒
- **测试原则**：模拟用户操作，寻找后端漏洞

## 快速开始

前置条件：
1. 后端已在 WSL 中启动：`cd rims-goProgect && go run ./cmd/server`（默认监听 `:8080`）
2. 数据库已初始化，seed 的 admin 账号存在
3. 本机已安装 Python 3.10+

安装与运行：

```bash
cd rims-py-validator
pip install -r requirements.txt
cp .env.example .env            # 如需调整账号 / 后端地址，编辑 .env

# 最小冒烟：登录 + /users/me + /warehouses
python main.py smoke

# 单独执行登录流程
python main.py auth login
python main.py auth login -u admin -p admin123

# 查看所有子命令
python main.py --help
```

运行日志默认写入 `./logs/run.log`（按天切分，保留 7 天）。

## 目录结构

```
rims-py-validator/
├── main.py                      # Click CLI 入口
├── requirements.txt
├── .env.example
├── core/                        # 基础设施（不要绕过）
│   ├── config.py                # .env 加载
│   ├── logger.py                # 日志工厂
│   ├── client.py                # ★ APIClient（统一请求/响应日志）
│   ├── session.py               # 进程内登录态
│   └── assertions.py            # 断言工具
└── flows/                       # 业务流程（按 RIMS 模块分组）
    ├── _placeholder.py          # 占位工具
    ├── auth/login.py            # ✅ 已实现
    ├── user/user_crud.py
    ├── warehouse/warehouse_crud.py
    ├── warehouse/switch_warehouse.py
    ├── product/product_crud.py
    ├── product/barcode_lookup.py
    ├── inventory/list_and_alert.py
    ├── inventory/non_std_convert.py
    ├── document/inbound.py
    ├── document/sale.py
    ├── document/return_flow.py
    ├── document/transfer.py
    ├── document/stocktake.py
    ├── report/sales_stats.py
    ├── report/inventory_overview.py
    ├── file/upload_download.py
    └── audit/query_logs.py
```

## 已实现 vs TODO

### 已实现

- [x] `core/*` — 配置/日志/HTTP 客户端/会话/断言
- [x] `flows/auth/login.py` — 登录 + /users/me 校验
- [x] `main.py smoke` — 登录 + 拉 `/warehouses` 分页

### TODO（按 PRD 对应业务流）

**user 模块**
- [x] `flows/user/user_crud.py` — 用户创建 / 列表 / 详情 / 改密 / 删除 + 权限校验

**warehouse 模块**
- [x] `flows/warehouse/warehouse_crud.py` — 仓库 CRUD + 绑定/解绑用户
- [x] `flows/warehouse/switch_warehouse.py` — 设置默认仓库 + 切换当前仓库

**product 模块**
- [x] `flows/product/product_crud.py` — 商品 CRUD + 成本价 admin 屏蔽校验
- [x] `flows/product/barcode_lookup.py` — 条码正向/反向查询

**inventory 模块**
- [x] `flows/inventory/list_and_alert.py` — 标准库存列表 + 告警阈值 + 仓库隔离
- [x] `flows/inventory/non_std_convert.py` — 非标创建 / 转标 / admin 权限

**document 模块**
- [x] `flows/document/inbound.py` — 入库单 创建→完成→库存校验
- [x] `flows/document/sale.py` — 销售单 创建→完成→库存&报表校验
- [x] `flows/document/return_flow.py` — 退货单 关联原销售单→完成→库存回补
- [x] `flows/document/transfer.py` — 跨仓调拨 + 双仓库存校验
- [x] `flows/document/stocktake.py` — 盘点三阶段（recording → confirmed → settled）

**report 模块**
- [x] `flows/report/sales_stats.py` — 销售统计/趋势/排行 + admin 字段屏蔽
- [x] `flows/report/inventory_overview.py` — 库存总览/周转/滞销

**file 模块**
- [ ] `flows/file/upload_download.py` — public/private 两类文件上传下载 + 大小限制

**audit 模块**
- [ ] `flows/audit/query_logs.py` — 审计列表/详情 + admin 权限 + 时间窗口限制

---

## 给 AGENT 的约束（实现 TODO 时必须严格遵守）

> 本节是为自动化代理（Claude Code / Cursor / 其他 AI 助手）准备的硬约束。
> 补齐 TODO 时请先完整阅读本节，再动手。

1. **一个流程 = 一个 .py 文件**。不要把多个业务场景塞进同一个文件，也不要把流程逻辑塞进 `main.py`。
   `main.py` 只负责 click 参数解析 + 调用对应 flow 的 `run()`。

2. **每个 .py 顶部必须有中文 docstring**，说明：
   - 对应 PRD 的章节（如"PRD 第 7 章 - 销售"）
   - 业务预期步骤的分步说明

3. **每个函数必须有中文注释**，至少解释一遍"做什么 / 为什么"。
   对关键业务分支（admin-only、期望失败、事务回滚）要额外注释。

4. **严禁绕过 `core.client.APIClient`**。不允许在 flows/ 下 `import requests`，
   不允许使用 `httpx` / `urllib` 等其他 HTTP 库。所有 API 调用必须通过 APIClient，
   才能保证 `[REQ] / [RSP]` 日志成对出现。

5. **每步 API 前后打 info 级业务日志**。仅依赖 APIClient 的通用日志不够，
   业务层必须用 `logger.info` 明确标注语义，例如：
   ```python
   _log.info("开始创建销售单：warehouseId=%s productId=%s qty=%s", wid, pid, qty)
   ...
   _log.info("销售单创建成功：docId=%s docNo=%s", doc_id, doc_no)
   ```

6. **失败路径也要覆盖**。凡 PRD 规定"无权"/"状态不合法"/"超限"的调用，
   必须用 `core.assertions.expect_error` 包一层，断言预期的 `code` 或 `http_status`。
   不允许只测成功路径。

7. **不要引入新依赖**。只能使用 `requests` / `click` / `python-dotenv` 以及 Python 标准库。
   如果觉得必须引新库，先在 README 开 issue 讨论，不要直接改 `requirements.txt`。

8. **完成一个 TODO，更新一次 README**。把上面清单中对应的 `- [ ]` 改成 `- [x]`，
   并在 git commit 中引用该流程名（例如 `feat(validator): implement document.sale flow`）。

9. **登录是前置条件**。除 `flows/auth/*` 以外，每个 flow 的 `run()` 默认假设已经登录，
   调用方（`main.py`）会在调你的 `run()` 前先调一次 `flows.auth.login.run()`。
   你 **不要** 在自己的 flow 里再调一次登录。

10. **不得写入本地数据库 / 文件系统**。验证器是纯黑盒，只通过后端 HTTP API 交互。
    唯一允许写盘的是 `./logs/` 下的日志文件（由 `core.logger` 管理）。

## 日志

- 位置：`./logs/run.log`（按天切分，保留 7 天，文件名后缀 `.YYYY-MM-DD`）
- 格式：`[时间] [级别] [模块名] 内容`
- 关键标记：
  - `[REQ] METHOD URL token=yes|no warehouse=<id> params=... body=...`
  - `[RSP] METHOD URL status=xxx code=0 message=success traceId=...`
  - `[ERR] ...` 网络层异常
- 敏感字段：任何 key 含 `password` 的请求体字段都会被替换成 `***`

## 常见问题

**Q：执行 smoke 报 `code=10003 无效的令牌`？**
A：检查 `.env` 里 `ADMIN_USER` / `ADMIN_PASSWORD` 是否匹配后端 seed 账号，确认后端已重启。

**Q：报 `请先选择仓库`？**
A：当前登录用户没有绑定默认仓库，先跑 `warehouse_crud` 流程绑定，或在 `.env` 中设置
`DEFAULT_WAREHOUSE_ID` 让 CLI 自动附带 `X-Warehouse-ID` 头。

**Q：可以用 pytest 代替 click 跑吗？**
A：当前选型是 Click CLI（见 `main.py`）。若需要 pytest 风格，可以另起 `tests/` 目录，
但仍必须调用同一套 `flows/*.run()` 与 `core.client.APIClient`，不能另起一套 HTTP 代码。
