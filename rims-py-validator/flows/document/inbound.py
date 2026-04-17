"""入库单 创建→完成→库存校验 流程（PRD 第 6 章 — 入库）。

预期步骤：
     1) 准备：admin 身份、WH001 seed 仓库、创建测试商品 A
     2) POST /documents 创建入库单（docType=1），明细行 qty=10
     3) GET /documents/:id 确认 status=1(草稿)、docNo 前缀 RK、lines 正确
     4) GET /documents?docType=1 列表确认出现
     5) POST /documents/:id/complete 完成入库
     6) GET /documents/:id 确认 status=2(已完成)
     7) GET /inventory 验证商品 A 库存增加 10
     8) GET /transactions 验证流水记录（direction=1, qty=10）
     9) 第二次入库 qty=5 → 完成 → 库存应为 15
    10) 错误路径：重复完成已完成单据(20002)、空明细行(400)、qty=0(400)
    11) 权限探测：普通用户创建+完成入库单（PRD 说 admin-only，
        但后端 handler 未 gate 入库——记录漏洞而非 assert）
    12) 清理：删普通用户；商品 A 有库存时无法删除仅记录

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

_log = get_logger("flow.document.inbound")


# --------------- 工具函数 ---------------


def _unique_code() -> str:
    """生成唯一商品编码。"""
    return f"inb_{int(time.time() * 1000)}"


def _unique_name() -> str:
    """生成唯一商品名称。"""
    return f"入库测试商品_{int(time.time() * 1000)}"


def _unique_username() -> str:
    """生成唯一测试用户名。"""
    return f"inb_test_user_{int(time.time() * 1000)}"


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
    """执行入库单 创建→完成→库存校验 全流程验证。

    前置条件：调用方（main.py._register_todo）已以 admin 身份登录。
    """
    vulnerabilities: List[str] = []

    # ---------- 第 1 步：环境准备 ----------
    admin_id = SESSION.user.get("id")
    if admin_id is None:
        raise AssertionError("SESSION.user.id 为空，疑似未登录")
    _log.info("当前 admin：id=%s username=%s", admin_id, SESSION.user.get("username"))

    seed_wh_id = _find_seed_warehouse_id(client, "WH001")
    _log.info("seed 仓库 WH001.id=%s", seed_wh_id)
    client.set_warehouse(seed_wh_id)

    user_role_id = _get_role_id_by_code(client, "user")

    # 创建测试商品 A
    product_code = _unique_code()
    product_name = _unique_name()
    _log.info("创建测试商品：code=%s name=%s", product_code, product_name)
    product = client.post("/products", json={
        "code": product_code,
        "name": product_name,
        "unit": "件",
        "retailPrice": 50.0,
        "costPrice": 25.0,
    }, scoped=False)
    product_id = product["id"]
    _log.info("商品创建成功：id=%s", product_id)

    normal_user_id: Optional[int] = None

    try:
        # ---------- 第 2 步：创建入库单 ----------
        _log.info("POST /documents 创建入库单：docType=1 productId=%s qty=10", product_id)
        doc = client.post("/documents", json={
            "docType": 1,
            "remark": "入库测试单据",
            "lines": [{
                "productId": product_id,
                "quantity": 10,
            }],
        })
        doc_id = doc["id"]
        doc_no = doc["docNo"]
        _log.info("入库单创建成功：id=%s docNo=%s", doc_id, doc_no)

        # ---------- 第 3 步：验证草稿状态和字段 ----------
        assert_eq(doc["status"], 1, "入库单.status 应为 1(草稿)")
        assert_eq(doc["docType"], 1, "入库单.docType 应为 1(入库)")
        if not doc_no.startswith("RK"):
            raise AssertionError(f"入库单 docNo 应以 RK 开头，实际 {doc_no!r}")
        assert_eq(doc["warehouseId"], seed_wh_id, "入库单.warehouseId")

        _log.info("GET /documents/%s 详情回查（含 lines）", doc_id)
        detail = client.get(f"/documents/{doc_id}")
        assert_eq(detail["id"], doc_id, "详情.id")
        assert_eq(detail["status"], 1, "详情.status")
        assert_in("lines", detail, "详情应含 lines")
        lines = detail["lines"]
        assert_eq(len(lines), 1, "详情.lines 长度")
        assert_eq(lines[0]["productId"], product_id, "line.productId")
        assert_eq(lines[0]["quantity"], 10, "line.quantity")
        assert_in("productCode", lines[0], "line.productCode（应被反查填充）")
        assert_in("productName", lines[0], "line.productName（应被反查填充）")
        _log.info("入库单详情验证通过")

        # ---------- 第 4 步：列表查询 ----------
        _log.info("GET /documents?docType=1 查询入库单列表")
        page = client.get("/documents", params={"docType": 1, "page": 1, "pageSize": 50})
        assert_page(page, min_total=1)
        found = any(d.get("id") == doc_id for d in page.get("list") or [])
        if not found:
            raise AssertionError(f"入库单列表未包含刚创建的 id={doc_id}")
        _log.info("入库单列表验证通过")

        # ---------- 第 5 步：完成入库 ----------
        _log.info("POST /documents/%s/complete 完成入库", doc_id)
        client.post(f"/documents/{doc_id}/complete")
        _log.info("入库单完成调用成功")

        # ---------- 第 6 步：确认完成状态 ----------
        _log.info("GET /documents/%s 确认状态变更", doc_id)
        completed = client.get(f"/documents/{doc_id}")
        assert_eq(completed["status"], 2, "完成后.status 应为 2(已完成)")
        assert_in("operatedAt", completed, "完成后应有 operatedAt")
        _log.info("入库单完成状态验证通过")

        # ---------- 第 7 步：验证库存变化 ----------
        _log.info("GET /inventory 检查商品库存是否增加")
        inv = _find_inventory_by_product(client, product_id)
        if inv is None:
            raise AssertionError(f"入库完成后 /inventory 未找到 productId={product_id}")
        assert_eq(inv["quantity"], 10, "入库后库存.quantity")
        assert_eq(inv["warehouseId"], seed_wh_id, "库存.warehouseId")
        _log.info("入库后库存验证通过：qty=10")

        # ---------- 第 8 步：验证流水记录 ----------
        _log.info("GET /transactions 检查入库流水")
        txn_page = client.get("/transactions", params={
            "page": 1, "pageSize": 50, "keyword": doc_no,
        })
        assert_page(txn_page, min_total=1)
        txn_list = txn_page.get("list") or []
        # 找到本单据的流水
        doc_txns = [t for t in txn_list if t.get("docId") == doc_id]
        if not doc_txns:
            raise AssertionError(f"未找到 docId={doc_id} 的流水记录")
        txn = doc_txns[0]
        assert_eq(txn["direction"], 1, "入库流水.direction 应为 1(入)")
        assert_eq(txn["quantity"], 10, "入库流水.quantity")
        assert_eq(txn["productId"], product_id, "入库流水.productId")
        assert_eq(txn["afterQty"], 10, "入库流水.afterQty（从 0 到 10）")
        _log.info("入库流水验证通过")

        # ---------- 第 9 步：第二次入库，验证累加 ----------
        _log.info("创建第二张入库单 qty=5，验证库存累加")
        doc2 = client.post("/documents", json={
            "docType": 1,
            "remark": "入库第二批",
            "lines": [{
                "productId": product_id,
                "quantity": 5,
            }],
        })
        doc2_id = doc2["id"]
        _log.info("第二张入库单创建成功：id=%s docNo=%s", doc2_id, doc2["docNo"])

        client.post(f"/documents/{doc2_id}/complete")
        _log.info("第二张入库单完成")

        inv2 = _find_inventory_by_product(client, product_id)
        if inv2 is None:
            raise AssertionError("第二次入库后库存行消失")
        assert_eq(inv2["quantity"], 15, "二次入库后库存.quantity 应为 15")
        _log.info("库存累加验证通过：qty=15")

        # ---------- 第 10 步：错误路径 ----------
        _log.info("===== 错误路径测试 =====")

        # 重复完成已完成单据
        _log.info("重复完成已完成入库单 id=%s，期望 ErrInvalidState", doc_id)
        expect_error(
            lambda: client.post(f"/documents/{doc_id}/complete"),
            code=20002,
        )
        _log.info("重复完成拦截验证通过")

        # 空明细行
        _log.info("创建入库单但不传 lines，期望 HTTP 400")
        expect_error(
            lambda: client.post("/documents", json={
                "docType": 1,
                "remark": "无明细",
                "lines": [],
            }),
            http_status=400,
        )
        _log.info("空明细行拦截验证通过")

        # qty=0 应被 binding 拦截
        _log.info("创建入库单 line.quantity=0，期望 HTTP 400")
        expect_error(
            lambda: client.post("/documents", json={
                "docType": 1,
                "lines": [{"productId": product_id, "quantity": 0}],
            }),
            http_status=400,
        )
        _log.info("quantity=0 拦截验证通过")

        # 完成不存在的单据
        _log.info("完成不存在的单据 id=999999，期望 404")
        expect_error(
            lambda: client.post("/documents/999999/complete"),
            http_status=404,
        )
        _log.info("不存在单据完成拦截验证通过")

        _log.info("===== 错误路径测试结束 =====")

        # ---------- 第 11 步：权限探测 ----------
        # PRD 规定入库是 admin-only，但后端 handler 对 inbound complete 没有 admin gate
        # 这里测试并记录结果（漏洞探测而非硬断言）
        normal_username = _unique_username()
        normal_password = "InbTest@12345"
        _log.info("创建普通用户 %s 做权限探测", normal_username)
        normal_user = client.post("/users", json={
            "username": normal_username,
            "password": normal_password,
            "realName": "入库测试用户",
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

        _log.info("===== 非 admin 入库权限探测 =====")

        # 普通用户尝试创建入库单
        if not _probe_permission(
            "非admin POST /documents (docType=1 入库)",
            lambda: client.post("/documents", json={
                "docType": 1,
                "lines": [{"productId": product_id, "quantity": 1}],
            }),
        ):
            vulnerabilities.append("非admin可创建入库单")

        # 如果上面创建成功了，尝试完成（用一个新的草稿单）
        # 切回 admin 创建一个草稿，再切回普通用户完成
        _log.info("切回 admin 创建一个草稿入库单供权限探测")
        auth_login_flow.run(client, CONFIG.admin_user, CONFIG.admin_password)
        client.set_warehouse(seed_wh_id)
        probe_doc = client.post("/documents", json={
            "docType": 1,
            "lines": [{"productId": product_id, "quantity": 1}],
        })
        probe_doc_id = probe_doc["id"]

        auth_login_flow.run(client, normal_username, normal_password)
        client.set_warehouse(seed_wh_id)

        if not _probe_permission(
            "非admin POST /documents/:id/complete (入库)",
            lambda: client.post(f"/documents/{probe_doc_id}/complete"),
        ):
            vulnerabilities.append("非admin可完成入库单")

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

        _log.info("尝试删除商品：id=%s（有库存时后端返回 20002）", product_id)
        try:
            client.delete(f"/products/{product_id}", scoped=False)
            _log.info("商品删除成功")
        except APIError as e:
            _log.warning("商品无法删除（符合预期，已存在库存行）：%s", e)

        if vulnerabilities:
            _log.warning("⚠ 发现 %d 个权限漏洞: %s",
                         len(vulnerabilities), ", ".join(vulnerabilities))
        else:
            _log.info("权限探测未发现漏洞")

        _log.info("入库单流程验证完成")
