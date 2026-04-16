"""条码查询流程（PRD 第 5 章 - 条码扫描接入）。

预期步骤：
    1) admin 创建带条码的商品 POST /products
    2) admin 反向查询 GET /products/barcode/:barcode，断言返回同一记录 + costPrice 可见
    3) admin 正向一致性：GET /products/:id 的 barcode 与创建时一致
    4) 用不同 code 相同 barcode 再创建，期望 code=10005（条码唯一）
    5) 查询不存在的条码，期望 HTTP 404
    6) 切换普通用户登录：反向查询应返回商品但屏蔽 costPrice
    7) 切回 admin，清理商品 + 测试用户

测试原则：模拟用户操作，寻找后端漏洞。
"""

from __future__ import annotations

import time
from typing import Any, List

from core.assertions import assert_eq, assert_in, expect_error
from core.client import APIClient
from core.config import CONFIG
from core.logger import get_logger
from core.session import SESSION
from flows.auth import login as auth_login_flow

_log = get_logger("flow.product.barcode")


# --------------- 工具函数 ---------------


def _unique_code() -> str:
    """生成唯一商品编码。"""
    return f"barc_{int(time.time() * 1000)}"


def _unique_barcode() -> str:
    """生成唯一条码（13 位数字，GTIN 前缀 690 常用于中国商品）。"""
    return f"690{int(time.time() * 1000)}"


def _unique_name() -> str:
    """生成唯一商品名。"""
    return f"条码测试_{int(time.time() * 1000)}"


def _unique_username() -> str:
    """生成唯一测试用户名。"""
    return f"barc_test_user_{int(time.time() * 1000)}"


def _get_role_id_by_code(client: APIClient, code: str) -> int:
    """通过角色编码获取角色 ID。"""
    _log.info("查询角色列表，寻找 code=%s", code)
    roles = client.get("/roles", scoped=False)
    for r in roles:
        if r.get("code") == code:
            _log.info("找到角色：id=%s code=%s", r["id"], r["code"])
            return r["id"]
    raise AssertionError(f"未找到 code={code!r} 的角色")


def _assert_cost_price(payload: Any, expected_visible: bool, label: str) -> None:
    """断言 costPrice 字段的可见性（与 product_crud 的同名函数逻辑一致）。

    非 admin 看到 costPrice → 敏感字段泄露漏洞，直接 AssertionError。
    admin 看不到 costPrice → 后端序列化 bug，同样 AssertionError。
    """
    if isinstance(payload, dict):
        has_key = "costPrice" in payload
    else:
        raise AssertionError(f"{label}: 期望 dict，实际 {type(payload).__name__}")

    if expected_visible and not has_key:
        raise AssertionError(
            f"{label}: 期望 costPrice 可见，但字段缺失：{list(payload.keys())}"
        )
    if not expected_visible and has_key:
        raise AssertionError(
            f"{label}: ⚠ 敏感字段泄露！非 admin 不应看到 costPrice={payload['costPrice']!r}"
        )
    _log.info(
        "%s: costPrice 可见性校验通过（expected_visible=%s）",
        label, expected_visible,
    )


# --------------- 主流程 ---------------


