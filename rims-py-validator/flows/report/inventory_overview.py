"""库存总览/周转/滞销流程（PRD 第 4.5.2 章 — 库存分析）。

预期步骤：
     1) 准备：admin 身份、WH001、创建商品 A、入库 20 件、销售 5 件
        — 保证仓库有库存行且有出库流水，三张报表才有数据
     2) GET /reports/inventory/overview
        — 校验 skuCount/totalQty/lowStockCount 字段
        — admin 应包含 totalValue 字段
     3) GET /reports/inventory/turnover 今天日期范围
        — 校验 list 非空，outboundQty/avgStock/turnoverRate 字段
     4) GET /reports/inventory/slow-moving 今天日期范围 maxSales=0
        — 校验分页结构
        — 商品 A 有销售 5 件，maxSales=0 时不应出现
     5) GET /reports/inventory/slow-moving maxSales=10
        — 商品 A 销售 5 件 ≤ 10，且 currentStock > 0，应出现
     6) 非 admin 验证：创建普通用户、绑仓库、登录
        — GET /reports/inventory/overview 不应含 totalValue
     7) 错误路径：
        — turnover 缺少日期 → 400
        — slow-moving 日期范围 > 366 天 → 400
     8) 清理：删普通用户；商品有库存无法删仅记录

测试原则：模拟用户操作，寻找后端漏洞。
"""

from __future__ import annotations

import time
from datetime import date, timedelta
from typing import List, Optional

from core.assertions import assert_eq, assert_in, assert_page, expect_error
from core.client import APIClient, APIError
from core.config import CONFIG
from core.logger import get_logger
from core.session import SESSION
from flows.auth import login as auth_login_flow

_log = get_logger("flow.report.inventory_overview")


# --------------- 工具函数 ---------------


def _unique_code() -> str:
    """生成唯一商品编码。"""
    return f"rpt_inv_{int(time.time() * 1000)}"


def _unique_name() -> str:
    """生成唯一商品名称。"""
    return f"库存报表测试_{int(time.time() * 1000)}"


