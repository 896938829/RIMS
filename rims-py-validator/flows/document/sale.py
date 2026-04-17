"""销售单 创建→完成→库存校验 流程（PRD 第 7 章 — 销售）。

预期步骤：
     1) 准备：admin 身份、WH001、创建商品 A、入库 20 件建立库存
     2) POST /documents 创建销售单（docType=2），line qty=5
     3) GET /documents/:id 验证 status=1(草稿)、docNo 前缀 XS
     4) POST /documents/:id/complete 完成销售
     5) GET /inventory 验证库存减少（20→15）
     6) GET /transactions 验证流水（direction=-1, qty=5）
     7) 第二次销售 qty=5 → 库存 10
     8) 超卖测试：qty=20 → ErrInsufficientStock
     9) 普通用户：可创建+完成销售、costPrice 不可见
    10) 错误路径：重复完成(20002)、空行(400)、qty=0(400)
    11) 清理：删普通用户；商品有库存时无法删除仅记录

测试原则：模拟用户操作，寻找后端漏洞。
"""

from __future__ import annotations

import time
from typing import List, Optional

from core.assertions import assert_eq, assert_in, assert_page, expect_error
from core.client import APIClient, APIError
from core.config import CONFIG
from core.logger import get_logger
from core.session import SESSION
from flows.auth import login as auth_login_flow

_log = get_logger("flow.document.sale")


# --------------- 工具函数 ---------------


def _unique_code() -> str:
    """生成唯一商品编码。"""
    return f"sale_{int(time.time() * 1000)}"


def _unique_name() -> str:
    """生成唯一商品名称。"""
    return f"销售测试商品_{int(time.time() * 1000)}"


def _unique_username() -> str:
    """生成唯一测试用户名。"""
    return f"sale_test_user_{int(time.time() * 1000)}"


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


def _create_inbound_and_complete(
    client: APIClient, product_id: int, qty: int,
) -> dict:
    """创建并完成一张入库单，用于为销售测试准备库存。"""
    _log.info("创建入库单为销售准备库存：productId=%s qty=%s", product_id, qty)
    doc = client.post("/documents", json={
        "docType": 1,
        "remark": "销售测试前置入库",
        "lines": [{"productId": product_id, "quantity": qty}],
    })
    client.post(f"/documents/{doc['id']}/complete")
    _log.info("入库完成：docId=%s docNo=%s", doc["id"], doc["docNo"])
    return doc


# --------------- 主流程 ---------------


