"""跨仓调拨 + 双仓库存校验 流程（PRD 第 9 章 — 调拨，admin-only）。

预期步骤：
     1) 准备：admin 身份、WH001（源仓）、创建 wh_b（目标仓）、创建商品 A、入库 20 件到 WH001
     2) POST /documents 创建调拨单（docType=4），toWarehouseId=wh_b，line qty=8
     3) GET /documents/:id 验证 status=1、docNo 前缀 DB、toWarehouseId
     4) POST /documents/:id/complete 完成调拨
     5) 切 WH001 → GET /inventory 验证减少（20→12）
     6) 切 wh_b → GET /inventory 验证增加（0→8）
     7) GET /transactions 验证双仓各一条流水
     8) 错误路径：调拨到同一仓库(error)、超库存(error)、缺 toWarehouseId(error)
     9) 权限探测：普通用户 complete 调拨 → 期望 403
    10) 清理：删 wh_b、普通用户；商品有库存时仅记录

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

_log = get_logger("flow.document.transfer")


# --------------- 工具函数 ---------------


def _unique_code() -> str:
    """生成唯一编码。"""
    return f"xfer_{int(time.time() * 1000)}"


def _unique_name() -> str:
    """生成唯一名称。"""
    return f"调拨测试_{int(time.time() * 1000)}"


def _unique_username() -> str:
    """生成唯一测试用户名。"""
    return f"xfer_test_user_{int(time.time() * 1000)}"


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


def _create_inbound_and_complete(
    client: APIClient, product_id: int, qty: int,
) -> dict:
    """创建并完成入库单，为调拨准备库存。"""
    _log.info("前置入库：productId=%s qty=%s", product_id, qty)
    doc = client.post("/documents", json={
        "docType": 1,
        "remark": "调拨测试前置入库",
        "lines": [{"productId": product_id, "quantity": qty}],
    })
    client.post(f"/documents/{doc['id']}/complete")
    _log.info("入库完成：docId=%s", doc["id"])
    return doc


# --------------- 主流程 ---------------


def run(client: APIClient) -> None:  # noqa: C901
    """执行跨仓调拨 + 双仓库存校验 全流程验证。

    前置条件：调用方已以 admin 身份登录。
    """
    vulnerabilities: List[str] = []

    # ---------- 第 1 步：环境准备 ----------
    admin_id = SESSION.user.get("id")
    if admin_id is None:
        raise AssertionError("SESSION.user.id 为空，疑似未登录")
    _log.info("当前 admin：id=%s username=%s", admin_id, SESSION.user.get("username"))

    seed_wh_id = _find_seed_warehouse_id(client, "WH001")
    _log.info("seed 仓库 WH001.id=%s（源仓）", seed_wh_id)
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
        "retailPrice": 80.0,
        "costPrice": 40.0,
    }, scoped=False)
    product_id = product["id"]
    _log.info("商品创建成功：id=%s", product_id)

    # 创建目标仓库 wh_b
    wh_b_code = _unique_code() + "_b"
    _log.info("创建目标仓库 B：code=%s", wh_b_code)
    wh_b = client.post("/warehouses", json={
        "code": wh_b_code,
        "name": _unique_name() + "_B",
    }, scoped=False)
    wh_b_id = wh_b["id"]
    _log.info("仓库 B 创建成功：id=%s", wh_b_id)

    # 绑 admin 到 wh_b（admin 可多仓）
    _log.info("绑定 admin 到 wh_b")
    client.post(f"/warehouses/{wh_b_id}/users",
                json={"userIds": [admin_id]}, scoped=False)

    normal_user_id: Optional[int] = None

    try:
        # 入库 20 件到 WH001（源仓）
        _create_inbound_and_complete(client, product_id, 20)
        inv_src = _find_inventory_by_product(client, product_id)
        if inv_src is None:
            raise AssertionError("入库后源仓未找到库存行")
        assert_eq(inv_src["quantity"], 20, "源仓入库后库存.quantity")
        _log.info("源仓库存就绪：qty=20")

        # ---------- 第 2 步：创建调拨单 ----------
        _log.info("POST /documents 创建调拨单：docType=4 toWarehouseId=%s qty=8", wh_b_id)
        doc = client.post("/documents", json={
            "docType": 4,
            "toWarehouseId": wh_b_id,
            "remark": "调拨测试",
            "lines": [{
                "productId": product_id,
                "quantity": 8,
            }],
        })
        doc_id = doc["id"]
        doc_no = doc["docNo"]
        _log.info("调拨单创建成功：id=%s docNo=%s", doc_id, doc_no)

        # ---------- 第 3 步：验证草稿状态 ----------
        assert_eq(doc["status"], 1, "调拨单.status 应为 1(草稿)")
        assert_eq(doc["docType"], 4, "调拨单.docType 应为 4(调拨)")
        if not doc_no.startswith("DB"):
            raise AssertionError(f"调拨单 docNo 应以 DB 开头，实际 {doc_no!r}")

        _log.info("GET /documents/%s 详情回查", doc_id)
        detail = client.get(f"/documents/{doc_id}")
        assert_eq(detail["toWarehouseId"], wh_b_id, "详情.toWarehouseId")
        assert_in("lines", detail, "详情应含 lines")
        assert_eq(len(detail["lines"]), 1, "lines 长度")
        assert_eq(detail["lines"][0]["quantity"], 8, "line.quantity")
        _log.info("调拨单详情验证通过")

        # ---------- 第 4 步：完成调拨 ----------
        _log.info("POST /documents/%s/complete 完成调拨", doc_id)
        client.post(f"/documents/{doc_id}/complete")
        _log.info("调拨完成")

        completed = client.get(f"/documents/{doc_id}")
        assert_eq(completed["status"], 2, "完成后.status 应为 2")

        # ---------- 第 5 步：验证源仓库存减少 ----------
        _log.info("切到源仓 WH001 验证库存减少：预期 20→12")
        client.set_warehouse(seed_wh_id)
        inv_src_after = _find_inventory_by_product(client, product_id)
        if inv_src_after is None:
            raise AssertionError("调拨后源仓库存行消失")
        assert_eq(inv_src_after["quantity"], 12, "源仓调拨后库存.quantity")
        _log.info("源仓库存验证通过：qty=12")

        # ---------- 第 6 步：验证目标仓库存增加 ----------
        _log.info("切到目标仓 wh_b 验证库存增加：预期 0→8")
        client.set_warehouse(wh_b_id)
        inv_dst = _find_inventory_by_product(client, product_id)
        if inv_dst is None:
            raise AssertionError("调拨后目标仓未找到库存行")
        assert_eq(inv_dst["quantity"], 8, "目标仓调拨后库存.quantity")
        _log.info("目标仓库存验证通过：qty=8")

        # ---------- 第 7 步：验证流水 ----------
        _log.info("检查调拨流水（源仓+目标仓各一条）")
        # 源仓流水
        client.set_warehouse(seed_wh_id)
        src_txn_page = client.get("/transactions", params={
            "page": 1, "pageSize": 50, "keyword": doc_no,
        })
        assert_page(src_txn_page, min_total=1)
        src_txns = [t for t in (src_txn_page.get("list") or []) if t.get("docId") == doc_id]
        if not src_txns:
            raise AssertionError("源仓未找到调拨流水")
        assert_eq(src_txns[0]["direction"], -1, "源仓流水.direction 应为 -1(出)")
        assert_eq(src_txns[0]["quantity"], 8, "源仓流水.quantity")
        _log.info("源仓流水验证通过")

        # 目标仓流水
        client.set_warehouse(wh_b_id)
        dst_txn_page = client.get("/transactions", params={
            "page": 1, "pageSize": 50, "keyword": doc_no,
        })
        assert_page(dst_txn_page, min_total=1)
        dst_txns = [t for t in (dst_txn_page.get("list") or []) if t.get("docId") == doc_id]
        if not dst_txns:
            raise AssertionError("目标仓未找到调拨流水")
        assert_eq(dst_txns[0]["direction"], 1, "目标仓流水.direction 应为 1(入)")
        assert_eq(dst_txns[0]["quantity"], 8, "目标仓流水.quantity")
        _log.info("目标仓流水验证通过")

        # 切回源仓继续测试
        client.set_warehouse(seed_wh_id)

        # ---------- 第 8 步：错误路径 ----------
        _log.info("===== 错误路径测试 =====")

        # 调拨到同一仓库
        _log.info("调拨到同一仓库（源=目标），期望报错")
        same_wh_doc = client.post("/documents", json={
            "docType": 4,
            "toWarehouseId": seed_wh_id,  # 同一仓库
            "lines": [{"productId": product_id, "quantity": 1}],
        })
        expect_error(
            lambda: client.post(f"/documents/{same_wh_doc['id']}/complete"),
            code=10003,
        )
        _log.info("同仓调拨拦截通过")

        # 超库存调拨
        _log.info("调拨数量超出库存（源仓剩 12），期望 ErrInsufficientStock")
        over_doc = client.post("/documents", json={
            "docType": 4,
            "toWarehouseId": wh_b_id,
            "lines": [{"productId": product_id, "quantity": 100}],
        })
        expect_error(
            lambda: client.post(f"/documents/{over_doc['id']}/complete"),
            code=20001,  # ErrInsufficientStock
        )
        _log.info("超库存调拨拦截通过")

        # 缺 toWarehouseId
        _log.info("创建调拨单不传 toWarehouseId，期望 code=10003")
        expect_error(
            lambda: client.post("/documents", json={
                "docType": 4,
                "lines": [{"productId": product_id, "quantity": 1}],
            }),
            code=10003,
        )
        _log.info("缺 toWarehouseId 拦截通过")

        _log.info("===== 错误路径测试结束 =====")

        # ---------- 第 9 步：权限探测 ----------
        normal_username = _unique_username()
        normal_password = "XferTest@12345"
        _log.info("创建普通用户 %s 做调拨权限探测", normal_username)
        normal_user = client.post("/users", json={
            "username": normal_username,
            "password": normal_password,
            "realName": "调拨测试用户",
            "roleId": user_role_id,
        }, scoped=False)
        normal_user_id = normal_user["id"]

        _log.info("绑定普通用户到 WH001")
        client.post(f"/warehouses/{seed_wh_id}/users",
                    json={"userIds": [normal_user_id]}, scoped=False)

        # admin 创建一个草稿调拨单供普通用户测试 complete
        _log.info("admin 创建草稿调拨单供权限探测")
        probe_doc = client.post("/documents", json={
            "docType": 4,
            "toWarehouseId": wh_b_id,
            "lines": [{"productId": product_id, "quantity": 1}],
        })
        probe_doc_id = probe_doc["id"]

        _log.info("切换登录为普通用户")
        auth_login_flow.run(client, normal_username, normal_password)
        assert_eq(SESSION.role_code, "user", "切换后角色应为 user")
        client.set_warehouse(seed_wh_id)

        _log.info("===== 非 admin 调拨权限探测 =====")

        # 普通用户 complete 调拨 — handler 有 admin gate
        if not _probe_permission(
            "非admin POST /documents/:id/complete (调拨)",
            lambda: client.post(f"/documents/{probe_doc_id}/complete"),
        ):
            vulnerabilities.append("非admin可完成调拨单")

        # 普通用户创建调拨单
        if not _probe_permission(
            "非admin POST /documents (docType=4 调拨)",
            lambda: client.post("/documents", json={
                "docType": 4,
                "toWarehouseId": wh_b_id,
                "lines": [{"productId": product_id, "quantity": 1}],
            }),
        ):
            vulnerabilities.append("非admin可创建调拨单")

        _log.info("===== 权限探测结束 =====")

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

        if wh_b_id is not None:
            _log.info("删除目标仓库 B：id=%s", wh_b_id)
            try:
                client.delete(f"/warehouses/{wh_b_id}", scoped=False)
            except APIError as e:
                _log.warning("删除 wh_b 失败（忽略）：%s", e)

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

        _log.info("跨仓调拨流程验证完成")