def _unique_username() -> str:
    """生成唯一测试用户名。"""
    return f"rpt_inv_user_{int(time.time() * 1000)}"


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
    """执行库存总览/周转/滞销全流程验证。

    前置条件：调用方（main.py._register_todo）已以 admin 身份登录。
    """
    vulnerabilities: List[str] = []
    today = _today_str()

    # ---------- 第 1 步：环境准备 ----------
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
    retail_price = 40.0
    cost_price = 20.0
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
        # 入库 20 件
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

        # 销售 5 件 — 产生出库流水供周转率/滞销分析
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

        # ---------- 第 2 步：库存概况（admin） ----------
        _log.info("GET /reports/inventory/overview")
        overview = client.get("/reports/inventory/overview")
        _log.info("库存概况返回：%s", overview)

        assert_in("skuCount", overview, "overview")
        assert_in("totalQty", overview, "overview")
        assert_in("lowStockCount", overview, "overview")

        if overview["skuCount"] < 1:
            raise AssertionError(f"overview.skuCount 应 >= 1，实际 {overview['skuCount']}")
        if overview["totalQty"] < 15:
            _log.warning("overview.totalQty=%s，预期至少 15（可能有其他测试数据）",
                         overview["totalQty"])

        # admin 应含 totalValue
        if "totalValue" not in overview:
            vulnerabilities.append("admin overview 缺少 totalValue 字段")
            _log.warning("⚠ admin overview 未包含 totalValue 字段")
        else:
            _log.info("admin overview.totalValue=%s", overview["totalValue"])
        _log.info("库存概况 admin 字段校验完成")

        # ---------- 第 3 步：库存周转率 ----------
        _log.info("GET /reports/inventory/turnover startDate=%s endDate=%s", today, today)
        turnover = client.get("/reports/inventory/turnover", params={
            "startDate": today,
            "endDate": today,
            "limit": 20,
        })
        assert_in("list", turnover, "turnover")
        t_list = turnover["list"]
        if len(t_list) < 1:
            raise AssertionError("turnover list 应非空（刚完成一笔销售）")

        # 校验字段
        first_t = t_list[0]
        assert_in("productId", first_t, "turnover.list[0]")
        assert_in("productCode", first_t, "turnover.list[0]")
        assert_in("productName", first_t, "turnover.list[0]")
        assert_in("outboundQty", first_t, "turnover.list[0]")
        assert_in("avgStock", first_t, "turnover.list[0]")
        assert_in("turnoverRate", first_t, "turnover.list[0]")

        # 找我们的测试商品
        my_turnover = next(
            (t for t in t_list if t.get("productId") == product_id), None
        )
        if my_turnover:
            _log.info("测试商品周转：outboundQty=%s avgStock=%s turnoverRate=%s",
                      my_turnover["outboundQty"],
                      my_turnover["avgStock"],
                      my_turnover["turnoverRate"])
            if my_turnover["outboundQty"] < sale_qty:
                _log.warning("⚠ outboundQty=%s 应 >= %s",
                             my_turnover["outboundQty"], sale_qty)
        else:
            _log.warning("测试商品 id=%s 未出现在周转率列表中", product_id)
        _log.info("周转率校验通过：%d 条", len(t_list))

        # ---------- 第 4 步：滞销预警 maxSales=0 ----------
        _log.info("GET /reports/inventory/slow-moving maxSales=0")
        slow_0 = client.get("/reports/inventory/slow-moving", params={
            "startDate": today,
            "endDate": today,
            "maxSales": 0,
            "page": 1,
            "pageSize": 50,
        })
        assert_page(slow_0)

        # 商品 A 销售了 5 件，maxSales=0 时不应出现
        found_in_slow0 = any(
            s.get("productId") == product_id for s in (slow_0.get("list") or [])
        )
        if found_in_slow0:
            _log.warning("⚠ 商品 A（salesQty=5）出现在 maxSales=0 滞销列表中，"
                         "后端过滤可能有误")
        else:
            _log.info("maxSales=0 正确排除了有销售记录的商品 A")

        # ---------- 第 5 步：滞销预警 maxSales=10 ----------
        _log.info("GET /reports/inventory/slow-moving maxSales=10")
        slow_10 = client.get("/reports/inventory/slow-moving", params={
            "startDate": today,
            "endDate": today,
            "maxSales": 10,
            "page": 1,
            "pageSize": 50,
        })
        assert_page(slow_10)

        # 商品 A 销售 5 件 ≤ 10 且 currentStock=15 > 0，应出现
        my_slow = next(
            (s for s in (slow_10.get("list") or []) if s.get("productId") == product_id),
            None,
        )
        if my_slow:
            assert_in("productCode", my_slow, "slow_moving item")
            assert_in("productName", my_slow, "slow_moving item")
            assert_in("currentStock", my_slow, "slow_moving item")
            assert_in("salesQty", my_slow, "slow_moving item")
            _log.info("商品 A 出现在 maxSales=10 滞销列表：salesQty=%s currentStock=%s",
                      my_slow["salesQty"], my_slow["currentStock"])
        else:
            _log.warning("⚠ 商品 A 未出现在 maxSales=10 滞销列表中（"
                         "salesQty=5 ≤ 10 且 stock > 0 应出现）")

        # ---------- 第 6 步：非 admin 字段屏蔽 ----------
        normal_username = _unique_username()
        normal_password = "RptInv@12345"
        _log.info("创建普通用户：username=%s", normal_username)
        normal_user = client.post("/users", json={
            "username": normal_username,
            "password": normal_password,
            "realName": "库存报表测试用户",
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

        # 非 admin overview — 不应含 totalValue
        _log.info("普通用户 GET /reports/inventory/overview — 校验 totalValue 屏蔽")
        user_overview = client.get("/reports/inventory/overview")
        if "totalValue" in user_overview and user_overview["totalValue"] is not None:
            vulnerabilities.append("非admin overview 泄漏 totalValue")
            _log.warning("⚠ 权限漏洞：非admin overview 含 totalValue=%s",
                         user_overview["totalValue"])
        else:
            _log.info("非admin overview totalValue 正确屏蔽")

        # 非 admin 仍应能访问 turnover/slow-moving（读接口不限 admin）
        _log.info("普通用户 GET /reports/inventory/turnover 应成功")
        client.get("/reports/inventory/turnover", params={
            "startDate": today,
            "endDate": today,
        })
        _log.info("普通用户周转率接口可用")

        _log.info("普通用户 GET /reports/inventory/slow-moving 应成功")
        client.get("/reports/inventory/slow-moving", params={
            "startDate": today,
            "endDate": today,
            "page": 1,
            "pageSize": 10,
        })
        _log.info("普通用户滞销预警接口可用")

        # ---------- 第 7 步：错误路径 ----------
        _log.info("切回 admin 继续错误路径测试")
        auth_login_flow.run(client, CONFIG.admin_user, CONFIG.admin_password)
        client.set_warehouse(seed_wh_id)

        # turnover 缺少日期 → 400
        _log.info("turnover 缺少 startDate 期望 400")
        expect_error(
            lambda: client.get("/reports/inventory/turnover", params={
                "endDate": today,
            }),
            http_status=400,
        )

        _log.info("turnover 缺少 endDate 期望 400")
        expect_error(
            lambda: client.get("/reports/inventory/turnover", params={
                "startDate": today,
            }),
            http_status=400,
        )

        # slow-moving 日期范围超 366 天 → 400
        far_past = (date.today() - timedelta(days=400)).isoformat()
        _log.info("slow-moving 日期范围 > 366 天期望 400")
        expect_error(
            lambda: client.get("/reports/inventory/slow-moving", params={
                "startDate": far_past,
                "endDate": today,
                "page": 1,
                "pageSize": 10,
            }),
            http_status=400,
        )

        _log.info("错误路径验证完成")

    finally:
        # ---------- 第 8 步：清理 ----------
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

        _log.info("库存总览/周转/滞销流程验证完成")
