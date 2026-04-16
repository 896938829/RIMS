"""商品 CRUD 流程（PRD 第 5 章 - 商品档案管理）。

预期步骤：
     1) 管理员创建商品 POST /products
     2) 列表 GET /products 断言新商品出现 + costPrice 可见
     3) 详情 GET /products/:id 回查字段 + costPrice 可见
     4) 修改 PUT /products/:id name/retailPrice，断言未传字段保持不变
     5) 创建普通用户用于权限探测（商品接口不 scoped，无需绑仓库）
     6) 切换为普通用户登录：
        - 列表 / 详情应屏蔽 costPrice（核心硬断言）
        - POST / PUT / DELETE 应被 403 拦截（收集漏洞）
     7) 切回 admin
     8) 错误路径：重复 code / 缺必填 / 未知 id 的 GET/PUT/DELETE
     9) 删除商品 + GET 返回 404 + 再次删除 404
    10) 清理测试用户 + 漏洞汇总

测试原则：模拟用户操作，寻找后端漏洞。
"""

from __future__ import annotations

import time
from typing import Any, Callable, List

from core.assertions import assert_eq, assert_in, assert_page, expect_error
from core.client import APIClient
from core.config import CONFIG
from core.logger import get_logger
from core.session import SESSION
from flows.auth import login as auth_login_flow

_log = get_logger("flow.product.crud")


# --------------- 工具函数 ---------------


def _unique_code() -> str:
    """生成唯一商品编码，避免多次运行冲突。"""
    return f"prod_{int(time.time() * 1000)}"


def _unique_barcode() -> str:
    """生成唯一条码（13 位数字，GTIN 前缀 690 常用于中国商品）。"""
    return f"690{int(time.time() * 1000)}"


def _unique_name() -> str:
    """生成唯一商品名。"""
    return f"测试商品_{int(time.time() * 1000)}"


def _unique_username() -> str:
    """生成唯一测试用户名。"""
    return f"prod_test_user_{int(time.time() * 1000)}"


def _get_role_id_by_code(client: APIClient, code: str) -> int:
    """通过角色编码获取角色 ID（/roles 返回数组）。"""
    _log.info("查询角色列表，寻找 code=%s", code)
    roles = client.get("/roles", scoped=False)
    for r in roles:
        if r.get("code") == code:
            _log.info("找到角色：id=%s code=%s", r["id"], r["code"])
            return r["id"]
    raise AssertionError(f"未找到 code={code!r} 的角色")


def _probe_permission(label: str, fn: Callable[[], object]) -> bool:
    """探测非 admin 是否被正确拦截（期望 HTTP 403）。

    返回 True 表示后端正确拒绝，False 表示存在权限漏洞。
    不抛异常，仅记录日志，便于主流程继续执行。
    """
    try:
        expect_error(fn, http_status=403)
        _log.info("权限拦截正常：%s", label)
        return True
    except AssertionError:
        _log.warning("⚠ 权限漏洞: %s — 非 admin 未被拦截 (期望 HTTP 403)", label)
        return False


def _assert_cost_price(payload: Any, expected_visible: bool, label: str) -> None:
    """核心断言：返回体中 costPrice 字段的可见性必须符合 admin/非 admin 预期。

    payload 可能是：
      - dict（单个商品详情）
      - dict（分页响应 {list, total, ...}，检查 list 中每一项）
      - list（直接的商品数组）

    非 admin 看到 costPrice 属于严重漏洞，直接 AssertionError；
    admin 看不到 costPrice 同样说明后端序列化有 bug。
    """
    # 归一化出待检查的商品 dict 列表
    if isinstance(payload, dict) and "list" in payload and isinstance(payload["list"], list):
        items = payload["list"]
    elif isinstance(payload, list):
        items = payload
    elif isinstance(payload, dict):
        items = [payload]
    else:
        raise AssertionError(f"{label}: 无法解析 payload 结构：{type(payload).__name__}")

    if not items:
        # 空列表无法验证，作为警告；在调用处应保证至少有一条记录
        _log.warning("%s: 待检查列表为空，跳过 costPrice 断言", label)
        return

    for idx, item in enumerate(items):
        if not isinstance(item, dict):
            continue
        has_key = "costPrice" in item
        if expected_visible and not has_key:
            raise AssertionError(
                f"{label}[#{idx}] 期望 costPrice 可见，但响应字段缺失：{list(item.keys())}"
            )
        if not expected_visible and has_key:
            raise AssertionError(
                f"{label}[#{idx}] ⚠ 敏感字段泄露！非 admin 不应看到 costPrice={item['costPrice']!r}"
            )
    _log.info(
        "%s: costPrice 可见性校验通过（expected_visible=%s, 检查 %d 条）",
        label, expected_visible, len(items),
    )


