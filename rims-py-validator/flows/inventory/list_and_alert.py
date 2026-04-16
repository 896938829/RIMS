"""库存列表 + 告警阈值 + 仓库隔离流程（PRD 第 4.2 / 4.3.3 章）。

预期步骤：
     1) 准备：拿到 admin 身份、seed 仓库 WH001 id、user 角色 id
     2) 创建测试商品 A（非 scoped）
     3) 用非标创建 + 转标作为"种子"，在 WH001 为商品 A 创建库存行（qty=10）
        — 后端 Product.Create 不会自动建 Inventory 行，只有 Inbound 单或非标转标会；
        由于 Inbound 流程尚未实现，这里只能借非标 convert 路径播种
     4) PUT /inventory/:id 设 alertThreshold=15 → 触发低库存（status=2）
     5) GET /inventory/alerts 应包含商品 A，校验字段 productCode/productName
     6) PUT /inventory/:id 降 alertThreshold=5 → 解除低库存（status=1）
        再次 GET /alerts 应不再包含商品 A
     7) 仓库隔离：新建 wh_b，切换到 wh_b 后 /inventory 不应看到商品 A 行，
        GET /inventory/{inv_a_id} 返回 404（跨仓库访问）
     8) 切回 WH001，创建普通用户并绑到 WH001，登录：
        - GET /inventory / /alerts 应成功（读接口不限 admin）
        - PUT /inventory/:id 应 403（写接口仅 admin）
     9) 切回 admin，错误路径：GET/PUT 不存在 id → 404；非法 alertThreshold → 400
    10) 清理：删 wh_b、普通用户；商品 A 因存在库存无法硬删（20002），仅记录告警

测试原则：模拟用户操作，寻找后端漏洞。
"""

from __future__ import annotations

import time
from typing import Callable, List, Optional

from core.assertions import assert_eq, assert_in, assert_page, expect_error
from core.client import APIClient, APIError
from core.config import CONFIG
from core.logger import get_logger
from core.session import SESSION
from flows.auth import login as auth_login_flow

_log = get_logger("flow.inventory.list_alert")


# --------------- 工具函数 ---------------


def _unique_code() -> str:
    """生成唯一商品/仓库编码。"""
    return f"inv_{int(time.time() * 1000)}"


def _unique_name() -> str:
    """生成唯一名称。"""
    return f"库存测试_{int(time.time() * 1000)}"


def _unique_username() -> str:
    """生成唯一测试用户名。"""
    return f"inv_test_user_{int(time.time() * 1000)}"


def _unique_temp_label() -> str:
    """生成唯一非标临时标签号。"""
    return f"NST_INV_{int(time.time() * 1000)}"


def _get_role_id_by_code(client: APIClient, code: str) -> int:
    """通过角色编码获取角色 ID。"""
    roles = client.get("/roles", scoped=False)
    for r in roles:
        if r.get("code") == code:
            return r["id"]
    raise AssertionError(f"未找到 code={code!r} 的角色")


def _find_seed_warehouse_id(client: APIClient, code: str) -> int:
    """按 code 查找 seed 仓库的 ID（分页接口，遍历匹配）。"""
    page = client.get("/warehouses", params={
        "page": 1, "pageSize": 50, "keyword": code,
    }, scoped=False)
    for w in page.get("list") or []:
        if w.get("code") == code:
            return w["id"]
    raise AssertionError(f"未找到 seed 仓库 code={code!r}")


def _probe_permission(label: str, fn: Callable[[], object]) -> bool:
    """探测非 admin 是否被正确拦截（期望 HTTP 403）。

    返回 True 表示后端正确拒绝，False 表示存在权限漏洞；不抛异常。
    """
    try:
        expect_error(fn, http_status=403)
        _log.info("权限拦截正常：%s", label)
        return True
    except AssertionError:
        _log.warning("⚠ 权限漏洞: %s — 非 admin 未被拦截 (期望 HTTP 403)", label)
        return False


def _find_inventory_by_product(client: APIClient, product_id: int) -> Optional[dict]:
    """从当前仓库库存分页中定位指定商品的库存行（返回 None 表示未找到）。"""
    page = client.get("/inventory", params={"page": 1, "pageSize": 100})
    for inv in page.get("list") or []:
        if inv.get("productId") == product_id:
            return inv
    return None


