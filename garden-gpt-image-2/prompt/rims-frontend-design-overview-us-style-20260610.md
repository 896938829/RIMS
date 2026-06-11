<!-- SPDX-License-Identifier: AGPL-3.0-or-later -->
<!-- Copyright (c) 2026 ShangBin Wang -->

Create a new visual variant of the RIMS mobile app frontend design overview.

Goal:
- Redraw the existing RIMS frontend design overview in a more American modern app style.
- Preserve the same product information architecture: five primary tabs, warehouse context, role permissions, document workflows, and backend/API mapping.
- Make it feel like a polished US-market iOS / B2B SaaS mobile app design board rather than a dense Chinese enterprise diagram.

Canvas:
- 16:9 landscape design presentation board.
- Clean off-white / cool white background.
- Wide margins, generous whitespace, crisp alignment.
- Use a modern San Francisco / Inter-like sans-serif typography feel.
- Use short simplified Chinese labels for business meaning, but make the visual style American / iOS-native.

Visual style:
- Modern American app style: Apple Human Interface Guidelines, Linear, Square Dashboard, Notion mobile, Stripe-like product polish.
- Light mode, minimal, premium, calm, operational.
- Large rounded iPhone mockup on the left, with realistic native iOS status bar, soft cards, tab bar, floating scan action.
- Cards should have soft shadows, subtle borders, 12-16px radius, clean spacing.
- Use accent colors sparingly:
  - emerald green for inventory / positive status
  - blue for documents / workflows
  - amber for warnings
  - purple for audit / permissions
  - graphite for neutral UI
- Avoid heavy tables, dense grid boxes, thick borders, loud colors, gradients, or decoration.

Composition:
1. Left hero phone:
   - Main dashboard screen.
   - Header: "上海仓" with small warehouse chevron.
   - Large greeting / work summary card.
   - KPI cards: 商品, 库存, 预警.
   - Quick action row: 扫码销售, 退货, 入库, 调拨.
   - Bottom iOS tab bar: 首页, 库存, 单据, 报表, 我的.
   - The phone should be the most polished part of the image.

2. Center area: five smaller app screens in a clean horizontal storyboard:
   - 首页: warehouse context, quick actions, alerts, recent docs.
   - 库存: search, scan, segmented tabs 标准 / 商品 / 非标, inventory cards.
   - 单据: type filters, sales/return/inbound/transfer/stocktake cards, submit stepper.
   - 报表: date filter, elegant charts, ranking, inventory overview.
   - 我的: profile, settings, admin center, audit.
   These screens should look like native mobile UI mockups, not technical boxes.

3. Top row: compact architecture chips:
   - 登录
   - 角色
   - 仓库
   - 权限
   - API
   - 后端模块
   Use rounded pills with icons, connected by subtle arrows.

4. Right side: role and API mapping as elegant cards:
   - "管理员" card: 多仓, 入库, 调拨, 盘点, 非标, 成本, 审计.
   - "普通用户" card: 固定仓, 销售, 退货, 库存, 报表.
   - API guard chips: JWT, X-Warehouse-ID, Permission, Idempotency-Key, traceId.
   - Backend module chips: user, warehouse, product, document, report, file, audit.
   Use cards and chips, not a spreadsheet-like table.

5. Bottom row: four elegant workflow strips:
   - 销售: 扫码 -> 商品 -> 校验 -> 确认 -> 完成
   - 退货: 销售单 -> 可退量 -> 原因 -> 入库
   - 盘点: 录入 -> 差异 -> 结转
   - 转标准: 非标 -> 标准商品 -> 数量 -> 转换
   Each workflow strip uses clean icons and small connected steps.

Text:
- Main title: "RIMS Frontend App Design"
- Subtitle: "移动库存作业 · 5-tab app shell · role-aware workflows"
- Keep Chinese business labels short and legible.
- Avoid tiny text. Prefer fewer, larger labels.

Quality constraints:
- Looks like a professional product design board from a US startup design team.
- More app mockup than technical architecture diagram.
- Balanced information density: enough detail to understand the app, but spacious and premium.
- No stock photos, no people, no 3D decorative objects, no marketing hero.
- Preserve practical retail inventory meaning.
