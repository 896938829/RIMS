"""登录流程（对应 PRD 第 3 章 - 用户登录与鉴权）。

业务步骤：
    1) POST /auth/login 用用户名+密码换取 JWT token
    2) 把 token 写入 APIClient 与全局 SESSION
    3) GET /users/me 验证 token 确实生效
    4) 打印当前登录用户 + 角色，供后续流程复用

本文件是最小验证闭环的核心，其它所有 flows 都依赖登录成功后的 session。
"""

from __future__ import annotations

from core.assertions import assert_in
from core.client import APIClient
from core.logger import get_logger
from core.session import SESSION

_log = get_logger("flow.auth.login")


def run(client: APIClient, username: str, password: str) -> dict:
    """执行登录流程，返回登录后的 user 信息字典。

    参数：
        client: 共享 HTTP 客户端（本函数会向其写入 token）
        username: 登录用户名
        password: 登录密码

    返回：
        后端返回的 `user` 对象（含 id/username/realName/roleCode/roleName）

    抛出：
        APIError — 登录失败（密码错误、用户不存在、后端异常）
        AssertionError — 返回结构不符预期
    """
    # 第 1 步：调用登录接口，注意这是公开路由，不需要 warehouse 头
    _log.info("开始登录：username=%s", username)
    data = client.post(
        "/auth/login",
        json={"username": username, "password": password},
        scoped=False,  # 公开路由不需要 X-Warehouse-ID
    )

    # 校验返回结构
    assert_in("token", data, "登录响应")
    assert_in("user", data, "登录响应")
    token: str = data["token"]
    user: dict = data["user"]
    assert_in("roleCode", user, "登录响应.user")

    # 第 2 步：把 token 写入客户端与全局会话，后续请求自动带上
    client.set_token(token)
    SESSION.token = token
    SESSION.user = user
    SESSION.role_code = user["roleCode"]
    _log.info(
        "登录成功：userId=%s username=%s role=%s",
        user.get("id"),
        user.get("username"),
        user.get("roleCode"),
    )

    # 第 3 步：调 /users/me 确认 token 生效（同时验证 JWT 中间件工作正常）
    _log.info("校验 token：GET /users/me")
    me = client.get("/users/me", scoped=False)
    assert_in("username", me, "GET /users/me 响应")
    _log.info("token 有效，当前用户=%s realName=%s", me.get("username"), me.get("realName"))

    return user
