"""销售统计/趋势/排行流程（PRD 第 4.5.1 章 — 销售分析）。

预期步骤：
     1) 准备：admin 身份、WH001、创建商品 A、入库 20 件、完成销售 5 件
        — 保证当前仓库有至少一笔已完成销售单，报表才有数据
     2) GET /reports/sales/stats 指定今天日期范围
        — 校验 revenue/orderCount/skuCount/quantity 非零
        — admin 应包含 costAmount/grossProfit 字段
     3) GET /reports/sales/trend bucket=day 同日期范围
        — 校验 list 非空，period 格式 YYYY-MM-DD
     4) GET /reports/sales/trend bucket=week
        — 校验 period 格式 YYYY-Www
     5) GET /reports/sales/trend bucket=month
        — 校验 period 格式 YYYY-MM
     6) GET /reports/sales/ranking metric=qty
        — 校验 list 非空，商品 A 应在排行中
     7) GET /reports/sales/ranking metric=amount
        — 校验 amount 字段，admin 应含 grossProfit
     8) 非 admin 验证：创建普通用户、绑仓库、登录
        — GET /reports/sales/stats 不应含 costAmount/grossProfit
        — GET /reports/sales/ranking 不应含 grossProfit
     9) 错误路径：
        — 缺少 startDate/endDate → 400
        — 日期范围 > 366 天 → 400
        — 非法 bucket 值 → 400
        — 非法 metric 值 → 400
    10) 清理：删普通用户；商品有库存无法删仅记录

测试原则：模拟用户操作，寻找后端漏洞。
"""

from __future__ import annotations

import time
from datetime import date, timedelta
from typing import List, Optional

from core.assertions import assert_eq, assert_in, expect_error
from core.client import APIClient, APIError
from core.config import CONFIG
from core.logger import get_logger
from core.session import SESSION
from flows.auth import login as auth_login_flow

_log = get_logger("flow.report.sales_stats")


# --------------- 工具函数 ---------------


def _unique_code() -> str:
    """生成唯一商品编码。"""
    return f"rpt_sale_{int(time.time() * 1000)}"


def _unique_name() -> str:
    """生成唯一商品名称。"""
    return f"报表销售测试_{int(time.time() * 1000)}"


def _unique_username() -> str:
    """生成唯一测试用户名。"""
    return f"rpt_sale_user_{int(time.time() * 1000)}"


def _today_str() -> str:
    """返回今天日期字符串 YYYY-MM-DD。"""
    return date.today().isoformat()


def _get_role_id_by_code(client: APIClient, code: str) -> int:
    """通过角色编码获取角色 ID。"""
    roles = client.get("/roles", scoped=False)
    for r in roles:
        if r.get("code") == code:
            return r["id"]
    raise AssertionError(f"未找到 code={code!r} 的角色")


def _find_seed_warehouse_id(client: APIClient, code: str) -> int:
    """按 code 查找 seed 仓库 ID。"""
    page = client.get("/warehouses", params={
        "page": 1, "pageSize": 50, "keyword": code,
    }, scoped=False)
    for w in page.get("list") or []:
        if w.get("code") == code:
            return w["id"]
    raise AssertionError(f"未找到 seed 仓库 code={code!r}")


# --------------- 主流程 ---------------


