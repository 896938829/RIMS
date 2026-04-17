"""盘点单三阶段 recording→confirmed→settled 流程（PRD 第 10 章 — 盘点，admin-only）。

预期步骤：
     1) 准备：admin 身份、WH001、创建商品 A、入库 20 件建立库存
     2) POST /documents 创建盘点单（docType=5），actualQty=18 → systemQty=20, diffQty=-2
     3) GET /documents/:id 验证 status=1(盘点中)、docNo 前缀 PD、line 差异字段
     4) POST /documents/:id/complete → 期望被拒（盘点单不走 complete）
     5) POST /documents/:id/confirm → status=2(差异已确认)，库存不变
     6) GET /inventory 确认库存仍为 20
     7) POST /documents/:id/settle → status=3(已结转)
     8) GET /inventory 验证库存变为 18（减少 2）
     9) GET /transactions 验证流水（direction=-1, qty=2）
    10) 盘盈测试：新建盘点 actualQty=25 → diffQty=+7 → settle → 库存 25
    11) 错误路径：settle 前未 confirm(20002)、confirm 非盘点单(error)、
        重复 settle(20002)
    12) 权限探测：普通用户 confirm/settle → 403
    13) 清理

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

_log = get_logger("flow.document.stocktake")


# --------------- 工具函数 ---------------


def _unique_code() -> str:
    """生成唯一编码。"""
    return f"stk_{int(time.time() * 1000)}"


def _unique_name() -> str:
    """生成唯一名称。"""
    return f"盘点测试商品_{int(time.time() * 1000)}"


def _unique_username() -> str:
    """生成唯一测试用户名。"""
    return f"stk_test_user_{int(time.time() * 1000)}"


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
    """创建并完成入库单，为盘点准备库存。"""
    _log.info("前置入库：productId=%s qty=%s", product_id, qty)
    doc = client.post("/documents", json={
        "docType": 1,
        "remark": "盘点测试前置入库",
        "lines": [{"productId": product_id, "quantity": qty}],
    })
    client.post(f"/documents/{doc['id']}/complete")
    _log.info("入库完成：docId=%s", doc["id"])
    return doc


# --------------- 主流程 ---------------


def run(client: APIClient) -> None:  # noqa: C901
    """执行盘点单三阶段（recording→confirmed→settled）全流程验证。

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
        "retailPrice": 60.0,
        "costPrice": 30.0,
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

        # ---------- 第 2 步：创建盘点单（盘亏场景） ----------
        _log.info("POST /documents 创建盘点单：docType=5 actualQty=18（系统 20，差异 -2）")
        doc = client.post("/documents", json={
            "docType": 5,
            "remark": "盘点测试-盘亏",
            "lines": [{
                "productId": product_id,
                "actualQty": 18,
            }],
        })
        doc_id = doc["id"]
        doc_no = doc["docNo"]
        _log.info("盘点单创建成功：id=%s docNo=%s", doc_id, doc_no)

        # ---------- 第 3 步：验证盘点中状态和差异字段 ----------
        assert_eq(doc["status"], 1, "盘点单.status 应为 1(盘点中/recording)")
        assert_eq(doc["docType"], 5, "盘点单.docType 应为 5")
        if not doc_no.startswith("PD"):
            raise AssertionError(f"盘点单 docNo 应以 PD 开头，实际 {doc_no!r}")

        _log.info("GET /documents/%s 详情回查", doc_id)
        detail = client.get(f"/documents/{doc_id}")
        assert_in("lines", detail, "详情应含 lines")
        lines = detail["lines"]
        assert_eq(len(lines), 1, "lines 长度")
        line = lines[0]
        # 后端在 buildLines 时自动填充 systemQty、计算 diffQty
        assert_eq(line["systemQty"], 20, "line.systemQty 应为 20（入库后系统量）")
        assert_eq(line["actualQty"], 18, "line.actualQty 应为 18（盘点录入量）")
        assert_eq(line["diffQty"], -2, "line.diffQty 应为 -2（18-20）")
        _log.info("盘点单差异字段验证通过：systemQty=20, actualQty=18, diffQty=-2")

        # ---------- 第 4 步：盘点单不能走 complete ----------
        _log.info("POST /documents/%s/complete 期望被拒（盘点单不走 complete）", doc_id)
        expect_error(
            lambda: client.post(f"/documents/{doc_id}/complete"),
            code=20002,  # ErrInvalidState: 盘点单请使用确认和结转操作
        )
        _log.info("盘点单 complete 拦截验证通过")

        # ---------- 第 5 步：confirm（差异确认） ----------
        _log.info("POST /documents/%s/confirm 确认盘点差异", doc_id)
        client.post(f"/documents/{doc_id}/confirm")
        _log.info("盘点确认完成")

        confirmed = client.get(f"/documents/{doc_id}")
        assert_eq(confirmed["status"], 2, "confirm 后.status 应为 2(差异已确认)")
        _log.info("盘点确认状态验证通过")

        # ---------- 第 6 步：confirm 后库存不变 ----------
        _log.info("验证 confirm 后库存不变（仍为 20）")
        inv_after_confirm = _find_inventory_by_product(client, product_id)
        if inv_after_confirm is None:
            raise AssertionError("confirm 后库存行消失")
        assert_eq(inv_after_confirm["quantity"], 20, "confirm 后库存.quantity 应不变")
        _log.info("confirm 后库存不变验证通过：qty=20")

        # ---------- 第 7 步：settle（结转） ----------
        _log.info("POST /documents/%s/settle 结转盘点", doc_id)
        client.post(f"/documents/{doc_id}/settle")
        _log.info("盘点结转完成")

        settled = client.get(f"/documents/{doc_id}")
        assert_eq(settled["status"], 3, "settle 后.status 应为 3(已结转)")
        assert_in("operatedAt", settled, "settle 后应有 operatedAt")
        _log.info("盘点结转状态验证通过")

        # ---------- 第 8 步：结转后库存校验 ----------
        _log.info("验证结转后库存：预期 20→18（diffQty=-2）")
        inv_after_settle = _find_inventory_by_product(client, product_id)
        if inv_after_settle is None:
            raise AssertionError("settle 后库存行消失")
        assert_eq(inv_after_settle["quantity"], 18, "settle 后库存.quantity 应为 18")
        _log.info("盘点结转后库存验证通过：qty=18")

        # ---------- 第 9 步：验证流水 ----------
        _log.info("GET /transactions 检查盘点结转流水")
        txn_page = client.get("/transactions", params={
            "page": 1, "pageSize": 50, "keyword": doc_no,
        })
        assert_page(txn_page, min_total=1)
        doc_txns = [t for t in (txn_page.get("list") or []) if t.get("docId") == doc_id]
        if not doc_txns:
            raise AssertionError(f"未找到 docId={doc_id} 的盘点流水")
        txn = doc_txns[0]
        assert_eq(txn["direction"], -1, "盘亏流水.direction 应为 -1(出)")
        assert_eq(txn["quantity"], 2, "盘亏流水.quantity 应为 2（绝对值）")
        _log.info("盘点结转流水验证通过")

        # ---------- 第 10 步：盘盈测试 ----------
        _log.info("===== 盘盈测试 =====")
        _log.info("创建盘盈盘点单：actualQty=25（系统 18，差异 +7）")
        doc2 = client.post("/documents", json={
            "docType": 5,
            "remark": "盘点测试-盘盈",
            "lines": [{
                "productId": product_id,
                "actualQty": 25,
            }],
        })
        doc2_id = doc2["id"]
        doc2_no = doc2["docNo"]
        _log.info("盘盈单创建成功：id=%s docNo=%s", doc2_id, doc2_no)

        # 验证差异字段
        detail2 = client.get(f"/documents/{doc2_id}")
        line2 = detail2["lines"][0]
        assert_eq(line2["systemQty"], 18, "盘盈 line.systemQty 应为 18")
        assert_eq(line2["actualQty"], 25, "盘盈 line.actualQty 应为 25")
        assert_eq(line2["diffQty"], 7, "盘盈 line.diffQty 应为 +7")
        _log.info("盘盈差异字段验证通过")

        # confirm → settle
        client.post(f"/documents/{doc2_id}/confirm")
        _log.info("盘盈单已确认")
        client.post(f"/documents/{doc2_id}/settle")
        _log.info("盘盈单已结转")

        inv_surplus = _find_inventory_by_product(client, product_id)
        if inv_surplus is None:
            raise AssertionError("盘盈结转后库存行消失")
        assert_eq(inv_surplus["quantity"], 25, "盘盈结转后库存.quantity 应为 25")
        _log.info("盘盈结转后库存验证通过：qty=25")

        # 验证盘盈流水
        txn2_page = client.get("/transactions", params={
            "page": 1, "pageSize": 50, "keyword": doc2_no,
        })
        doc2_txns = [t for t in (txn2_page.get("list") or []) if t.get("docId") == doc2_id]
        if not doc2_txns:
            raise AssertionError(f"未找到 docId={doc2_id} 的盘盈流水")
        assert_eq(doc2_txns[0]["direction"], 1, "盘盈流水.direction 应为 1(入)")
        assert_eq(doc2_txns[0]["quantity"], 7, "盘盈流水.quantity 应为 7")
        _log.info("盘盈流水验证通过")

        # ---------- 第 11 步：错误路径 ----------
        _log.info("===== 错误路径测试 =====")

        # settle 前未 confirm（直接从 recording 跳到 settle）
        _log.info("创建新盘点单，跳过 confirm 直接 settle，期望 code=20002")
        err_doc = client.post("/documents", json={
            "docType": 5,
            "lines": [{"productId": product_id, "actualQty": 20}],
        })
        err_doc_id = err_doc["id"]
        expect_error(
            lambda: client.post(f"/documents/{err_doc_id}/settle"),
            code=20002,
        )
        _log.info("跳过 confirm 直接 settle 拦截通过")

        # confirm 非盘点单（用一个入库单试试）
        _log.info("对入库单调 confirm，期望报错")
        inb_doc = client.post("/documents", json={
            "docType": 1,
            "lines": [{"productId": product_id, "quantity": 1}],
        })
        expect_error(
            lambda: client.post(f"/documents/{inb_doc['id']}/confirm"),
            code=10003,  # ErrValidation: 非盘点单不支持此操作
        )
        _log.info("非盘点单 confirm 拦截通过")

        # settle 非盘点单
        _log.info("对入库单调 settle，期望报错")
        expect_error(
            lambda: client.post(f"/documents/{inb_doc['id']}/settle"),
            code=10003,
        )
        _log.info("非盘点单 settle 拦截通过")

        # 重复 settle 已结转的盘点单
        _log.info("重复 settle 已结转盘点单 id=%s，期望 code=20002", doc_id)
        expect_error(
            lambda: client.post(f"/documents/{doc_id}/settle"),
            code=20002,
        )
        _log.info("重复 settle 拦截通过")

        # 重复 confirm 已确认的盘点单（状态不对）
        # err_doc 目前 status=1，先 confirm 它到 status=2，再重复 confirm
        client.post(f"/documents/{err_doc_id}/confirm")
        _log.info("重复 confirm 已确认盘点单 id=%s，期望 code=20002", err_doc_id)
        expect_error(
            lambda: client.post(f"/documents/{err_doc_id}/confirm"),
            code=20002,
        )
        _log.info("重复 confirm 拦截通过")

        _log.info("===== 错误路径测试结束 =====")

        # ---------- 第 12 步：权限探测 ----------
        normal_username = _unique_username()
        normal_password = "StkTest@12345"
        _log.info("创建普通用户 %s 做盘点权限探测", normal_username)
        normal_user = client.post("/users", json={
            "username": normal_username,
            "password": normal_password,
            "realName": "盘点测试用户",
            "roleId": user_role_id,
        }, scoped=False)
        normal_user_id = normal_user["id"]

        _log.info("绑定普通用户到 WH001")
        client.post(f"/warehouses/{seed_wh_id}/users",
                    json={"userIds": [normal_user_id]}, scoped=False)

        # admin 创建一个草稿盘点单，confirm 到 status=2，供普通用户测 settle
        _log.info("admin 创建并 confirm 一个盘点单供权限探测")
        probe_doc = client.post("/documents", json={
            "docType": 5,
            "lines": [{"productId": product_id, "actualQty": 25}],
        })
        probe_doc_id = probe_doc["id"]
        # 另建一个 recording 状态的盘点单供普通用户测 confirm
        probe_doc_rec = client.post("/documents", json={
            "docType": 5,
            "lines": [{"productId": product_id, "actualQty": 25}],
        })
        probe_doc_rec_id = probe_doc_rec["id"]

        # confirm probe_doc 到 status=2
        client.post(f"/documents/{probe_doc_id}/confirm")

        _log.info("切换登录为普通用户")
        auth_login_flow.run(client, normal_username, normal_password)
        assert_eq(SESSION.role_code, "user", "切换后角色应为 user")
        client.set_warehouse(seed_wh_id)

        _log.info("===== 非 admin 盘点权限探测 =====")

        # 普通用户 confirm → 403
        if not _probe_permission(
            "非admin POST /documents/:id/confirm (盘点)",
            lambda: client.post(f"/documents/{probe_doc_rec_id}/confirm"),
        ):
            vulnerabilities.append("非admin可确认盘点")

        # 普通用户 settle → 403
        if not _probe_permission(
            "非admin POST /documents/:id/settle (盘点)",
            lambda: client.post(f"/documents/{probe_doc_id}/settle"),
        ):
            vulnerabilities.append("非admin可结转盘点")

        # 普通用户创建盘点单（PRD 说 admin-only，但后端 create 可能没 gate）
        if not _probe_permission(
            "非admin POST /documents (docType=5 盘点)",
            lambda: client.post("/documents", json={
                "docType": 5,
                "lines": [{"productId": product_id, "actualQty": 25}],
            }),
        ):
            vulnerabilities.append("非admin可创建盘点单")

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

        _log.info("盘点单三阶段流程验证完成")