# --------------- 主流程 ---------------


def run(client: APIClient) -> None:  # noqa: C901
    """执行库存列表 + 告警阈值 + 仓库隔离全流程验证。

    前置条件：调用方（main.py._register_todo）已以 admin 身份登录。
    """
    vulnerabilities: List[str] = []

    # ---------- 第 1 步：环境准备 ----------
    admin_id = SESSION.user.get("id")
    if admin_id is None:
        raise AssertionError("SESSION.user.id 为空，疑似未登录")
    _log.info("当前 admin：id=%s username=%s", admin_id, SESSION.user.get("username"))

    _log.info("定位 seed 仓库 WH001")
    seed_wh_id = _find_seed_warehouse_id(client, "WH001")
    _log.info("seed 仓库 WH001.id=%s", seed_wh_id)

    # 确保后续 scoped=True 调用落在 WH001
    client.set_warehouse(seed_wh_id)

    user_role_id = _get_role_id_by_code(client, "user")

    # ---------- 第 2 步：创建测试商品 A ----------
    product_a_code = _unique_code()
    product_a_name = _unique_name()
    _log.info("开始创建测试商品 A：code=%s name=%s", product_a_code, product_a_name)
    product_a = client.post("/products", json={
        "code": product_a_code,
        "name": product_a_name,
        "unit": "件",
        "retailPrice": 10.0,
        "costPrice": 5.0,
    }, scoped=False)
    product_a_id = product_a["id"]
    _log.info("商品 A 创建成功：id=%s", product_a_id)

    # 其他资源变量先声明，finally 清理时才能访问
    wh_b_id: Optional[int] = None
    normal_user_id: Optional[int] = None

    try:
        # ---------- 第 3 步：借非标 + 转标播种 WH001 库存行 ----------
        seed_label = _unique_temp_label()
        _log.info("创建非标条目作为库存种子：tempLabel=%s qty=10", seed_label)
        ns_seed = client.post("/non-std-inventory", json={
            "tempLabel": seed_label,
            "description": "list_and_alert 播种",
            "unit": "件",
            "quantity": 10,
            "sourceMethod": "other",
            "sourceDocument": "seed",
        })
        ns_seed_id = ns_seed["id"]
        _log.info("非标条目创建成功：id=%s", ns_seed_id)

        _log.info("开始非标 → 标准 转换：nsId=%s productId=%s qty=10",
                  ns_seed_id, product_a_id)
        client.post(f"/non-std-inventory/{ns_seed_id}/convert", json={
            "productId": product_a_id,
            "quantity": 10,
        })
        _log.info("非标转标完成，(WH001, 商品 A) 库存行应已生成")

        _log.info("查询 WH001 库存列表，定位商品 A")
        inv_a = _find_inventory_by_product(client, product_a_id)
        if inv_a is None:
            raise AssertionError(
                f"WH001 库存列表未找到 productId={product_a_id}（转换后应自动建行）"
            )
        inv_a_id = inv_a["id"]
        assert_eq(inv_a["warehouseId"], seed_wh_id, "库存.warehouseId")
        assert_eq(inv_a["productId"], product_a_id, "库存.productId")
        assert_eq(inv_a["quantity"], 10, "库存.quantity")
        assert_eq(inv_a["alertThreshold"], 0, "库存初始 alertThreshold 应为 0")
        _log.info("库存种子就绪：invId=%s qty=10", inv_a_id)

        # 分页结构额外校验一下（至少能找到 1 条）
        _log.info("再次 GET /inventory 验证分页结构")
        page_full = client.get("/inventory", params={"page": 1, "pageSize": 10})
        assert_page(page_full, min_total=1)

        # ---------- 第 4 步：设告警阈值触发低库存 ----------
        _log.info("PUT /inventory/%s 设 alertThreshold=15（>quantity，预期 status→2）",
                  inv_a_id)
        updated = client.put(f"/inventory/{inv_a_id}", json={"alertThreshold": 15})
        assert_eq(updated["id"], inv_a_id, "updated.id")
        assert_eq(updated["alertThreshold"], 15, "updated.alertThreshold")
        assert_eq(updated["quantity"], 10, "updated.quantity 不应被改动")
        assert_eq(updated["status"], 2, "updated.status 应为 2(低库存)")
        _log.info("低库存状态切换验证通过")

        # ---------- 第 5 步：/alerts 应包含商品 A ----------
        _log.info("GET /inventory/alerts 分页，确认商品 A 在低库存清单中")
        alerts_page = client.get("/inventory/alerts", params={"page": 1, "pageSize": 50})
        assert_page(alerts_page, min_total=1)
        alert_item = next(
            (x for x in (alerts_page.get("list") or []) if x.get("productId") == product_a_id),
            None,
        )
        if alert_item is None:
            raise AssertionError(
                f"/inventory/alerts 未包含商品 A(productId={product_a_id})，"
                f"实际 list={alerts_page.get('list')}"
            )
        assert_eq(alert_item["quantity"], 10, "alert.quantity")
        assert_eq(alert_item["alertThreshold"], 15, "alert.alertThreshold")
        assert_in("productCode", alert_item, "alert.productCode")
        assert_in("productName", alert_item, "alert.productName")
        assert_eq(alert_item["productCode"], product_a_code, "alert.productCode")
        assert_eq(alert_item["productName"], product_a_name, "alert.productName")
        _log.info("告警列表验证通过")

        # ---------- 第 6 步：降阈值解除低库存 ----------
        _log.info("PUT /inventory/%s 设 alertThreshold=5（<quantity，预期 status→1）",
                  inv_a_id)
        cleared = client.put(f"/inventory/{inv_a_id}", json={"alertThreshold": 5})
        assert_eq(cleared["alertThreshold"], 5, "cleared.alertThreshold")
        assert_eq(cleared["status"], 1, "cleared.status 应恢复为 1(正常)")
        _log.info("低库存状态解除验证通过")

        _log.info("再查 /inventory/alerts，商品 A 不应再出现")
        alerts_page2 = client.get("/inventory/alerts", params={"page": 1, "pageSize": 50})
        for it in alerts_page2.get("list") or []:
            if it.get("productId") == product_a_id:
                raise AssertionError(
                    "阈值下降后商品 A 仍出现在告警列表，后端可能没按新阈值重算"
                )
        _log.info("告警列表清空验证通过")

        # ---------- 第 7 步：仓库隔离 ----------
        wh_b_code = _unique_code() + "_b"
        _log.info("创建仓库 B：code=%s", wh_b_code)
        wh_b = client.post("/warehouses", json={
            "code": wh_b_code,
            "name": _unique_name() + "_B",
        }, scoped=False)
        wh_b_id = wh_b["id"]
        _log.info("仓库 B 创建成功：id=%s", wh_b_id)

        _log.info("把 admin 绑到仓库 B（admin 可多仓）")
        client.post(f"/warehouses/{wh_b_id}/users",
                    json={"userIds": [admin_id]}, scoped=False)

        _log.info("切当前仓库到 wh_b，验证库存隔离")
        client.set_warehouse(wh_b_id)

        wh_b_inv = _find_inventory_by_product(client, product_a_id)
        if wh_b_inv is not None:
            raise AssertionError(
                f"⚠ 仓库隔离漏洞：wh_b 的 /inventory 看到了 WH001 的商品 A 行"
            )
        _log.info("wh_b 列表未看到 WH001 的商品 A，隔离通过")

        _log.info("跨仓库 GET /inventory/%s 在 wh_b 下应 404", inv_a_id)
        expect_error(
            lambda: client.get(f"/inventory/{inv_a_id}"),
            http_status=404,
        )
        _log.info("跨仓库访问被正确拒绝")

        # 切回 WH001 继续后续测试
        _log.info("切回 WH001 继续测试")
        client.set_warehouse(seed_wh_id)

        # ---------- 第 8 步：非 admin 读/写权限 ----------
        normal_username = _unique_username()
        normal_password = "InvTest@12345"
        _log.info("创建普通用户：username=%s", normal_username)
        normal_user = client.post("/users", json={
            "username": normal_username,
            "password": normal_password,
            "realName": "库存测试用户",
            "roleId": user_role_id,
        }, scoped=False)
        normal_user_id = normal_user["id"]

        _log.info("绑定普通用户到 WH001（否则 WarehouseScope 中间件会先拦截）")
        client.post(f"/warehouses/{seed_wh_id}/users",
                    json={"userIds": [normal_user_id]}, scoped=False)

        _log.info("切换登录为普通用户，验证读可用 / 写被拦截")
        auth_login_flow.run(client, normal_username, normal_password)
        assert_eq(SESSION.role_code, "user", "切换后角色应为 user")
        client.set_warehouse(seed_wh_id)  # 登录后重置一次仓库头

        _log.info("普通用户 GET /inventory 应成功")
        nu_page = client.get("/inventory", params={"page": 1, "pageSize": 50})
        assert_page(nu_page, min_total=1)
        if _find_inventory_by_product(client, product_a_id) is None:
            raise AssertionError("普通用户 /inventory 未返回商品 A，读接口可能被误拦")
        _log.info("普通用户读接口可用")

        _log.info("普通用户 GET /inventory/alerts 应成功")
        client.get("/inventory/alerts", params={"page": 1, "pageSize": 50})
        _log.info("普通用户读 alerts 可用")

        if not _probe_permission(
            "非admin PUT /inventory/:id",
            lambda: client.put(f"/inventory/{inv_a_id}", json={"alertThreshold": 1}),
        ):
            vulnerabilities.append("非admin可修改库存阈值")

        # ---------- 第 9 步：切回 admin + 错误路径 ----------
        _log.info("切回 admin 继续错误路径测试")
        auth_login_flow.run(client, CONFIG.admin_user, CONFIG.admin_password)
        client.set_warehouse(seed_wh_id)

        _log.info("GET /inventory/999999 期望 404")
        expect_error(
            lambda: client.get("/inventory/999999"),
            http_status=404,
        )

        _log.info("PUT /inventory/999999 期望 404")
        expect_error(
            lambda: client.put("/inventory/999999", json={"alertThreshold": 1}),
            http_status=404,
        )

        _log.info("PUT /inventory/%s alertThreshold=-1 期望 400(binding min=0)", inv_a_id)
        expect_error(
            lambda: client.put(f"/inventory/{inv_a_id}", json={"alertThreshold": -1}),
            http_status=400,
        )

        _log.info("错误路径验证结束")

    finally:
        # ---------- 第 10 步：清理 ----------
        _log.info("===== 开始清理 =====")
        # 保证后面的 DELETE 在 admin 身份下执行
        if not SESSION.is_admin():
            try:
                auth_login_flow.run(client, CONFIG.admin_user, CONFIG.admin_password)
            except Exception as e:  # noqa: BLE001
                _log.warning("清理阶段切回 admin 失败：%s", e)
        client.set_warehouse(seed_wh_id)

        if wh_b_id is not None:
            _log.info("删除仓库 B：id=%s（级联解绑 admin）", wh_b_id)
            try:
                client.delete(f"/warehouses/{wh_b_id}", scoped=False)
            except APIError as e:
                _log.warning("删除 wh_b 失败（忽略）：%s", e)

        if normal_user_id is not None:
            _log.info("删除普通用户：id=%s", normal_user_id)
            try:
                client.delete(f"/users/{normal_user_id}", scoped=False)
            except APIError as e:
                _log.warning("删除普通用户失败（忽略）：%s", e)

        # 商品 A 有库存行时无法硬删（ErrInvalidState 20002），仅尝试
        _log.info("尝试删除商品 A：id=%s（有库存时后端会返回 20002）", product_a_id)
        try:
            client.delete(f"/products/{product_a_id}", scoped=False)
            _log.info("商品 A 删除成功")
        except APIError as e:
            _log.warning("商品 A 无法删除（符合预期，已存在库存行）：%s", e)

        if vulnerabilities:
            _log.warning("⚠ 发现 %d 个权限漏洞: %s",
                         len(vulnerabilities), ", ".join(vulnerabilities))
        else:
            _log.info("权限探测未发现漏洞")

        _log.info("库存列表 + 告警 + 仓库隔离流程验证完成")
