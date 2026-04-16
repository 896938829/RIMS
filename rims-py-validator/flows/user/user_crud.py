"""用户 CRUD 流程（PRD 第 3 章 - 用户管理）。

预期步骤：
    1) 管理员创建新用户 POST /users
    2) 列表 GET /users 断言新建用户出现
    3) 详情 GET /users/:id
    4) 修改 PUT /users/:id
    5) 重置密码 PUT /users/:id/password
    6) 用新密码登录验证
    7) 用户自己改密码 PUT /users/me/password
    8) 非 admin 权限探测（找漏洞）
    9) 删除 DELETE /users/:id + 验证 404
   10) 重复/非法请求 错误路径覆盖

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

_log = get_logger("flow.user.crud")


# --------------- 工具函数 ---------------


def _unique_username() -> str:
    """生成唯一测试用户名，避免多次运行冲突。"""
    return f"test_user_{int(time.time() * 1000)}"


def _get_role_id_by_code(client: APIClient, code: str) -> int:
    """通过角色编码获取角色 ID。"""
    _log.info("查询角色列表，寻找 code=%s", code)
    roles = client.get("/roles", scoped=False)
    for r in roles:
        if r.get("code") == code:
            _log.info("找到角色：id=%s code=%s name=%s", r["id"], r["code"], r["name"])
            return r["id"]
    raise AssertionError(f"未找到 code={code!r} 的角色")


def _probe_permission(label: str, fn: Callable[[], object]) -> bool:
    """探测非 admin 用户是否被正确拦截。

    返回 True 表示后端正确返回 403，False 表示存在权限漏洞。
    不会抛出异常，只记录日志。
    """
    try:
        expect_error(fn, http_status=403)
        _log.info("权限拦截正常：%s", label)
        return True
    except AssertionError:
        # 没有返回 403 —— 可能请求成功了，也可能返回了其他错误码
        _log.warning("⚠ 权限漏洞: %s — 非 admin 未被拦截 (期望 HTTP 403)", label)
        return False


# --------------- 主流程 ---------------


def run(client: APIClient) -> None:  # noqa: C901
    """执行用户 CRUD 全流程验证。

    前置条件：调用方已以 admin 身份登录，client 已持有有效 token。
    """
    vulnerabilities: List[str] = []  # 收集发现的权限漏洞

    # --- 准备：获取 user 角色的 ID（创建普通用户用） ---
    user_role_id = _get_role_id_by_code(client, "user")

    # ========== 第 1 步：创建新用户 ==========
    test_username = _unique_username()
    test_password = "Test@12345"
    _log.info("创建测试用户：username=%s roleId=%s", test_username, user_role_id)
    created = client.post("/users", json={
        "username": test_username,
        "password": test_password,
        "realName": "测试用户",
        "phone": "13800000001",
        "email": f"{test_username}@test.com",
        "roleId": user_role_id,
    }, scoped=False)

    assert_in("id", created, "创建用户响应")
    assert_in("username", created, "创建用户响应")
    assert_eq(created["username"], test_username, "创建用户.username")
    user_id = created["id"]
    _log.info("用户创建成功：id=%s username=%s", user_id, test_username)

    # ========== 第 2 步：列表查询，断言新用户出现 ==========
    _log.info("查询用户列表：keyword=%s", test_username)
    page_data = client.get("/users", params={
        "page": 1, "pageSize": 10, "keyword": test_username,
    }, scoped=False)
    assert_page(page_data, min_total=1)

    # 确认新用户在列表中
    found = any(u["id"] == user_id for u in page_data["list"])
    if not found:
        raise AssertionError(f"用户列表中未找到刚创建的用户 id={user_id}")
    _log.info("用户列表验证通过：total=%s，目标用户已出现", page_data["total"])

    # ========== 第 3 步：查看用户详情 ==========
    _log.info("获取用户详情：id=%s", user_id)
    detail = client.get(f"/users/{user_id}", scoped=False)
    assert_eq(detail["username"], test_username, "用户详情.username")
    assert_eq(detail["realName"], "测试用户", "用户详情.realName")
    assert_eq(detail["phone"], "13800000001", "用户详情.phone")
    _log.info("用户详情验证通过：username=%s realName=%s", detail["username"], detail["realName"])

    # ========== 第 4 步：修改用户信息 ==========
    _log.info("修改用户信息：id=%s realName→测试用户_改 phone→13900000002", user_id)
    updated = client.put(f"/users/{user_id}", json={
        "realName": "测试用户_改",
        "phone": "13900000002",
    }, scoped=False)
    assert_eq(updated["realName"], "测试用户_改", "修改后.realName")
    assert_eq(updated["phone"], "13900000002", "修改后.phone")
    _log.info("用户信息修改验证通过")

    # ========== 第 5 步：管理员重置密码 ==========
    new_password = "Reset@99999"
    _log.info("管理员重置用户密码：userId=%s", user_id)
    client.put(f"/users/{user_id}/password", json={
        "newPassword": new_password,
    }, scoped=False)
    _log.info("密码重置成功")

    # ========== 第 6 步：用新密码登录验证 ==========
    _log.info("用重置后的密码登录：username=%s", test_username)
    auth_login_flow.run(client, test_username, new_password)
    assert_eq(SESSION.role_code, "user", "测试用户角色")
    _log.info("重置密码登录验证通过")

    # ========== 第 7 步：用户自己改密码 ==========
    self_new_password = "SelfChange@111"
    _log.info("用户自行修改密码：PUT /users/me/password")
    client.put("/users/me/password", json={
        "oldPassword": new_password,
        "newPassword": self_new_password,
    }, scoped=False)
    _log.info("自行改密成功，验证新密码能登录")

    # 用新密码重新登录确认
    auth_login_flow.run(client, test_username, self_new_password)
    _log.info("自行改密后登录验证通过")

    # ========== 第 8 步：非 admin 权限探测（找漏洞） ==========
    # 当前已以 test_user (普通用户) 身份登录
    _log.info("===== 开始非 admin 权限探测 =====")

    # 创建第二个测试用户作为探测目标
    probe_username = _unique_username()

    # 8a) 非 admin 尝试创建用户
    if not _probe_permission("非admin创建用户 POST /users", lambda: client.post(
        "/users", json={
            "username": probe_username,
            "password": "Probe@12345",
            "realName": "探测用户",
            "roleId": user_role_id,
        }, scoped=False,
    )):
        vulnerabilities.append("非admin可创建用户")

    # 8b) 非 admin 尝试修改其他用户
    if not _probe_permission(f"非admin修改用户 PUT /users/{user_id}", lambda: client.put(
        f"/users/{user_id}", json={"realName": "被篡改"}, scoped=False,
    )):
        vulnerabilities.append("非admin可修改其他用户")

    # 8c) 非 admin 尝试重置其他用户密码
    if not _probe_permission(f"非admin重置密码 PUT /users/{user_id}/password", lambda: client.put(
        f"/users/{user_id}/password", json={"newPassword": "Hacked@000"}, scoped=False,
    )):
        vulnerabilities.append("非admin可重置他人密码")

    # 8d) 非 admin 尝试删除用户
    if not _probe_permission(f"非admin删除用户 DELETE /users/{user_id}", lambda: client.delete(
        f"/users/{user_id}", scoped=False,
    )):
        vulnerabilities.append("非admin可删除用户")

    _log.info("===== 权限探测结束 =====")

    # 切回 admin 继续后续操作
    _log.info("切回 admin 身份")
    auth_login_flow.run(client, CONFIG.admin_user, CONFIG.admin_password)

    # ========== 第 9 步：删除用户 + 验证 404 ==========
    _log.info("删除测试用户：id=%s", user_id)
    client.delete(f"/users/{user_id}", scoped=False)
    _log.info("用户删除成功")

    # 验证已删除用户返回 404
    _log.info("验证已删除用户 GET /users/%s 返回 404", user_id)
    expect_error(
        lambda: client.get(f"/users/{user_id}", scoped=False),
        http_status=404,
    )
    _log.info("删除后 404 验证通过")

    # 如果探测阶段非 admin 创建了 probe_username，也需要清理
    # 先查一下是否存在
    _log.info("检查探测阶段是否意外创建了用户 %s", probe_username)
    probe_page = client.get("/users", params={
        "page": 1, "pageSize": 10, "keyword": probe_username,
    }, scoped=False)
    if isinstance(probe_page, dict) and probe_page.get("total", 0) > 0:
        for u in probe_page["list"]:
            if u.get("username") == probe_username:
                _log.warning("清理探测阶段意外创建的用户：id=%s username=%s", u["id"], probe_username)
                client.delete(f"/users/{u['id']}", scoped=False)

    # ========== 第 10 步：错误路径覆盖 ==========
    _log.info("===== 开始错误路径测试 =====")

    # 10a) 重复用户名
    # 先创建一个用户，再用相同用户名创建
    dup_username = _unique_username()
    _log.info("创建用户用于重复测试：username=%s", dup_username)
    dup_user = client.post("/users", json={
        "username": dup_username,
        "password": "Dup@123456",
        "realName": "重复测试",
        "roleId": user_role_id,
    }, scoped=False)
    dup_user_id = dup_user["id"]

    _log.info("尝试创建同名用户，期望 code=10005")
    expect_error(
        lambda: client.post("/users", json={
            "username": dup_username,
            "password": "Dup@123456",
            "realName": "重复测试2",
            "roleId": user_role_id,
        }, scoped=False),
        code=10005,
    )
    _log.info("重复用户名拦截验证通过")

    # 10b) 查询不存在的用户
    _log.info("查询不存在的用户 id=999999，期望 404")
    expect_error(
        lambda: client.get("/users/999999", scoped=False),
        http_status=404,
    )
    _log.info("不存在用户 404 验证通过")

    # 10c) 缺少必填字段
    _log.info("创建用户缺少 username，期望 400")
    expect_error(
        lambda: client.post("/users", json={
            "password": "NoName@123",
            "roleId": user_role_id,
        }, scoped=False),
        http_status=400,
    )
    _log.info("缺少必填字段拦截验证通过")

    # 10d) 删除已删除的用户（先删除 dup_user，再删一次）
    _log.info("删除用户 id=%s 用于重复删除测试", dup_user_id)
    client.delete(f"/users/{dup_user_id}", scoped=False)
    _log.info("再次删除同一用户，期望 404")
    expect_error(
        lambda: client.delete(f"/users/{dup_user_id}", scoped=False),
        http_status=404,
    )
    _log.info("重复删除 404 验证通过")

    _log.info("===== 错误路径测试结束 =====")

    # ========== 汇总 ==========
    if vulnerabilities:
        _log.warning("⚠ 发现 %d 个权限漏洞: %s", len(vulnerabilities), ", ".join(vulnerabilities))
    else:
        _log.info("权限探测未发现漏洞，所有非 admin 操作均被正确拦截")

    _log.info("用户 CRUD 全流程验证完成")