# --------------- 主流程 ---------------


def run(client: APIClient) -> None:  # noqa: C901
    """执行商品 CRUD 全流程验证。

    前置条件：调用方（main.py _register_todo）已以 admin 身份登录。
    """
    vulnerabilities: List[str] = []  # 收集发现的权限漏洞

    # --- 准备：确认 admin 身份、拿到 user 角色 id、创建测试用户 ---
    admin_id = SESSION.user.get("id")
    if admin_id is None:
        raise AssertionError("SESSION.user.id 为空，疑似未登录")
    _log.info("当前 admin 身份：id=%s username=%s", admin_id, SESSION.user.get("username"))
    user_role_id = _get_role_id_by_code(client, "user")

    normal_username = _unique_username()
    normal_password = "ProdTest@12345"
    _log.info("准备创建测试用户：username=%s", normal_username)
    normal_user = client.post("/users", json={
        "username": normal_username,
        "password": normal_password,
        "realName": "商品测试用户",
        "roleId": user_role_id,
    }, scoped=False)
    normal_user_id = normal_user["id"]
    _log.info("测试用户创建成功：id=%s", normal_user_id)

    # ========== 第 1 步：admin 创建商品 ==========
    code_a = _unique_code()
    name_a = _unique_name()
    barcode_a = _unique_barcode()
    _log.info("开始创建商品：code=%s name=%s barcode=%s", code_a, name_a, barcode_a)
    created = client.post("/products", json={
        "code": code_a,
        "name": name_a,
        "category": "测试分类",
        "spec": "500g/包",
        "unit": "包",
        "barcode": barcode_a,
        "retailPrice": 99.9,
        "costPrice": 50.0,
    }, scoped=False)
    assert_in("id", created, "创建商品响应")
    assert_eq(created["code"], code_a, "商品.code")
    assert_eq(created["name"], name_a, "商品.name")
    assert_eq(created["barcode"], barcode_a, "商品.barcode")
    assert_eq(created["retailPrice"], 99.9, "商品.retailPrice")
    # admin 创建后必须返回 costPrice
    _assert_cost_price(created, expected_visible=True, label="admin 创建商品响应")
    assert_eq(created["costPrice"], 50.0, "商品.costPrice")
    product_id = created["id"]
    _log.info("商品创建成功：id=%s code=%s", product_id, code_a)

    # ========== 第 2 步：admin 列表 ==========
    _log.info("admin 查询商品列表：keyword=%s", code_a)
    page_data = client.get("/products", params={
        "page": 1, "pageSize": 20, "keyword": code_a,
    }, scoped=False)
    assert_page(page_data, min_total=1)
    matched = [p for p in page_data["list"] if p.get("id") == product_id]
    if not matched:
        raise AssertionError(f"商品列表未找到刚创建的 id={product_id}")
    _assert_cost_price(matched, expected_visible=True, label="admin 商品列表")
    _log.info("admin 列表验证通过：total=%s", page_data["total"])

    # ========== 第 3 步：admin 详情 ==========
    _log.info("admin 获取商品详情：id=%s", product_id)
    detail = client.get(f"/products/{product_id}", scoped=False)
    assert_eq(detail["code"], code_a, "详情.code")
    assert_eq(detail["name"], name_a, "详情.name")
    assert_eq(detail["barcode"], barcode_a, "详情.barcode")
    _assert_cost_price(detail, expected_visible=True, label="admin 商品详情")
    assert_eq(detail["costPrice"], 50.0, "详情.costPrice")
    _log.info("admin 详情验证通过")

    # ========== 第 4 步：admin 修改 ==========
    new_name = name_a + "_改"
    new_retail = 199.9
    _log.info("修改商品：id=%s name→%s retailPrice→%s", product_id, new_name, new_retail)
    updated = client.put(f"/products/{product_id}", json={
        "name": new_name,
        "retailPrice": new_retail,
    }, scoped=False)
    assert_eq(updated["name"], new_name, "修改后.name")
    assert_eq(updated["retailPrice"], new_retail, "修改后.retailPrice")
    # 未传字段不应被清空
    assert_eq(updated["code"], code_a, "修改后.code 应保持不变")
    assert_eq(updated["barcode"], barcode_a, "修改后.barcode 应保持不变")
    assert_eq(updated["costPrice"], 50.0, "修改后.costPrice 应保持不变")
    _log.info("商品修改验证通过")

    # ========== 第 5 步：切换普通用户登录 ==========
    _log.info("切换登录为普通用户 username=%s，用于权限 + 屏蔽验证", normal_username)
    auth_login_flow.run(client, normal_username, normal_password)
    assert_eq(SESSION.role_code, "user", "切换后角色应为 user")

    # ========== 第 6 步：非 admin 屏蔽 + 权限探测 ==========
    # 6a 列表 costPrice 屏蔽
    _log.info("普通用户查询商品列表：keyword=%s", code_a)
    normal_page = client.get("/products", params={
        "page": 1, "pageSize": 20, "keyword": code_a,
    }, scoped=False)
    assert_page(normal_page, min_total=1)
    _assert_cost_price(normal_page, expected_visible=False, label="普通用户商品列表")

    # 6b 详情 costPrice 屏蔽
    _log.info("普通用户获取商品详情：id=%s", product_id)
    normal_detail = client.get(f"/products/{product_id}", scoped=False)
    _assert_cost_price(normal_detail, expected_visible=False, label="普通用户商品详情")
    # 其它可见字段仍应返回
    assert_eq(normal_detail["id"], product_id, "普通用户详情.id")
    assert_eq(normal_detail["code"], code_a, "普通用户详情.code")

    # 6c 非 admin 写操作均应 403
    _log.info("===== 开始非 admin 权限探测 =====")
    if not _probe_permission(
        "非admin创建商品 POST /products",
        lambda: client.post("/products", json={
            "code": _unique_code(),
            "name": _unique_name(),
            "unit": "包",
        }, scoped=False),
    ):
        vulnerabilities.append("非admin可创建商品")

    if not _probe_permission(
        f"非admin修改商品 PUT /products/{product_id}",
        lambda: client.put(f"/products/{product_id}", json={"name": "被篡改"}, scoped=False),
    ):
        vulnerabilities.append("非admin可修改商品")

    if not _probe_permission(
        f"非admin删除商品 DELETE /products/{product_id}",
        lambda: client.delete(f"/products/{product_id}", scoped=False),
    ):
        vulnerabilities.append("非admin可删除商品")

    _log.info("===== 权限探测结束 =====")

    # ========== 第 7 步：切回 admin ==========
    _log.info("切回 admin 身份继续后续测试")
    auth_login_flow.run(client, CONFIG.admin_user, CONFIG.admin_password)

    # ========== 第 8 步：错误路径 ==========
    _log.info("===== 开始错误路径测试 =====")

    # 8a 重复 code
    _log.info("用已存在的 code=%s 再创建，期望 code=10005", code_a)
    expect_error(
        lambda: client.post("/products", json={
            "code": code_a,
            "name": "重复测试",
            "unit": "包",
        }, scoped=False),
        code=10005,
    )
    _log.info("重复 code 拦截验证通过")

    # 8b 缺必填字段 name
    _log.info("创建商品缺 name，期望 HTTP 400")
    expect_error(
        lambda: client.post("/products", json={
            "code": _unique_code(),
            "unit": "包",
        }, scoped=False),
        http_status=400,
    )
    _log.info("缺必填字段拦截验证通过")

    # 8c 未知商品 id
    _log.info("GET 不存在的商品 id=999999，期望 HTTP 404")
    expect_error(
        lambda: client.get("/products/999999", scoped=False),
        http_status=404,
    )
    _log.info("未知商品 GET 404 验证通过")

    _log.info("PUT 不存在的商品 id=999999，期望 HTTP 404")
    expect_error(
        lambda: client.put("/products/999999", json={"name": "ghost"}, scoped=False),
        http_status=404,
    )
    _log.info("未知商品 PUT 404 验证通过")

    _log.info("DELETE 不存在的商品 id=999999，期望 HTTP 404")
    expect_error(
        lambda: client.delete("/products/999999", scoped=False),
        http_status=404,
    )
    _log.info("未知商品 DELETE 404 验证通过")

    _log.info("===== 错误路径测试结束 =====")

    # ========== 第 9 步：删除商品 + 404 确认 ==========
    _log.info("删除测试商品：id=%s", product_id)
    client.delete(f"/products/{product_id}", scoped=False)

    _log.info("验证已删除商品 GET /products/%s 返回 404", product_id)
    expect_error(
        lambda: client.get(f"/products/{product_id}", scoped=False),
        http_status=404,
    )
    _log.info("再次删除同一商品，期望 HTTP 404")
    expect_error(
        lambda: client.delete(f"/products/{product_id}", scoped=False),
        http_status=404,
    )
    _log.info("删除后 404 验证通过")

    # ========== 第 10 步：清理 + 汇总 ==========
    _log.info("清理测试用户：id=%s", normal_user_id)
    client.delete(f"/users/{normal_user_id}", scoped=False)

    if vulnerabilities:
        _log.warning("⚠ 发现 %d 个权限漏洞: %s",
                     len(vulnerabilities), ", ".join(vulnerabilities))
    else:
        _log.info("权限探测未发现漏洞，所有非 admin 操作均被正确拦截")

    _log.info("商品 CRUD 全流程验证完成")
