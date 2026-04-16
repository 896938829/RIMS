"""仓库 CRUD 流程（PRD 第 4 章 - 仓库管理）。

预期步骤：
     1) 管理员创建仓库 POST /warehouses
     2) 列表 GET /warehouses 断言新仓库出现
     3) 详情 GET /warehouses/:id 回查字段
     4) 修改 PUT /warehouses/:id
     5) 绑定普通用户 POST /warehouses/:id/users
        - 同时验证：仓库用户列表、/users/me/warehouses 的 isDefault=true
     6) 非 admin 权限探测 —— 登录普通用户后尝试 admin-only 操作，记录漏洞
     7) "普通用户只能绑 1 个仓库" 约束：再建仓库 B 并尝试绑定同一普通用户
     8) 解绑用户 DELETE /warehouses/:id/users/:userId + 重复解绑 404
     9) 错误路径覆盖：重复 code / 未知 id / 缺必填 / 禁用仓库绑用户
    10) 删除仓库 DELETE /warehouses/:id + GET 返回 404
    11) 清理：删除测试用户；汇总权限漏洞日志

测试原则：模拟用户操作，寻找后端漏洞。
"""

from __future__ import annotations

import time
from typing import Callable, List

from core.assertions import assert_eq, assert_in, assert_page, expect_error
from core.client import APIClient
from core.config import CONFIG
from core.logger import get_logger
from core.session import SESSION
from flows.auth import login as auth_login_flow

_log = get_logger("flow.warehouse.crud")


# --------------- 工具函数 ---------------


def _unique_code() -> str:
    """生成唯一仓库编码，长度在 2~32 之间。"""
    return f"wh_{int(time.time() * 1000)}"


def _unique_name() -> str:
    """生成唯一仓库名称。"""
    return f"测试仓库_{int(time.time() * 1000)}"


def _unique_username() -> str:
    """生成唯一测试用户名。"""
    return f"wh_test_user_{int(time.time() * 1000)}"


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


# --------------- 主流程 ---------------


