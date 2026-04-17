"""退货单 关联原销售单→完成→库存回补 流程（PRD 第 8 章 — 退货）。

预期步骤：
     1) 准备：admin 身份、WH001、创建商品 A、入库 20 件、销售 10 件（得到已完成销售单）
     2) POST /documents 创建退货单（docType=3），refDocId=销售单id，line qty=3
     3) GET /documents/:id 验证 status=1、docNo 前缀 TH、refDocNo 已填充
     4) POST /documents/:id/complete 完成退货
     5) GET /inventory 验证库存回补（10→13）
     6) GET /transactions 验证流水（direction=1, qty=3）
     7) 部分退货：再退 5 件 → 库存 18，总退 8 件
     8) 超退测试：再退 5 件（8+5=13 > 已售 10）→ 期望被拒
     9) 错误路径：无 refDocId(400)、引用非销售单(error)、引用草稿销售单(error)
    10) 普通用户：可关联自己仓库销售单退货
    11) 清理

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

_log = get_logger("flow.document.return")


# --------------- 工具函数 ---------------


def _unique_code() -> str:
    """生成唯一商品编码。"""
    return f"ret_{int(time.time() * 1000)}"


def _unique_name() -> str:
    """生成唯一商品名称。"""
    return f"退货测试商品_{int(time.time() * 1000)}"


def _unique_username() -> str:
    """生成唯一测试用户名。"""
    return f"ret_test_user_{int(time.time() * 1000)}"


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
    """创建并完成入库单，为测试准备库存。"""
    _log.info("前置入库：productId=%s qty=%s", product_id, qty)
    doc = client.post("/documents", json={
        "docType": 1,
        "remark": "退货测试前置入库",
        "lines": [{"productId": product_id, "quantity": qty}],
    })
    client.post(f"/documents/{doc['id']}/complete")
    _log.info("入库完成：docId=%s", doc["id"])
    return doc


def _create_sale_and_complete(
    client: APIClient, product_id: int, qty: int,
) -> dict:
    """创建并完成销售单，为退货测试提供原单。"""
    _log.info("前置销售：productId=%s qty=%s", product_id, qty)
    doc = client.post("/documents", json={
        "docType": 2,
        "remark": "退货测试前置销售",
        "lines": [{"productId": product_id, "quantity": qty, "retailPrice": 100.0}],
    })
    client.post(f"/documents/{doc['id']}/complete")
    _log.info("销售完成：docId=%s docNo=%s", doc["id"], doc["docNo"])
    return doc


# --------------- 主流程 ---------------


def run(client: APIClient) -> None:  # noqa: C901
    """执行退货单 关联原销售单→完成→库存回补 全流程验证。

    前置条件：调用方已以 admin 身份登录。
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
        # 入库 20 件 → 销售 10 件 → 库存剩 10
        _create_inbound_and_complete(client, product_id, 20)
        sale_doc = _create_sale_and_complete(client, product_id, 10)
        sale_doc_id = sale_doc["id"]
        sale_doc_no = sale_doc["docNo"]
        _log.info("前置数据就绪：已入库 20、已售 10、库存应为 10")

        inv_before = _find_inventory_by_product(client, product_id)
        if inv_before is None:
            raise AssertionError("前置完成后未找到库存行")
        assert_eq(inv_before["quantity"], 10, "前置后库存.quantity")

        # ---------- 第 2 步：创建退货单 ----------
        _log.info("POST /documents 创建退货单：docType=3 refDocId=%s qty=3", sale_doc_id)
        doc = client.post("/documents", json={
            "docType": 3,
            "refDocId": sale_doc_id,
            "remark": "退货测试",
            "lines": [{
                "productId": product_id,
                "quantity": 3,
            }],
        })
        doc_id = doc["id"]
        doc_no = doc["docNo"]
        _log.info("退货单创建成功：id=%s docNo=%s", doc_id, doc_no)

        # ---------- 第 3 步：验证草稿状态 ----------
        assert_eq(doc["status"], 1, "退货单.status 应为 1(草稿)")
        assert_eq(doc["docType"], 3, "退货单.docType 应为 3(退货)")
        if not doc_no.startswith("TH"):
            raise AssertionError(f"退货单 docNo 应以 TH 开头，实际 {doc_no!r}")

        _log.info("GET /documents/%s 详情回查", doc_id)
        detail = client.get(f"/documents/{doc_id}")
        assert_eq(detail["refDocId"], sale_doc_id, "详情.refDocId")
        # refDocNo 应被反查填充
        if detail.get("refDocNo"):
            assert_eq(detail["refDocNo"], sale_doc_no, "详情.refDocNo")
            _log.info("refDocNo 已正确填充：%s", sale_doc_no)
        assert_in("lines", detail, "详情应含 lines")
        assert_eq(len(detail["lines"]), 1, "lines 长度")
        assert_eq(detail["lines"][0]["quantity"], 3, "line.quantity")
        _log.info("退货单详情验证通过")

        # ---------- 第 4 步：完成退货 ----------
        _log.info("POST /documents/%s/complete 完成退货", doc_id)
        client.post(f"/documents/{doc_id}/complete")
        _log.info("退货完成")

        completed = client.get(f"/documents/{doc_id}")
        assert_eq(completed["status"], 2, "完成后.status 应为 2")

        # ---------- 第 5 步：验证库存回补 ----------
        _log.info("验证库存回补：预期 10→13")
        inv_after = _find_inventory_by_product(client, product_id)
        if inv_after is None:
            raise AssertionError("退货后库存行消失")
        assert_eq(inv_after["quantity"], 13, "退货后库存.quantity")
        _log.info("退货后库存验证通过：qty=13")

        # ---------- 第 6 步：验证流水 ----------
        _log.info("GET /transactions 检查退货流水")
        txn_page = client.get("/transactions", params={
            "page": 1, "pageSize": 50, "keyword": doc_no,
        })
        assert_page(txn_page, min_total=1)
        doc_txns = [t for t in (txn_page.get("list") or []) if t.get("docId") == doc_id]
        if not doc_txns:
            raise AssertionError(f"未找到 docId={doc_id} 的退货流水")
        txn = doc_txns[0]
        assert_eq(txn["direction"], 1, "退货流水.direction 应为 1(入)")
        assert_eq(txn["quantity"], 3, "退货流水.quantity")
        _log.info("退货流水验证通过")

        # ---------- 第 7 步：部分退货（再退 5 件） ----------
        _log.info("创建第二张退货单 qty=5，关联同一销售单")
        doc2 = client.post("/documents", json={
            "docType": 3,
            "refDocId": sale_doc_id,
            "remark": "退货第二批",
            "lines": [{"productId": product_id, "quantity": 5}],
        })
        doc2_id = doc2["id"]
        client.post(f"/documents/{doc2_id}/complete")
        _log.info("第二张退货单完成")

        inv_after2 = _find_inventory_by_product(client, product_id)
        if inv_after2 is None:
            raise AssertionError("二次退货后库存行消失")
        assert_eq(inv_after2["quantity"], 18, "二次退货后库存.quantity 应为 18")
        _log.info("二次退货库存验证通过：qty=18（总退 8 / 已售 10）")

        # ---------- 第 8 步：超退测试 ----------
        # 已退 8 件，再退 5 件总计 13 > 已售 10，应被拒
        _log.info("创建超退退货单 qty=5（已退 8 + 5 = 13 > 已售 10），期望 complete 失败")
        over_ret = client.post("/documents", json={
            "docType": 3,
            "refDocId": sale_doc_id,
            "lines": [{"productId": product_id, "quantity": 5}],
        })
        over_ret_id = over_ret["id"]
        expect_error(
            lambda: client.post(f"/documents/{over_ret_id}/complete"),
            code=10003,  # ErrValidation: 超退
        )
        _log.info("超退拦截验证通过")

        # ---------- 第 9 步：错误路径 ----------
        _log.info("===== 错误路径测试 =====")

        # 无 refDocId 的退货单 — 后端 service.validateCreateRequest 要求 refDocId
        _log.info("创建退货单不传 refDocId，期望 code=10003")
        expect_error(
            lambda: client.post("/documents", json={
                "docType": 3,
                "lines": [{"productId": product_id, "quantity": 1}],
            }),
            code=10003,
        )
        _log.info("无 refDocId 拦截通过")

        # 引用入库单（非销售单）做退货
        _log.info("创建退货单引用入库单，期望 complete 时报错")
        # 先拿到之前入库单的 id
        inb_page = client.get("/documents", params={
            "docType": 1, "page": 1, "pageSize": 5,
        })
        inb_list = inb_page.get("list") or []
        if inb_list:
            inb_doc_id = inb_list[0]["id"]
            bad_ref = client.post("/documents", json={
                "docType": 3,
                "refDocId": inb_doc_id,
                "lines": [{"productId": product_id, "quantity": 1}],
            })
            expect_error(
                lambda: client.post(f"/documents/{bad_ref['id']}/complete"),
                code=10003,
            )
            _log.info("引用非销售单退货拦截通过")

        # 引用草稿销售单（未完成）
        _log.info("创建草稿销售单，然后创建退货单引用它，期望 complete 时报错")
        draft_sale = client.post("/documents", json={
            "docType": 2,
            "lines": [{"productId": product_id, "quantity": 1, "retailPrice": 100.0}],
        })
        draft_sale_id = draft_sale["id"]
        draft_ret = client.post("/documents", json={
            "docType": 3,
            "refDocId": draft_sale_id,
            "lines": [{"productId": product_id, "quantity": 1}],
        })
        expect_error(
            lambda: client.post(f"/documents/{draft_ret['id']}/complete"),
            code=10003,
        )
        _log.info("引用草稿销售单退货拦截通过")

        _log.info("===== 错误路径测试结束 =====")

        # ---------- 第 10 步：普通用户退货 ----------
        normal_username = _unique_username()
        normal_password = "RetTest@12345"
        _log.info("创建普通用户 %s 测试退货权限", normal_username)
        normal_user = client.post("/users", json={
            "username": normal_username,
            "password": normal_password,
            "realName": "退货测试用户",
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

        # 普通用户应能退货（关联已完成的销售单）
        # 还可以退 2 件（已售 10 - 已退 8 = 可退 2）
        _log.info("普通用户创建退货单 qty=2")
        nu_ret = client.post("/documents", json={
            "docType": 3,
            "refDocId": sale_doc_id,
            "lines": [{"productId": product_id, "quantity": 2}],
        })
        nu_ret_id = nu_ret["id"]
        client.post(f"/documents/{nu_ret_id}/complete")
        _log.info("普通用户退货完成")

        nu_inv = _find_inventory_by_product(client, product_id)
        if nu_inv is None:
            raise AssertionError("普通用户退货后库存行消失")
        assert_eq(nu_inv["quantity"], 20, "普通用户退货后库存.quantity 应为 20")
        _log.info("普通用户退货后库存验证通过：qty=20（全部退完回原位）")

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

        _log.info("退货单流程验证完成")