def run(client: APIClient) -> None:  # noqa: C901
    """执行条码查询流程验证。

    前置条件：调用方已以 admin 身份登录。
    """
    # --- 准备：user 角色 id + 测试用户 ---
    user_role_id = _get_role_id_by_code(client, "user")

    normal_username = _unique_username()
    normal_password = "BarcTest@12345"
    _log.info("准备创建测试用户：username=%s", normal_username)
    normal_user = client.post("/users", json={
        "username": normal_username,
        "password": normal_password,
        "realName": "条码测试用户",
        "roleId": user_role_id,
    }, scoped=False)
    normal_user_id = normal_user["id"]
    _log.info("测试用户创建成功：id=%s", normal_user_id)

    created_product_ids: List[int] = []  # 便于 finally 清理

    try:
        # ========== 第 1 步：admin 创建带条码的商品 ==========
        code_a = _unique_code()
        name_a = _unique_name()
        barcode_a = _unique_barcode()
        _log.info("开始创建带条码的商品：code=%s barcode=%s", code_a, barcode_a)
        created = client.post("/products", json={
            "code": code_a,
            "name": name_a,
            "unit": "包",
            "barcode": barcode_a,
            "retailPrice": 29.9,
            "costPrice": 12.5,
        }, scoped=False)
        assert_in("id", created, "创建商品响应")
        assert_eq(created["barcode"], barcode_a, "商品.barcode")
        product_id = created["id"]
        created_product_ids.append(product_id)
        _log.info("带条码商品创建成功：id=%s barcode=%s", product_id, barcode_a)

        # ========== 第 2 步：admin 反向查询 ==========
        _log.info("admin 反向查询：GET /products/barcode/%s", barcode_a)
        by_barcode = client.get(f"/products/barcode/{barcode_a}", scoped=False)
        assert_eq(by_barcode["id"], product_id, "反向查询.id")
        assert_eq(by_barcode["barcode"], barcode_a, "反向查询.barcode")
        assert_eq(by_barcode["code"], code_a, "反向查询.code")
        assert_eq(by_barcode["name"], name_a, "反向查询.name")
        _assert_cost_price(by_barcode, expected_visible=True, label="admin 条码反查")
        assert_eq(by_barcode["costPrice"], 12.5, "反向查询.costPrice")
        _log.info("admin 条码反查验证通过")

        # ========== 第 3 步：admin 正向一致性 ==========
        _log.info("admin 正向查询详情：id=%s，确认 barcode 字段", product_id)
        detail = client.get(f"/products/{product_id}", scoped=False)
        assert_eq(detail["barcode"], barcode_a, "详情.barcode 与反查一致")
        _log.info("正反向一致性验证通过")

        # ========== 第 4 步：重复条码拦截 ==========
        _log.info("用不同 code 但相同 barcode 再创建，期望 code=10005")
        expect_error(
            lambda: client.post("/products", json={
                "code": _unique_code(),
                "name": "重复条码测试",
                "unit": "包",
                "barcode": barcode_a,
            }, scoped=False),
            code=10005,
        )
        _log.info("重复条码拦截验证通过")

        # ========== 第 5 步：未命中条码 ==========
        miss_barcode = f"999{int(time.time() * 1000)}"
        _log.info("查询不存在的条码 barcode=%s，期望 HTTP 404", miss_barcode)
        expect_error(
            lambda: client.get(f"/products/barcode/{miss_barcode}", scoped=False),
            http_status=404,
        )
        _log.info("未命中条码 404 验证通过")

        # ========== 第 6 步：普通用户反查应屏蔽 costPrice ==========
        _log.info("切换登录为普通用户 username=%s", normal_username)
        auth_login_flow.run(client, normal_username, normal_password)
        assert_eq(SESSION.role_code, "user", "切换后角色应为 user")

        _log.info("普通用户反向查询：GET /products/barcode/%s", barcode_a)
        normal_by_barcode = client.get(f"/products/barcode/{barcode_a}", scoped=False)
        # 业务字段仍应返回
        assert_eq(normal_by_barcode["id"], product_id, "普通用户反查.id")
        assert_eq(normal_by_barcode["barcode"], barcode_a, "普通用户反查.barcode")
        # costPrice 必须屏蔽
        _assert_cost_price(normal_by_barcode, expected_visible=False,
                           label="普通用户条码反查")
        _log.info("普通用户屏蔽 costPrice 验证通过")

        # 同样验证普通用户未命中条码时的 404（回归安全边界）
        _log.info("普通用户查询不存在的条码，期望 HTTP 404")
        expect_error(
            lambda: client.get(f"/products/barcode/{miss_barcode}", scoped=False),
            http_status=404,
        )
        _log.info("普通用户未命中 404 验证通过")

    finally:
        # ========== 第 7 步：清理（无论中途失败与否都要清） ==========
        _log.info("切回 admin 身份执行清理")
        auth_login_flow.run(client, CONFIG.admin_user, CONFIG.admin_password)

        for pid in created_product_ids:
            try:
                _log.info("清理商品：id=%s", pid)
                client.delete(f"/products/{pid}", scoped=False)
            except Exception as e:  # noqa: BLE001
                _log.warning("清理商品 id=%s 失败：%s", pid, e)

        try:
            _log.info("清理测试用户：id=%s", normal_user_id)
            client.delete(f"/users/{normal_user_id}", scoped=False)
        except Exception as e:  # noqa: BLE001
            _log.warning("清理测试用户失败：%s", e)

    _log.info("条码查询流程验证完成")
