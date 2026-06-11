<!-- SPDX-License-Identifier: AGPL-3.0-or-later -->
<!-- Copyright (c) 2026 ShangBin Wang -->

RIMS mobile app frontend design overview board.

Generate a clean 16:9 product design blueprint for a Retail Inventory Management System mobile app.

Language and labels:
- Use simplified Chinese labels where visible.
- Keep all text short and large enough to read.
- Do not fill the image with paragraphs. Use concise labels and icon-like UI hints.

Canvas:
- Landscape 16:9, presentation-board style.
- White / very light gray background, restrained enterprise UI.
- Accent colors: inventory green, document blue, warning amber, audit purple, neutral graphite.
- Avoid decorative gradients, bokeh, mascots, stock photos, or marketing hero style.

Main title:
- "RIMS 移动端前端设计图"
- Subtitle: "5 个主分窗 · 仓库上下文 · 角色权限 · 单据闭环"

Overall layout:
1. Top band: app architecture flow
   - 登录 / 会话
   - 用户角色
   - 当前仓库
   - 权限裁剪
   - 统一 API
   - 后端模块
   Draw this as a calm left-to-right flow with small icons.

2. Center-left: a large mobile app shell mockup
   - Header: current warehouse selector "当前仓库：上海仓"
   - Right header icons: scan, network state, notifications
   - Bottom navigation with five tabs:
     首页, 库存, 单据, 报表, 我的
   - The shell should look like a real mobile operations app, not a marketing page.

3. Center and right: five smaller phone-screen wireframes, one per primary tab:
   - 首页:
     "仓库卡片", "快捷操作", "库存预警", "待处理单据", "报表摘要"
     Quick action chips: 扫码销售, 退货入库, 入库, 调拨, 盘点
   - 库存:
     Tabs: 标准库存, 商品档案, 非标库存
     Elements: search bar, scan button, inventory list rows, low-stock tag, product detail drawer
   - 单据:
     Filters: 类型, 状态, 时间
     Flow cards: 销售出库, 退货入库, 入库, 调拨, 盘点
     Show a stepper: 明细 -> 确认 -> 提交 -> 回执
   - 报表:
     Date range, warehouse filter, chart cards
     Cards: 销售趋势, 商品排行, 库存概览, 滞销预警
   - 我的:
     Profile, role, warehouse settings, theme, file center, admin center
     Admin-only badges: 用户管理, 角色权限, 仓库管理, 审计日志

4. Bottom band: four key business flows as compact swimlanes:
   - 销售: 扫码 -> 选商品 -> 库存校验 -> 二次确认 -> 完成
   - 退货: 原销售单 -> 可退数量 -> 原因 -> 确认 -> 入库
   - 盘点: 录入 -> 差异确认 -> 结转
   - 非标转标准: 非标记录 -> 标准商品 -> 数量 -> 转换

5. Side panel: role and backend mapping
   - Role matrix mini table:
     管理员: 多仓切换, 入库, 调拨, 盘点, 非标, 成本, 审计
     普通用户: 固定仓库, 销售, 退货, 查库存, 查报表
   - Backend module chips:
     user, warehouse, product, document, report, file, audit
   - API guard chips:
     JWT, X-Warehouse-ID, Permission, Idempotency-Key, traceId

Visual style:
- Low-to-mid fidelity UX design board.
- Professional enterprise mobile SaaS, dense but organized.
- Crisp UI components, aligned grid, realistic spacing.
- Use simple line icons for scan, inventory, document, chart, user, lock, warehouse.
- Rounded cards with small radius, not overly bubbly.
- Good contrast in both light and dark text.
- Make the design feel practical for warehouse / retail staff doing repeated operations.

Constraints:
- Do not create a standalone landing page.
- Do not use fake people, product photography, or 3D decorative objects.
- Do not overuse a single blue or purple palette.
- Avoid tiny unreadable text.
- Keep Chinese labels legible and minimal.