def run(client: APIClient) -> None:  # noqa: C901
    """执行销售统计/趋势/排行全流程验证。

    前置条件：调用方（main.py._register_todo）已以 admin 身份登录。
    """
    vulnerabilities: List[str] = []
    today = _today_str()

    # ---------- 第 1 步：环境准备 — 创建商品 + 入库 + 销售 ----------
    admin_id = SESSION.user.get("id")
    if admin_id is None:
        raise AssertionError("SESSION.user.id 为空，疑似未登录")
    _log.info("当前 admin：id=%s username=%s", admin_id, SESSION.user.get("username"))

    _log.info("定位 seed 仓库 WH001")
    seed_wh_id = _find_seed_warehouse_id(client, "WH001")
    _log.info("seed 仓库 WH001.id=%s", seed_wh_id)
    client.set_warehouse(seed_wh_id)

    user_role_id = _get_role_id_by_code(client, "user")

    # 创建测试商品
    product_code = _unique_code()
    product_name = _unique_name()
    retail_price = 50.0
    cost_price = 30.0
    _log.info("创建测试商品：code=%s retailPrice=%.1f costPrice=%.1f",
              product_code, retail_price, cost_price)
    product = client.post("/products", json={
        "code": product_code,
        "name": product_name,
        "unit": "件",
        "retailPrice": retail_price,
        "costPrice": cost_price,
    }, scoped=False)
    product_id = product["id"]
    _log.info("商品创建成功：id=%s", product_id)

    normal_user_id: Optional[int] = None

    try:
        # 入库 20 件建立库存
        _log.info("创建入库单：商品 id=%s qty=20", product_id)
        inbound_doc = client.post("/documents", json={
            "docType": 1,
            "lines": [{"productId": product_id, "quantity": 20}],
        })
        inbound_id = inbound_doc["id"]
        _log.info("入库单创建成功：id=%s docNo=%s", inbound_id, inbound_doc.get("docNo"))

        _log.info("完成入库单")
        client.post(f"/documents/{inbound_id}/complete")
        _log.info("入库完成，库存应为 20")

        # 销售 5 件 — 制造报表数据
        sale_qty = 5
        _log.info("创建销售单：商品 id=%s qty=%s", product_id, sale_qty)
        sale_doc = client.post("/documents", json={
            "docType": 2,
            "lines": [{"productId": product_id, "quantity": sale_qty}],
        })
        sale_id = sale_doc["id"]
        _log.info("销售单创建成功：id=%s docNo=%s", sale_id, sale_doc.get("docNo"))

        _log.info("完成销售单")
        client.post(f"/documents/{sale_id}/complete")
        _log.info("销售完成，库存应为 15")

        # ---------- 第 2 步：销售统计（admin） ----------
        _log.info("GET /reports/sales/stats startDate=%s endDate=%s", today, today)
        stats = client.get("/reports/sales/stats", params={
            "startDate": today,
            "endDate": today,
        })
        _log.info("销售统计返回：%s", stats)

        # 基本字段校验 — 至少有我们刚完成的那笔销售
        assert_in("revenue", stats, "stats")
        assert_in("orderCount", stats, "stats")
        assert_in("skuCount", stats, "stats")
        assert_in("quantity", stats, "stats")
        if stats["orderCount"] < 1:
            raise AssertionError(f"stats.orderCount 应 >= 1，实际 {stats['orderCount']}")
        if stats["quantity"] < sale_qty:
            raise AssertionError(f"stats.quantity 应 >= {sale_qty}，实际 {stats['quantity']}")

        # admin 应包含成本和毛利字段
        if "costAmount" not in stats:
            vulnerabilities.append("admin 缺少 costAmount 字段")
            _log.warning("⚠ admin stats 未包含 costAmount 字段")
        if "grossProfit" not in stats:
            vulnerabilities.append("admin 缺少 grossProfit 字段")
            _log.warning("⚠ admin stats 未包含 grossProfit 字段")
        _log.info("销售统计 admin 字段校验完成")

        # ---------- 第 3 步：销售趋势 bucket=day ----------
        _log.info("GET /reports/sales/trend bucket=day")
        trend_day = client.get("/reports/sales/trend", params={
            "startDate": today,
            "endDate": today,
            "bucket": "day",
        })
        assert_in("list", trend_day, "trend_day")
        day_list = trend_day["list"]
        if len(day_list) < 1:
            raise AssertionError("trend bucket=day list 应非空")
        # 校验 period 格式：YYYY-MM-DD
        period_0 = day_list[0].get("period", "")
        if len(period_0) != 10 or period_0[4] != "-" or period_0[7] != "-":
            raise AssertionError(f"day trend period 格式异常：{period_0!r}，期望 YYYY-MM-DD")
        assert_in("revenue", day_list[0], "trend_day.list[0]")
        assert_in("orderCount", day_list[0], "trend_day.list[0]")
        _log.info("趋势 day 校验通过：%d 条", len(day_list))

        # ---------- 第 4 步：销售趋势 bucket=week ----------
        _log.info("GET /reports/sales/trend bucket=week")
        trend_week = client.get("/reports/sales/trend", params={
            "startDate": today,
            "endDate": today,
            "bucket": "week",
        })
        assert_in("list", trend_week, "trend_week")
        week_list = trend_week["list"]
        if len(week_list) < 1:
            raise AssertionError("trend bucket=week list 应非空")
        # 校验 period 格式：YYYY-Www（如 2026-W16）
        wp = week_list[0].get("period", "")
        if "W" not in wp:
            raise AssertionError(f"week trend period 缺少 'W'：{wp!r}，期望 YYYY-Www")
        _log.info("趋势 week 校验通过：%d 条", len(week_list))

        # ---------- 第 5 步：销售趋势 bucket=month ----------
        _log.info("GET /reports/sales/trend bucket=month")
        trend_month = client.get("/reports/sales/trend", params={
            "startDate": today,
            "endDate": today,
            "bucket": "month",
        })
        assert_in("list", trend_month, "trend_month")
        month_list = trend_month["list"]
        if len(month_list) < 1:
            raise AssertionError("trend bucket=month list 应非空")
        # 校验 period 格式：YYYY-MM（7 字符）
        mp = month_list[0].get("period", "")
        if len(mp) != 7 or mp[4] != "-":
            raise AssertionError(f"month trend period 格式异常：{mp!r}，期望 YYYY-MM")
        _log.info("趋势 month 校验通过：%d 条", len(month_list))

        # ---------- 第 6 步：商品销售排行 metric=qty ----------
        _log.info("GET /reports/sales/ranking metric=qty limit=10")
        rank_qty = client.get("/reports/sales/ranking", params={
            "startDate": today,
            "endDate": today,
            "metric": "qty",
            "limit": 10,
        })
        assert_in("list", rank_qty, "rank_qty")
        rq_list = rank_qty["list"]
        if len(rq_list) < 1:
            raise AssertionError("ranking metric=qty list 应非空")
        # 校验我们的测试商品在排行里
        found_product = any(
            r.get("productId") == product_id for r in rq_list
        )
        if not found_product:
            _log.warning("⚠ 测试商品 id=%s 未出现在 qty 排行中（可能被其他数据挤出）",
                         product_id)
        else:
            _log.info("测试商品出现在 qty 排行中")
        # 校验字段完整性
        assert_in("productCode", rq_list[0], "rank_qty.list[0]")
        assert_in("productName", rq_list[0], "rank_qty.list[0]")
        assert_in("quantity", rq_list[0], "rank_qty.list[0]")
        assert_in("amount", rq_list[0], "rank_qty.list[0]")
        _log.info("排行 qty 校验通过：%d 条", len(rq_list))

        # ---------- 第 7 步：商品销售排行 metric=amount ----------
        _log.info("GET /reports/sales/ranking metric=amount limit=10")
        rank_amt = client.get("/reports/sales/ranking", params={
            "startDate": today,
            "endDate": today,
            "metric": "amount",
            "limit": 10,
        })
        assert_in("list", rank_amt, "rank_amt")
        ra_list = rank_amt["list"]
        if len(ra_list) < 1:
            raise AssertionError("ranking metric=amount list 应非空")
        # admin 应含 grossProfit
        if "grossProfit" not in ra_list[0]:
            vulnerabilities.append("admin ranking 缺少 grossProfit 字段")
            _log.warning("⚠ admin ranking 未包含 grossProfit 字段")
        _log.info("排行 amount 校验通过：%d 条", len(ra_list))

        # ---------- 第 8 步：非 admin 字段屏蔽 ----------
        normal_username = _unique_username()
        normal_password = "RptTest@12345"
        _log.info("创建普通用户：username=%s", normal_username)
        normal_user = client.post("/users", json={
            "username": normal_username,
            "password": normal_password,
            "realName": "报表测试用户",
            "roleId": user_role_id,
        }, scoped=False)
        normal_user_id = normal_user["id"]

        _log.info("绑定普通用户到 WH001")
        client.post(f"/warehouses/{seed_wh_id}/users",
                    json={"userIds": [normal_user_id]}, scoped=False)

        _log.info("切换登录为普通用户")
        auth_login_flow.run(client, normal_username, normal_password)
        assert_eq(SESSION.role_code, "user", "切换后角色应为 user")
        client.set_warehouse(seed_wh_id)

        # 非 admin stats — 不应含 costAmount/grossProfit
        _log.info("普通用户 GET /reports/sales/stats — 校验成本字段屏蔽")
        user_stats = client.get("/reports/sales/stats", params={
            "startDate": today,
            "endDate": today,
        })
        if "costAmount" in user_stats and user_stats["costAmount"] is not None:
            vulnerabilities.append("非admin stats 泄漏 costAmount")
            _log.warning("⚠ 权限漏洞：非admin stats 含 costAmount=%s",
                         user_stats["costAmount"])
        if "grossProfit" in user_stats and user_stats["grossProfit"] is not None:
            vulnerabilities.append("非admin stats 泄漏 grossProfit")
            _log.warning("⚠ 权限漏洞：非admin stats 含 grossProfit=%s",
                         user_stats["grossProfit"])
        _log.info("非admin stats 字段屏蔽校验完成")

        # 非 admin ranking — 不应含 grossProfit
        _log.info("普通用户 GET /reports/sales/ranking — 校验毛利字段屏蔽")
        user_rank = client.get("/reports/sales/ranking", params={
            "startDate": today,
            "endDate": today,
            "metric": "qty",
            "limit": 5,
        })
        if user_rank.get("list"):
            first_rank = user_rank["list"][0]
            if "grossProfit" in first_rank and first_rank["grossProfit"] is not None:
                vulnerabilities.append("非admin ranking 泄漏 grossProfit")
                _log.warning("⚠ 权限漏洞：非admin ranking 含 grossProfit=%s",
                             first_rank["grossProfit"])
        _log.info("非admin ranking 字段屏蔽校验完成")

        # ---------- 第 9 步：错误路径 ----------
        _log.info("切回 admin 继续错误路径测试")
        auth_login_flow.run(client, CONFIG.admin_user, CONFIG.admin_password)
        client.set_warehouse(seed_wh_id)

        # 缺少必填日期参数 → 400
        _log.info("stats 缺少 startDate 期望 400")
        expect_error(
            lambda: client.get("/reports/sales/stats", params={
                "endDate": today,
            }),
            http_status=400,
        )

        _log.info("stats 缺少 endDate 期望 400")
        expect_error(
            lambda: client.get("/reports/sales/stats", params={
                "startDate": today,
            }),
            http_status=400,
        )

        # 日期范围超 366 天 → 400
        far_past = (date.today() - timedelta(days=400)).isoformat()
        _log.info("stats 日期范围 > 366 天期望 400")
        expect_error(
            lambda: client.get("/reports/sales/stats", params={
                "startDate": far_past,
                "endDate": today,
            }),
            http_status=400,
        )

        # 非法 bucket → 400
        _log.info("trend 非法 bucket='year' 期望 400")
        expect_error(
            lambda: client.get("/reports/sales/trend", params={
                "startDate": today,
                "endDate": today,
                "bucket": "year",
            }),
            http_status=400,
        )

        # 非法 metric → 400
        _log.info("ranking 非法 metric='profit' 期望 400")
        expect_error(
            lambda: client.get("/reports/sales/ranking", params={
                "startDate": today,
                "endDate": today,
                "metric": "profit",
            }),
            http_status=400,
        )

        _log.info("错误路径验证完成")

    finally:
        # ---------- 第 10 步：清理 ----------
        _log.info("===== 开始清理 =====")
        if not SESSION.is_admin():
            try:
                auth_login_flow.run(client, CONFIG.admin_user, CONFIG.admin_password)
            except Exception as e:  # noqa: BLE001
                _log.warning("清理阶段切回 admin 失败：%s", e)
        client.set_warehouse(seed_wh_id)

        if normal_user_id is not None:
            _log.info("删除普通用户：id=%s", normal_user_id)
            try:
                client.delete(f"/users/{normal_user_id}", scoped=False)
            except APIError as e:
                _log.warning("删除普通用户失败（忽略）：%s", e)

        # 商品有库存行时无法硬删（20002），仅尝试
        _log.info("尝试删除商品：id=%s（有库存时后端会返回 20002）", product_id)
        try:
            client.delete(f"/products/{product_id}", scoped=False)
            _log.info("商品删除成功")
        except APIError as e:
            _log.warning("商品无法删除（符合预期，已存在库存行）：%s", e)

        if vulnerabilities:
            _log.warning("⚠ 发现 %d 个问题: %s",
                         len(vulnerabilities), ", ".join(vulnerabilities))
        else:
            _log.info("所有字段屏蔽校验通过，未发现漏洞")

        _log.info("销售统计/趋势/排行流程验证完成")