def run(client: APIClient) -> None:  # noqa: C901
    """执行销售单 创建→完成→库存&流水校验 全流程验证。

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

    # 创建测试商品
    product_code = _unique_code()
    product_name = _unique_name()
    _log.info("创建测试商品：code=%s name=%s", product_code, product_name)
    product = client.post("/products", json={
        "code": product_code,
        "name": product_name,
        "unit": "件",
        "retailPrice": 100.0,
        "costPrice": 60.0,
    }, scoped=False)
    product_id = product["id"]
    _log.info("商品创建成功：id=%s", product_id)

    normal_user_id: Optional[int] = None

    try:
        # 入库 20 件建立库存
        _create_inbound_and_complete(client, product_id, 20)

        inv_before = _find_inventory_by_product(client, product_id)
        if inv_before is None:
            raise AssertionError("入库后未找到库存行")
        assert_eq(inv_before["quantity"], 20, "入库后库存.quantity")
        _log.info("入库准备完成，库存 qty=20")

        # ---------- 第 2 步：创建销售单 ----------
        _log.info("POST /documents 创建销售单：docType=2 qty=5 retailPrice=100")
        doc = client.post("/documents", json={
            "docType": 2,
            "remark": "销售测试",
            "lines": [{
                "productId": product_id,
                "quantity": 5,
                "retailPrice": 100.0,
            }],
        })
        doc_id = doc["id"]
        doc_no = doc["docNo"]
        _log.info("销售单创建成功：id=%s docNo=%s", doc_id, doc_no)

        # ---------- 第 3 步：验证草稿状态 ----------
        assert_eq(doc["status"], 1, "销售单.status 应为 1(草稿)")
        assert_eq(doc["docType"], 2, "销售单.docType 应为 2(销售)")
        if not doc_no.startswith("XS"):
            raise AssertionError(f"销售单 docNo 应以 XS 开头，实际 {doc_no!r}")

        _log.info("GET /documents/%s 详情回查", doc_id)
        detail = client.get(f"/documents/{doc_id}")
        assert_eq(detail["status"], 1, "详情.status")
        assert_in("lines", detail, "详情应含 lines")
        lines = detail["lines"]
        assert_eq(len(lines), 1, "lines 长度")
        assert_eq(lines[0]["productId"], product_id, "line.productId")
        assert_eq(lines[0]["quantity"], 5, "line.quantity")
        _log.info("销售单详情验证通过")

        # ---------- 第 4 步：完成销售 ----------
        _log.info("POST /documents/%s/complete 完成销售", doc_id)
        client.post(f"/documents/{doc_id}/complete")
        _log.info("销售完成")

        completed = client.get(f"/documents/{doc_id}")
        assert_eq(completed["status"], 2, "完成后.status 应为 2")
        assert_in("operatedAt", completed, "完成后应有 operatedAt")

        # ---------- 第 5 步：验证库存减少 ----------
        _log.info("验证库存减少：预期 20→15")
        inv_after = _find_inventory_by_product(client, product_id)
        if inv_after is None:
            raise AssertionError("销售后库存行消失")
        assert_eq(inv_after["quantity"], 15, "销售后库存.quantity")
        _log.info("销售后库存验证通过：qty=15")

        # ---------- 第 6 步：验证流水 ----------
        _log.info("GET /transactions 检查销售流水")
        txn_page = client.get("/transactions", params={
            "page": 1, "pageSize": 50, "keyword": doc_no,
        })
        assert_page(txn_page, min_total=1)
        doc_txns = [t for t in (txn_page.get("list") or []) if t.get("docId") == doc_id]
        if not doc_txns:
            raise AssertionError(f"未找到 docId={doc_id} 的流水")
        txn = doc_txns[0]
        assert_eq(txn["direction"], -1, "销售流水.direction 应为 -1(出)")
        assert_eq(txn["quantity"], 5, "销售流水.quantity")
        _log.info("销售流水验证通过")

        # ---------- 第 7 步：第二次销售 ----------
        _log.info("创建第二张销售单 qty=5，验证库存继续减少")
        doc2 = client.post("/documents", json={
            "docType": 2,
            "remark": "销售第二单",
            "lines": [{"productId": product_id, "quantity": 5, "retailPrice": 100.0}],
        })
        doc2_id = doc2["id"]
        client.post(f"/documents/{doc2_id}/complete")
        _log.info("第二张销售单完成")

        inv_after2 = _find_inventory_by_product(client, product_id)
        if inv_after2 is None:
            raise AssertionError("二次销售后库存行消失")
        assert_eq(inv_after2["quantity"], 10, "二次销售后库存.quantity 应为 10")
        _log.info("二次销售库存验证通过：qty=10")

        # ---------- 第 8 步：超卖测试 ----------
        _log.info("创建超卖销售单 qty=20（库存仅 10），期望 complete 时报错")
        oversell_doc = client.post("/documents", json={
            "docType": 2,
            "lines": [{"productId": product_id, "quantity": 20, "retailPrice": 100.0}],
        })
        oversell_id = oversell_doc["id"]
        _log.info("超卖单已创建为草稿，尝试 complete")
        expect_error(
            lambda: client.post(f"/documents/{oversell_id}/complete"),
            code=20003,  # ErrInsufficientStock
        )
        _log.info("超卖拦截验证通过（ErrInsufficientStock）")

        # ---------- 第 9 步：普通用户销售测试 ----------
        normal_username = _unique_username()
        normal_password = "SaleTest@12345"
        _log.info("创建普通用户 %s 测试销售权限", normal_username)
        normal_user = client.post("/users", json={
            "username": normal_username,
            "password": normal_password,
            "realName": "销售测试用户",
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

        # 普通用户应能创建并完成销售
        _log.info("普通用户创建销售单 qty=2")
        nu_doc = client.post("/documents", json={
            "docType": 2,
            "lines": [{"productId": product_id, "quantity": 2, "retailPrice": 100.0}],
        })
        nu_doc_id = nu_doc["id"]
        _log.info("普通用户销售单创建成功：id=%s", nu_doc_id)

        client.post(f"/documents/{nu_doc_id}/complete")
        _log.info("普通用户销售完成")

        # 验证库存减少（10→8）
        nu_inv = _find_inventory_by_product(client, product_id)
        if nu_inv is None:
            raise AssertionError("普通用户销售后库存行消失")
        assert_eq(nu_inv["quantity"], 8, "普通用户销售后库存.quantity 应为 8")
        _log.info("普通用户销售后库存验证通过：qty=8")

        # costPrice 不应对普通用户可见
        _log.info("验证普通用户看不到 costPrice")
        nu_detail = client.get(f"/documents/{nu_doc_id}")
        nu_lines = nu_detail.get("lines") or []
        for line in nu_lines:
            if line.get("costPrice") is not None and line["costPrice"] != 0:
                _log.warning("⚠ 权限漏洞: 普通用户可见 costPrice=%s", line["costPrice"])
                vulnerabilities.append("普通用户可见文档行 costPrice")
                break
        else:
            _log.info("costPrice 对普通用户正确隐藏（或为 0）")

        # ---------- 第 10 步：错误路径（切回 admin） ----------
        _log.info("切回 admin 做错误路径测试")
        auth_login_flow.run(client, CONFIG.admin_user, CONFIG.admin_password)
        client.set_warehouse(seed_wh_id)

        _log.info("===== 错误路径测试 =====")

        # 重复完成
        _log.info("重复完成已完成销售单 id=%s，期望 code=20002", doc_id)
        expect_error(
            lambda: client.post(f"/documents/{doc_id}/complete"),
            code=20002,
        )
        _log.info("重复完成拦截通过")

        # 空行
        _log.info("创建销售单空明细，期望 HTTP 400")
        expect_error(
            lambda: client.post("/documents", json={
                "docType": 2,
                "lines": [],
            }),
            http_status=400,
        )

        # qty=0
        _log.info("创建销售单 qty=0，期望 HTTP 400")
        expect_error(
            lambda: client.post("/documents", json={
                "docType": 2,
                "lines": [{"productId": product_id, "quantity": 0}],
            }),
            http_status=400,
        )

        _log.info("===== 错误路径测试结束 =====")

    finally:
        # ---------- 清理 ----------
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

        _log.info("尝试删除商品：id=%s", product_id)
        try:
            client.delete(f"/products/{product_id}", scoped=False)
            _log.info("商品删除成功")
        except APIError as e:
            _log.warning("商品无法删除（符合预期，已存在库存）：%s", e)

        if vulnerabilities:
            _log.warning("⚠ 发现 %d 个权限漏洞: %s",
                         len(vulnerabilities), ", ".join(vulnerabilities))
        else:
            _log.info("权限探测未发现漏洞")

        _log.info("销售单流程验证完成")
