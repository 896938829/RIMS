<!-- SPDX-License-Identifier: AGPL-3.0-or-later -->
<!-- Copyright (c) 2026 ShangBin Wang -->

Create a blue visual design variant for the RIMS mobile app frontend overview board.

Reference intent:
- Keep the same composition and information architecture as the previous US-style RIMS frontend app design board.
- Preserve: large hero phone on the left, five smaller app screens across the middle, top architecture chips, right role/API/backend cards, bottom workflow strips.
- Change the visual language to a coherent blue-series product design.

Canvas:
- 16:9 landscape presentation board.
- Premium light mode, cool white background (#F7FAFF / #F9FBFF).
- Soft blue-tinted cards, subtle blue-gray borders, gentle shadows.
- Clean modern US B2B app aesthetic: Apple HIG + Stripe Dashboard + Linear polish.

Color system:
- Primary: professional blue (#2563EB or similar).
- Deep navy text: #0F172A.
- Secondary: sky blue / cyan (#0EA5E9) for inventory and scan actions.
- Supporting blue-purple only as a very subtle accent for audit / permissions.
- Warning: very restrained amber (#F59E0B) only for warning labels.
- Avoid green as the dominant color. Avoid orange as a main action color. Avoid heavy purple.
- Overall feeling: one coherent blue family, but not monotonous; use blue depth, neutral gray, and tiny amber warnings.

Main title:
- "RIMS Frontend App Design"
- Subtitle: "Blue visual system · 5-tab app shell · role-aware inventory workflows"

Left hero phone:
- Make the phone UI feel like a real polished iOS inventory app.
- Header: "上海仓" with chevron.
- Hero card uses a rich blue gradient or blue solid surface, but keep it refined and not flashy.
- KPI cards: 商品, 库存, 预警.
- Quick actions use blue icon buttons:
  扫码销售, 退货, 入库, 调拨.
- Floating scan action uses strong primary blue.
- Bottom tab bar: 首页, 库存, 单据, 报表, 我的, with active tab in blue.

Middle five app screens:
- Screen 1 首页: blue dashboard cards, alert chips, recent docs.
- Screen 2 库存: blue segmented control 标准 / 商品 / 非标, inventory list cards, search bar, scan icon.
- Screen 3 单据: document cards in soft blue, workflow stepper in primary blue.
- Screen 4 报表: elegant blue line chart, blue bar chart, donut chart with mostly blue tones plus tiny amber warning slice.
- Screen 5 我的: profile card, settings list, API guard chips, backend module chips.
- All five screens should look spacious, native, and polished.

Top architecture chips:
- Use rounded white chips with blue icons and subtle arrows.
- Labels: 登录, 角色, 仓库, 权限, API, 后端模块.

Right side cards:
- Replace table-like role matrix with two refined cards:
  管理员: 多仓, 入库, 调拨, 盘点, 非标, 成本, 审计.
  普通用户: 固定仓, 销售, 退货, 库存, 报表.
- API guard chips: JWT, X-Warehouse-ID, Permission, Idempotency-Key, traceId.
- Backend module chips: user, warehouse, product, document, report, file, audit.
- Use blue outlined chips and soft blue fills.

Bottom workflows:
- Four workflow strips with blue-centric icons and connectors:
  销售: 扫码 -> 商品 -> 校验 -> 确认 -> 完成
  退货: 销售单 -> 可退量 -> 原因 -> 入库
  盘点: 录入 -> 差异 -> 结转
  转标准: 非标 -> 标准商品 -> 数量 -> 转换
- Use different blue tints per strip, but keep the whole row visually unified.

Typography and text:
- Keep labels short and legible.
- Simplified Chinese labels for business functions are acceptable.
- Avoid tiny paragraphs; prefer larger UI labels and icon captions.

Constraints:
- Do not change the core app concept.
- Do not create a marketing landing page.
- No people, no stock photos, no decorative 3D objects.
- Do not use a green-dominant palette.
- Do not make everything dark blue; keep the page bright and airy.
- Make it feel like a finished design board a US SaaS product team would present.