def run(client: APIClient) -> None:  # noqa: C901
    """执行仓库 CRUD 全流程验证。

    前置条件：调用方（main.py _register_todo）已以 admin 身份登录。
    """
    vulnerabilities: List[str] = []  # 收集发现的权限漏洞

    # --- 准备：记录 admin 身份、拿到 user 角色 ID ---
    admin_id = SESSION.user.get("id")
    if admin_id is None:
        raise AssertionError("SESSION.user.id 为空，疑似未登录")
    _log.info("当前 admin 身份：id=%s username=%s", admin_id, SESSION.user.get("username"))
    user_role_id = _get_role_id_by_code(client, "user")

    # 创建一个普通测试用户，后面既用来绑定仓库也用来做权限探测
    normal_username = _unique_username()
    normal_password = "WhTest@12345"
    _log.info("准备创建测试用户：username=%s", normal_username)
    normal_user = client.post("/users", json={
        "username": normal_username,
        "password": normal_password,
        "realName": "仓库测试用户",
        "roleId": user_role_id,
    }, scoped=False)
    normal_user_id = normal_user["id"]
    _log.info("测试用户创建成功：id=%s", normal_user_id)

    # ========== 第 1 步：创建仓库 A ==========
    code_a = _unique_code()
    name_a = _unique_name()
    _log.info("创建仓库 A：code=%s name=%s", code_a, name_a)
    wh_a = client.post("/warehouses", json={
        "code": code_a,
        "name": name_a,
        "address": "测试地址 A",
        "contactPerson": "张三",
        "contactPhone": "13800000000",
    }, scoped=False)
    assert_in("id", wh_a, "创建仓库响应")
    assert_eq(wh_a["code"], code_a, "仓库.code")
    assert_eq(wh_a["name"], name_a, "仓库.name")
    assert_eq(wh_a["status"], 1, "仓库.status 默认应为 1(启用)")
    assert_eq(wh_a["createdBy"], admin_id, "仓库.createdBy")
    wh_a_id = wh_a["id"]
    _log.info("仓库 A 创建成功：id=%s", wh_a_id)

    # ========== 第 2 步：列表查询 ==========
    _log.info("查询仓库列表：keyword=%s", code_a)
    page_data = client.get("/warehouses", params={
        "page": 1, "pageSize": 20, "keyword": code_a,
    }, scoped=False)
    assert_page(page_data, min_total=1)
    found = any(w["id"] == wh_a_id for w in page_data["list"])
    if not found:
        raise AssertionError(f"仓库列表未找到刚创建的 id={wh_a_id}")
    _log.info("仓库列表验证通过：total=%s，目标仓库已出现", page_data["total"])

    # ========== 第 3 步：详情 ==========
    _log.info("获取仓库详情：id=%s", wh_a_id)
    detail = client.get(f"/warehouses/{wh_a_id}", scoped=False)
    assert_eq(detail["code"], code_a, "详情.code")
    assert_eq(detail["name"], name_a, "详情.name")
    assert_eq(detail["address"], "测试地址 A", "详情.address")
    assert_eq(detail["contactPerson"], "张三", "详情.contactPerson")
    _log.info("仓库详情验证通过")

    # ========== 第 4 步：修改 ==========
    new_name = name_a + "_改"
    new_phone = "13900000001"
    _log.info("修改仓库：id=%s name→%s phone→%s", wh_a_id, new_name, new_phone)
    updated = client.put(f"/warehouses/{wh_a_id}", json={
        "name": new_name,
        "contactPhone": new_phone,
    }, scoped=False)
    assert_eq(updated["name"], new_name, "修改后.name")
    assert_eq(updated["contactPhone"], new_phone, "修改后.contactPhone")
    # 未传字段不应该被清空
    assert_eq(updated["code"], code_a, "修改后.code 应保持不变")
    assert_eq(updated["contactPerson"], "张三", "修改后.contactPerson 应保持不变")
    _log.info("仓库修改验证通过")

    # ========== 第 5 步：绑定普通用户到仓库 A ==========
    _log.info("绑定测试用户到仓库 A：userId=%s warehouseId=%s", normal_user_id, wh_a_id)
    client.post(f"/warehouses/{wh_a_id}/users", json={
        "userIds": [normal_user_id],
    }, scoped=False)
    _log.info("绑定成功，校验仓库用户列表")

    # 5.1 仓库用户列表能看到此用户
    wh_users = client.get(f"/warehouses/{wh_a_id}/users", params={
        "page": 1, "pageSize": 20,
    }, scoped=False)
    assert_page(wh_users, min_total=1)
    if not any(u.get("id") == normal_user_id or u.get("userId") == normal_user_id
               for u in wh_users["list"]):
        raise AssertionError(f"仓库用户列表未包含 userId={normal_user_id}")
    _log.info("仓库用户列表包含测试用户")

    # 5.2 以普通用户身份登录，校验 /users/me/warehouses 的 isDefault=true
    _log.info("切换登录为普通用户，验证其默认仓库=%s", wh_a_id)
    auth_login_flow.run(client, normal_username, normal_password)
    assert_eq(SESSION.role_code, "user", "切换后角色应为 user")
    my_whs = client.get("/users/me/warehouses", scoped=False)
    if not isinstance(my_whs, list) or len(my_whs) != 1:
        raise AssertionError(f"/users/me/warehouses 期望单项数组，实际 {my_whs!r}")
    assert_eq(my_whs[0]["warehouseId"], wh_a_id, "my_warehouses.warehouseId")
    assert_eq(my_whs[0]["isDefault"], True, "my_warehouses.isDefault 首次绑定应为 true")
    _log.info("普通用户默认仓库校验通过")

    # ========== 第 6 步：非 admin 权限探测 ==========
    _log.info("===== 开始非 admin 权限探测（当前身份=普通用户）=====")

    # 6a 创建仓库
    if not _probe_permission(
        "非admin创建仓库 POST /warehouses",
        lambda: client.post("/warehouses", json={
            "code": _unique_code(),
            "name": _unique_name(),
        }, scoped=False),
    ):
        vulnerabilities.append("非admin可创建仓库")

    # 6b 修改仓库
    if not _probe_permission(
        f"非admin修改仓库 PUT /warehouses/{wh_a_id}",
        lambda: client.put(f"/warehouses/{wh_a_id}", json={
            "name": "被篡改",
        }, scoped=False),
    ):
        vulnerabilities.append("非admin可修改仓库")

    # 6c 删除仓库
    if not _probe_permission(
        f"非admin删除仓库 DELETE /warehouses/{wh_a_id}",
        lambda: client.delete(f"/warehouses/{wh_a_id}", scoped=False),
    ):
        vulnerabilities.append("非admin可删除仓库")

    # 6d 绑定用户（普通用户尝试把 admin 绑到仓库）
    if not _probe_permission(
        f"非admin绑定用户 POST /warehouses/{wh_a_id}/users",
        lambda: client.post(f"/warehouses/{wh_a_id}/users", json={
            "userIds": [admin_id],
        }, scoped=False),
    ):
        vulnerabilities.append("非admin可绑定用户到仓库")

    # 6e 解绑用户
    if not _probe_permission(
        f"非admin解绑用户 DELETE /warehouses/{wh_a_id}/users/{admin_id}",
        lambda: client.delete(f"/warehouses/{wh_a_id}/users/{admin_id}", scoped=False),
    ):
        vulnerabilities.append("非admin可解绑仓库用户")

    _log.info("===== 权限探测结束 =====")

    # 切回 admin 继续后续步骤
    _log.info("切回 admin 身份")
    auth_login_flow.run(client, CONFIG.admin_user, CONFIG.admin_password)

    # ========== 第 7 步：普通用户只能绑 1 个仓库 ==========
    code_b = _unique_code()
    name_b = _unique_name()
    _log.info("创建仓库 B 用于单仓约束测试：code=%s", code_b)
    wh_b = client.post("/warehouses", json={
        "code": code_b,
        "name": name_b,
    }, scoped=False)
    wh_b_id = wh_b["id"]

    _log.info("尝试把已绑 A 的普通用户再绑到 B，期望 code=10003(validation)")
    expect_error(
        lambda: client.post(f"/warehouses/{wh_b_id}/users", json={
            "userIds": [normal_user_id],
        }, scoped=False),
        code=10003,
    )
    _log.info("单仓约束拦截验证通过")

    # 清理仓库 B（避免遗留）
    _log.info("清理仓库 B：id=%s", wh_b_id)
    client.delete(f"/warehouses/{wh_b_id}", scoped=False)

    # ========== 第 8 步：解绑用户 ==========
    _log.info("解绑用户：warehouseId=%s userId=%s", wh_a_id, normal_user_id)
    client.delete(f"/warehouses/{wh_a_id}/users/{normal_user_id}", scoped=False)

    # 8.1 仓库用户列表不再包含该用户
    wh_users_after = client.get(f"/warehouses/{wh_a_id}/users", params={
        "page": 1, "pageSize": 20,
    }, scoped=False)
    if any(u.get("id") == normal_user_id or u.get("userId") == normal_user_id
           for u in (wh_users_after.get("list") or [])):
        raise AssertionError(f"解绑后仓库用户列表仍包含 userId={normal_user_id}")
    _log.info("解绑后仓库用户列表已无该用户")

    # 8.2 重复解绑期望 404(10004 绑定不存在)
    _log.info("重复解绑同一用户，期望 code=10004")
    expect_error(
        lambda: client.delete(f"/warehouses/{wh_a_id}/users/{normal_user_id}", scoped=False),
        code=10004,
    )
    _log.info("重复解绑 404 验证通过")

    # ========== 第 9 步：错误路径 ==========
    _log.info("===== 开始错误路径测试 =====")

    # 9a 重复仓库编码
    _log.info("用已存在的 code=%s 再创建，期望 code=10005", code_a)
    expect_error(
        lambda: client.post("/warehouses", json={
            "code": code_a,
            "name": "重复测试",
        }, scoped=False),
        code=10005,
    )
    _log.info("重复编码拦截验证通过")

    # 9b 未知仓库 id
    _log.info("查询不存在的仓库 id=999999，期望 HTTP 404")
    expect_error(
        lambda: client.get("/warehouses/999999", scoped=False),
        http_status=404,
    )
    _log.info("不存在仓库 404 验证通过")

    # 9c 缺必填字段 code
    _log.info("创建仓库缺 code，期望 HTTP 400")
    expect_error(
        lambda: client.post("/warehouses", json={
            "name": "缺 code 测试",
        }, scoped=False),
        http_status=400,
    )
    _log.info("缺必填字段拦截验证通过")

    # 9d 禁用仓库绑用户：先改状态，再尝试绑定，再改回
    _log.info("把仓库 A 临时禁用：status→0")
    client.put(f"/warehouses/{wh_a_id}", json={"status": 0}, scoped=False)

    _log.info("给禁用仓库绑用户，期望 code=20002(invalid_state)")
    expect_error(
        lambda: client.post(f"/warehouses/{wh_a_id}/users", json={
            "userIds": [normal_user_id],
        }, scoped=False),
        code=20002,
    )
    _log.info("禁用仓库拦截验证通过，恢复状态")
    client.put(f"/warehouses/{wh_a_id}", json={"status": 1}, scoped=False)

    _log.info("===== 错误路径测试结束 =====")

    # ========== 第 10 步：删除仓库 + 404 确认 ==========
    _log.info("删除仓库 A：id=%s", wh_a_id)
    client.delete(f"/warehouses/{wh_a_id}", scoped=False)

    _log.info("验证已删除仓库 GET /warehouses/%s 返回 404", wh_a_id)
    expect_error(
        lambda: client.get(f"/warehouses/{wh_a_id}", scoped=False),
        http_status=404,
    )

    _log.info("再次删除同一仓库，期望 HTTP 404")
    expect_error(
        lambda: client.delete(f"/warehouses/{wh_a_id}", scoped=False),
        http_status=404,
    )
    _log.info("删除后 404 验证通过")

    # ========== 第 11 步：清理 + 汇总 ==========
    _log.info("清理测试用户：id=%s", normal_user_id)
    client.delete(f"/users/{normal_user_id}", scoped=False)

    if vulnerabilities:
        _log.warning("⚠ 发现 %d 个权限漏洞: %s",
                     len(vulnerabilities), ", ".join(vulnerabilities))
    else:
        _log.info("权限探测未发现漏洞，所有非 admin 操作均被正确拦截")

    _log.info("仓库 CRUD 全流程验证完成")
