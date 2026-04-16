"""非标库存 CRUD + 转标 + admin 权限流程（PRD 第 4.7 章）。

预期步骤：
     1) 准备：admin 身份、WH001 seed 仓库、user 角色 id；创建目标商品 A
     2) POST /non-std-inventory 创建非标条目（qty=100）
     3) GET /non-std-inventory 列表校验
     4) GET /non-std-inventory/:id 详情字段回查
     5) PUT /non-std-inventory/:id 更新 description + quantity（120）
     6) POST /:id/convert 部分转换 40 → 非标 convertedQty=40, remaining=80, status=2；
        /inventory 应出现 productId=A 的库存行 qty=40
     7) 再次 /convert 超额 999，预期 code=10003（ErrValidation）
     8) /convert 剩余 80，非标 status=3 全部转完；/inventory quantity=120
     9) DELETE 已转换非标，预期 code=20002（ErrInvalidState）
    10) 错误路径：重复 tempLabel / 缺必填 / convert 不存在 productId / 不存在 nsId
        / PUT quantity<=0；另建一个未转换非标可正常 DELETE
    11) 非 admin 权限探测：POST/GET/PUT/DELETE/convert 全部 403
    12) 清理：切回 admin、删普通用户、删商品 A（有库存时 20002 仅记录）

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

_log = get_logger("flow.inventory.non_std")


# --------------- 工具函数 ---------------


def _unique_code() -> str:
    """生成唯一商品编码。"""
    return f"nsprod_{int(time.time() * 1000)}"


def _unique_name() -> str:
    """生成唯一商品名。"""
    return f"非标测试商品_{int(time.time() * 1000)}"


def _unique_username() -> str:
    """生成唯一测试用户名。"""
    return f"ns_test_user_{int(time.time() * 1000)}"


def _unique_temp_label() -> str:
    """生成唯一非标临时标签号。"""
    return f"NST_CONV_{int(time.time() * 1000)}"


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


def _find_inventory_by_product(client: APIClient, product_id: int) -> Optional[dict]:
    """在当前仓库库存分页中定位商品库存行。"""
    page = client.get("/inventory", params={"page": 1, "pageSize": 100})
    for inv in page.get("list") or []:
        if inv.get("productId") == product_id:
            return inv
    return None


def _probe_permission(label: str, fn: Callable[[], object]) -> bool:
    """探测非 admin 是否被正确拦截（期望 HTTP 403）。"""
    try:
        expect_error(fn, http_status=403)
        _log.info("权限拦截正常：%s", label)
        return True
    except AssertionError:
        _log.warning("⚠ 权限漏洞: %s — 非 admin 未被拦截 (期望 HTTP 403)", label)
        return False


# --------------- 主流程 ---------------


def run(client: APIClient) -> None:  # noqa: C901
    """执行非标库存 CRUD + 转标 + 权限全流程验证。

    前置条件：调用方（main.py._register_todo）已以 admin 身份登录。
    """
    vulnerabilities: List[str] = []

    # ---------- 第 1 步：准备 ----------
    admin_id = SESSION.user.get("id")
    if admin_id is None:
        raise AssertionError("SESSION.user.id 为空，疑似未登录")
    _log.info("当前 admin：id=%s username=%s", admin_id, SESSION.user.get("username"))

    seed_wh_id = _find_seed_warehouse_id(client, "WH001")
    _log.info("seed 仓库 WH001.id=%s", seed_wh_id)
    client.set_warehouse(seed_wh_id)

    user_role_id = _get_role_id_by_code(client, "user")

    # 创建转换目标商品 A
    product_a_code = _unique_code()
    product_a_name = _unique_name()
    _log.info("创建目标商品 A：code=%s name=%s", product_a_code, product_a_name)
    product_a = client.post("/products", json={
        "code": product_a_code,
        "name": product_a_name,
        "unit": "件",
        "retailPrice": 20.0,
        "costPrice": 10.0,
    }, scoped=False)
    product_a_id = product_a["id"]
    _log.info("商品 A 创建成功：id=%s", product_a_id)

    normal_user_id: Optional[int] = None

    try:
        # ---------- 第 2 步：创建非标条目 ----------
        temp_label = _unique_temp_label()
        _log.info("POST /non-std-inventory 创建非标：tempLabel=%s qty=100", temp_label)
        ns = client.post("/non-std-inventory", json={
            "tempLabel": temp_label,
            "description": "批量入库",
            "unit": "件",
            "quantity": 100,
            "sourceMethod": "purchase",
            "sourceDocument": "PO-TEST",
        })
        assert_in("id", ns, "非标.id")
        ns_id = ns["id"]
        assert_eq(ns["warehouseId"], seed_wh_id, "非标.warehouseId")
        assert_eq(ns["tempLabel"], temp_label, "非标.tempLabel")
        assert_eq(ns["quantity"], 100, "非标.quantity")
        assert_eq(ns["convertedQty"], 0, "非标.convertedQty")
        assert_eq(ns["remainingQty"], 100, "非标.remainingQty")
        assert_eq(ns["status"], 1, "非标.status 应为 1(正常)")
        _log.info("非标条目创建成功：id=%s", ns_id)

        # ---------- 第 3 步：列表 ----------
        _log.info("GET /non-std-inventory 列表，确认新条目出现")
        ns_page = client.get("/non-std-inventory", params={"page": 1, "pageSize": 50})
        assert_page(ns_page, min_total=1)
        if not any(x.get("id") == ns_id for x in ns_page.get("list") or []):
            raise AssertionError(f"非标列表未包含刚创建的 id={ns_id}")
        _log.info("列表验证通过")

        # ---------- 第 4 步：详情 ----------
        _log.info("GET /non-std-inventory/%s 详情回查", ns_id)
        ns_detail = client.get(f"/non-std-inventory/{ns_id}")
        assert_eq(ns_detail["id"], ns_id, "详情.id")
        assert_eq(ns_detail["tempLabel"], temp_label, "详情.tempLabel")
        assert_eq(ns_detail["quantity"], 100, "详情.quantity")
        assert_eq(ns_detail["convertedQty"], 0, "详情.convertedQty")
        assert_eq(ns_detail["sourceMethod"], "purchase", "详情.sourceMethod")
        assert_eq(ns_detail["sourceDocument"], "PO-TEST", "详情.sourceDocument")
        _log.info("详情验证通过")

        # ---------- 第 5 步：修改描述 + 数量 ----------
        new_desc = "更新后的描述"
        _log.info("PUT /non-std-inventory/%s 更新 description + quantity=120", ns_id)
        ns_updated = client.put(f"/non-std-inventory/{ns_id}", json={
            "description": new_desc,
            "quantity": 120,
        })
        assert_eq(ns_updated["description"], new_desc, "修改后.description")
        assert_eq(ns_updated["quantity"], 120, "修改后.quantity")
        assert_eq(ns_updated["convertedQty"], 0, "修改后.convertedQty 不变")
        assert_eq(ns_updated["remainingQty"], 120, "修改后.remainingQty")
        _log.info("修改验证通过")

        # ---------- 第 6 步：部分转换 40 ----------
        _log.info("POST /non-std-inventory/%s/convert qty=40 → 商品 A", ns_id)
        client.post(f"/non-std-inventory/{ns_id}/convert", json={
            "productId": product_a_id,
            "quantity": 40,
        })
        _log.info("部分转换调用完成，回查非标与标准库存")

        ns_after_40 = client.get(f"/non-std-inventory/{ns_id}")
        assert_eq(ns_after_40["convertedQty"], 40, "转换后.convertedQty")
        assert_eq(ns_after_40["remainingQty"], 80, "转换后.remainingQty")
        assert_eq(ns_after_40["status"], 2, "转换后.status 应为 2(部分已转)")

        inv_a = _find_inventory_by_product(client, product_a_id)
        if inv_a is None:
            raise AssertionError(
                f"转换后 /inventory 未找到商品 A 库存行 productId={product_a_id}"
            )
        assert_eq(inv_a["quantity"], 40, "标准库存.quantity 首轮转换后")
        assert_eq(inv_a["warehouseId"], seed_wh_id, "标准库存.warehouseId")
        _log.info("部分转换验证通过：非标已转 40，标准库存 qty=40")

        # ---------- 第 7 步：超额转换应被拒绝 ----------
        _log.info("POST /convert qty=999 超过剩余 80，期望 code=10003")
        expect_error(
            lambda: client.post(f"/non-std-inventory/{ns_id}/convert", json={
                "productId": product_a_id,
                "quantity": 999,
            }),
            code=10003,
        )
        _log.info("超额转换拦截验证通过")

        # ---------- 第 8 步：转完剩余 80 ----------
        _log.info("POST /convert qty=80，期望全部转完 status=3")
        client.post(f"/non-std-inventory/{ns_id}/convert", json={
            "productId": product_a_id,
            "quantity": 80,
        })
        ns_after_all = client.get(f"/non-std-inventory/{ns_id}")
        assert_eq(ns_after_all["convertedQty"], 120, "全部转换后.convertedQty")
        assert_eq(ns_after_all["remainingQty"], 0, "全部转换后.remainingQty")
        assert_eq(ns_after_all["status"], 3, "全部转换后.status 应为 3(全部已转)")

        inv_a_full = _find_inventory_by_product(client, product_a_id)
        if inv_a_full is None:
            raise AssertionError("全部转换后 /inventory 未找到商品 A 库存行")
        assert_eq(inv_a_full["quantity"], 120, "标准库存.quantity 全部转换后")
        _log.info("全部转换验证通过：非标 status=3，标准库存 qty=120")

        # ---------- 第 9 步：删除已转换非标被拒 ----------
        _log.info("DELETE 已转换非标 id=%s，期望 code=20002", ns_id)
        expect_error(
            lambda: client.delete(f"/non-std-inventory/{ns_id}"),
            code=20002,
        )
        _log.info("已转换非标无法删除拦截验证通过")

        # ---------- 第 10 步：错误路径 ----------
        _log.info("===== 错误路径测试 =====")

        _log.info("重复 tempLabel 创建，期望 code=10005")
        expect_error(
            lambda: client.post("/non-std-inventory", json={
                "tempLabel": temp_label,  # 已存在
                "description": "重复创建",
                "unit": "件",
                "quantity": 1,
            }),
            code=10005,
        )

        _log.info("缺必填字段，期望 HTTP 400")
        expect_error(
            lambda: client.post("/non-std-inventory", json={
                "tempLabel": _unique_temp_label(),
                # 缺 description / unit / quantity
            }),
            http_status=400,
        )

        # 注意：ns_id 已 status=3 全部转完，直接对它调 convert 会命中 20002（状态不允许）
        # 在 product 404 检查之前；所以必须另建一个未转换的"守卫"非标来专测 404 分支。
        guard_label = _unique_temp_label()
        _log.info("建一个未转换的守卫非标 %s，测 convert 不存在商品 id 的 404", guard_label)
        guard_ns = client.post("/non-std-inventory", json={
            "tempLabel": guard_label,
            "description": "guard",
            "unit": "件",
            "quantity": 5,
        })
        guard_ns_id = guard_ns["id"]
        try:
            expect_error(
                lambda: client.post(f"/non-std-inventory/{guard_ns_id}/convert", json={
                    "productId": 999999,
                    "quantity": 1,
                }),
                http_status=404,
            )
            _log.info("不存在商品 id convert 404 验证通过")
        finally:
            _log.info("守卫非标未转换，直接 DELETE 清理")
            try:
                client.delete(f"/non-std-inventory/{guard_ns_id}")
            except APIError as e:
                _log.warning("守卫非标清理失败（忽略）：%s", e)

        _log.info("convert 不存在的 nsId=999999，期望 HTTP 404")
        expect_error(
            lambda: client.post("/non-std-inventory/999999/convert", json={
                "productId": product_a_id,
                "quantity": 1,
            }),
            http_status=404,
        )

        # PUT quantity 非法
        temp_put_label = _unique_temp_label()
        _log.info("再建一个未转非标 %s 来测非法 PUT 数量", temp_put_label)
        put_ns = client.post("/non-std-inventory", json={
            "tempLabel": temp_put_label,
            "description": "put-test",
            "unit": "件",
            "quantity": 5,
        })
        put_ns_id = put_ns["id"]
        try:
            _log.info("PUT quantity=0 期望 400（binding min=1）")
            expect_error(
                lambda: client.put(f"/non-std-inventory/{put_ns_id}",
                                   json={"quantity": 0}),
                http_status=400,
            )
            _log.info("PUT quantity=-1 期望 400")
            expect_error(
                lambda: client.put(f"/non-std-inventory/{put_ns_id}",
                                   json={"quantity": -1}),
                http_status=400,
            )
        finally:
            _log.info("未转非标可正常 DELETE，做用例收尾")
            try:
                client.delete(f"/non-std-inventory/{put_ns_id}")
                _log.info("未转换非标 DELETE 成功")
            except APIError as e:
                _log.warning("未转非标 DELETE 失败（异常）：%s", e)

        _log.info("===== 错误路径测试结束 =====")

        # ---------- 第 11 步：非 admin 权限探测 ----------
        normal_username = _unique_username()
        normal_password = "NsTest@12345"
        _log.info("创建普通用户 %s 做权限探测", normal_username)
        normal_user = client.post("/users", json={
            "username": normal_username,
            "password": normal_password,
            "realName": "非标测试用户",
            "roleId": user_role_id,
        }, scoped=False)
        normal_user_id = normal_user["id"]

        _log.info("绑定普通用户到 WH001（避免被 WarehouseScope 提前 403）")
        client.post(f"/warehouses/{seed_wh_id}/users",
                    json={"userIds": [normal_user_id]}, scoped=False)

        _log.info("切换登录为普通用户")
        auth_login_flow.run(client, normal_username, normal_password)
        assert_eq(SESSION.role_code, "user", "切换后角色应为 user")
        client.set_warehouse(seed_wh_id)

        _log.info("===== 非 admin 权限探测 =====")

        if not _probe_permission(
            "非admin POST /non-std-inventory",
            lambda: client.post("/non-std-inventory", json={
                "tempLabel": _unique_temp_label(),
                "description": "non-admin",
                "unit": "件",
                "quantity": 1,
            }),
        ):
            vulnerabilities.append("非admin可创建非标")

        if not _probe_permission(
            "非admin GET /non-std-inventory 列表",
            lambda: client.get("/non-std-inventory", params={"page": 1, "pageSize": 10}),
        ):
            vulnerabilities.append("非admin可读非标列表")

        if not _probe_permission(
            f"非admin GET /non-std-inventory/{ns_id}",
            lambda: client.get(f"/non-std-inventory/{ns_id}"),
        ):
            vulnerabilities.append("非admin可读非标详情")

        if not _probe_permission(
            f"非admin PUT /non-std-inventory/{ns_id}",
            lambda: client.put(f"/non-std-inventory/{ns_id}",
                               json={"description": "tampered"}),
        ):
            vulnerabilities.append("非admin可修改非标")

        if not _probe_permission(
            f"非admin DELETE /non-std-inventory/{ns_id}",
            lambda: client.delete(f"/non-std-inventory/{ns_id}"),
        ):
            vulnerabilities.append("非admin可删除非标")

        if not _probe_permission(
            f"非admin POST /non-std-inventory/{ns_id}/convert",
            lambda: client.post(f"/non-std-inventory/{ns_id}/convert", json={
                "productId": product_a_id,
                "quantity": 1,
            }),
        ):
            vulnerabilities.append("非admin可转标")

        _log.info("===== 权限探测结束 =====")

    finally:
        # ---------- 第 12 步：清理 ----------
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

        # 商品 A 已被转换的 120 件堆到了标准库存，删除会 20002；尝试并降级
        _log.info("尝试删除商品 A：id=%s（有库存时后端返回 20002）", product_a_id)
        try:
            client.delete(f"/products/{product_a_id}", scoped=False)
            _log.info("商品 A 删除成功")
        except APIError as e:
            _log.warning("商品 A 无法删除（符合预期，存在标准库存）：%s", e)

        if vulnerabilities:
            _log.warning("⚠ 发现 %d 个权限漏洞: %s",
                         len(vulnerabilities), ", ".join(vulnerabilities))
        else:
            _log.info("权限探测未发现漏洞")

        _log.info("非标库存 CRUD + 转标 + 权限流程验证完成")
