"""审计日志查询流程（PRD 第 10 章 - 操作审计，admin-only）。

对应 PRD 摘录与 CLAUDE.md 中记录的后端规则：
    - `GET /api/v1/audit/logs` 分页列表 + `GET /api/v1/audit/logs/:id` 详情
    - 路由不做仓库作用域；两处 handler 入口都先 `types.IsAdmin(c)` 判断，
      非 admin → 403 code=10002
    - 时间窗口最大 366 天（service.go parseTimeRange），超过 → 400 ErrValidation
    - 审计写入目前覆盖：用户登录成功/失败（user.Handler.Login 最佳努力写入）、
      单据完成（document.Service.Complete 在事务内写入）

预期业务步骤：
    1) 前置：制造审计数据 —— 故意用错密码触发一次 login failure 审计；
       再用 admin 正确登录一次触发 success 审计
    2) admin 查 `/audit/logs` 列表，按 resource=user / action=login 过滤
    3) 进一步过滤 result=failure，断言刚才的失败事件被记录
    4) 取列表第一条做 `/audit/logs/:id` 详情查询，校验字段结构
    5) 失败路径：时间窗口 > 366 天 → 期望 400
    6) 失败路径：不存在的 id → 期望 404
    7) 权限边界：临时普通用户访问 `/audit/logs` → 期望 403；
       若未拦截则记录为权限漏洞
    8) 清理：切回 admin，删除临时用户

测试原则：模拟用户操作，寻找后端漏洞。所有"应 403 却成功"的场景记录到 vulnerabilities。
"""

from __future__ import annotations

import time
from datetime import datetime, timedelta, timezone
from typing import List, Optional

from core.assertions import assert_eq, assert_in, assert_page, expect_error
from core.client import APIClient, APIError
from core.config import CONFIG
from core.logger import get_logger
from core.session import SESSION
from flows.auth import login as auth_login_flow

_log = get_logger("flow.audit.logs")


# --------------- 工具函数 ---------------


def _unique_username() -> str:
    """生成唯一测试用户名，避免多次运行冲突。"""
    return f"audit_probe_{int(time.time() * 1000)}"


def _get_role_id_by_code(client: APIClient, code: str) -> int:
    """通过角色 code 反查角色 id。"""
    _log.info("查询角色列表寻找 code=%s", code)
    roles = client.get("/roles", scoped=False)
    for r in roles:
        if r.get("code") == code:
            return r["id"]
    raise AssertionError(f"未找到 code={code!r} 的角色")


def _safe_delete_user(client: APIClient, user_id: Optional[int]) -> None:
    """best-effort 删除用户，吞掉 404/403；供 finally 收尾使用。"""
    if not user_id:
        return
    try:
        client.delete(f"/users/{user_id}", scoped=False)
        _log.info("清理测试用户：id=%s", user_id)
    except APIError as e:
        _log.warning("清理用户 id=%s 失败（已忽略）：%s", user_id, e.message)


def _iso(dt: datetime) -> str:
    """把 datetime 格式化成后端能识别的 YYYY-MM-DD（service.parseTimeRange 支持）。"""
    return dt.strftime("%Y-%m-%d")


# --------------- 主流程 ---------------


def run(client: APIClient) -> None:  # noqa: C901
    """执行审计日志查询全流程。

    前置：调用方已以 admin 身份登录（main.py._register_todo 保证）。
    """
    vulnerabilities: List[str] = []
    user_id: Optional[int] = None

    admin_user = CONFIG.admin_user
    admin_pwd = CONFIG.admin_password

    try:
        # ========== 第 1 步：制造审计数据 ==========
        # 1a) 故意用错密码触发一次 login failure（user.Handler.Login 写审计）
        _log.info("制造审计：故意用错密码触发 login failure")
        expect_error(
            lambda: client.post(
                "/auth/login",
                json={"username": admin_user, "password": "definitely_wrong_password_xx"},
                scoped=False,
            ),
        )
        _log.info("login failure 已触发（期望后端写入 result=failure 审计）")

        # 1b) 错密码登录会清掉 client 上的 token 吗？—— 不会，只是抛 APIError；
        # 但为保险起见，重新用 admin 正确凭据登录一次，既恢复 token，也产生 success 审计
        _log.info("admin 重新登录，顺带触发 login success 审计")
        auth_login_flow.run(client, admin_user, admin_pwd)

        # 后端写审计是 best-effort，这里稍微等一下避免刚写完立即查读到不一致
        time.sleep(0.3)

        # ========== 第 2 步：列表过滤 resource=user action=login ==========
        _log.info("查询审计列表：resource=user action=login")
        page = client.get(
            "/audit/logs",
            params={
                "resource": "user",
                "action": "login",
                "page": 1,
                "pageSize": 20,
            },
            scoped=False,
        )
        assert_page(page, min_total=1)
        _log.info("列表返回 total=%s list_len=%s", page["total"], len(page["list"]))
        # 每条记录校验必有字段
        for item in page["list"][:3]:
            for k in ("id", "action", "resource", "result", "createdAt", "userId"):
                assert_in(k, item, "审计日志条目")
        assert_eq(page["list"][0]["resource"], "user", "首条.resource")
        assert_eq(page["list"][0]["action"], "login", "首条.action")
        _log.info("按 resource/action 过滤 OK：首条 id=%s result=%s",
                  page["list"][0]["id"], page["list"][0]["result"])

        # ========== 第 3 步：过滤 result=failure 校验失败事件入库 ==========
        _log.info("过滤 result=failure，验证失败登录已落审计")
        fail_page = client.get(
            "/audit/logs",
            params={
                "resource": "user",
                "action": "login",
                "result": "failure",
                "page": 1,
                "pageSize": 10,
            },
            scoped=False,
        )
        assert_page(fail_page, min_total=1)
        # 所有返回行必须 result=failure，否则后端过滤逻辑有漏
        for item in fail_page["list"]:
            if item.get("result") != "failure":
                raise AssertionError(
                    f"result=failure 过滤失效：返回条目 result={item.get('result')!r}"
                )
        _log.info("result=failure 过滤有效：命中 %d 条", fail_page["total"])

        # ========== 第 4 步：详情查询 ==========
        first_id = page["list"][0]["id"]
        _log.info("查询审计详情：GET /audit/logs/%s", first_id)
        detail = client.get(f"/audit/logs/{first_id}", scoped=False)
        for k in ("id", "action", "resource", "result", "createdAt", "userId",
                  "username", "roleCode"):
            assert_in(k, detail, "审计详情")
        assert_eq(detail["id"], first_id, "详情.id")
        # ipAddress / userAgent 是 omitempty；登录走的是有客户端的场景，
        # 至少 userAgent（python-requests/…）应该存在，缺失则提示
        if not detail.get("userAgent"):
            _log.warning("⚠ 详情未包含 userAgent 字段（可能上游未捕获）")
        _log.info("审计详情 OK：id=%s user=%s role=%s result=%s",
                  detail["id"], detail.get("username"), detail.get("roleCode"),
                  detail["result"])

        # ========== 第 5 步：失败路径 - 时间窗口 > 366 天 ==========
        now = datetime.now(timezone.utc)
        too_old = now - timedelta(days=400)
        _log.info("失败路径：startTime=%s endTime=%s（跨度 > 366d），期望 HTTP 400",
                  _iso(too_old), _iso(now))
        expect_error(
            lambda: client.get(
                "/audit/logs",
                params={
                    "startTime": _iso(too_old),
                    "endTime": _iso(now),
                    "page": 1,
                    "pageSize": 10,
                },
                scoped=False,
            ),
            http_status=400,
        )
        _log.info("时间窗口超限拦截通过")

        # ========== 第 6 步：失败路径 - 不存在的 ID ==========
        _log.info("失败路径：GET /audit/logs/99999999，期望 HTTP 404")
        expect_error(
            lambda: client.get("/audit/logs/99999999", scoped=False),
            http_status=404,
        )
        _log.info("不存在 ID 拦截通过")

        # 额外边界：id=0（非法）
        _log.info("失败路径：GET /audit/logs/0，期望 HTTP 400")
        expect_error(
            lambda: client.get("/audit/logs/0", scoped=False),
            http_status=400,
        )
        _log.info("非法 ID 拦截通过")

        # ========== 第 7 步：权限边界 - 非 admin 访问审计 ==========
        _log.info("===== 开始非 admin 权限探测 =====")
        user_role_id = _get_role_id_by_code(client, "user")
        probe_name = _unique_username()
        probe_pwd = "AuditProbe@12345"
        _log.info("创建临时普通用户：%s", probe_name)
        created = client.post("/users", json={
            "username": probe_name,
            "password": probe_pwd,
            "realName": "审计权限探测",
            "roleId": user_role_id,
        }, scoped=False)
        user_id = created["id"]

        # 切身份
        _log.info("以普通用户身份登录")
        auth_login_flow.run(client, probe_name, probe_pwd)
        assert_eq(SESSION.role_code, "user", "探测用户角色")

        # 7a) 非 admin GET /audit/logs → 期望 403
        try:
            expect_error(
                lambda: client.get(
                    "/audit/logs",
                    params={"page": 1, "pageSize": 5},
                    scoped=False,
                ),
                http_status=403,
            )
            _log.info("权限拦截正常：非 admin 访问 /audit/logs 被 403")
        except AssertionError:
            _log.warning("⚠ 权限漏洞：非 admin 可访问 /audit/logs")
            vulnerabilities.append("非 admin 可访问审计列表")

        # 7b) 非 admin GET /audit/logs/:id → 期望 403
        try:
            expect_error(
                lambda: client.get(f"/audit/logs/{first_id}", scoped=False),
                http_status=403,
            )
            _log.info("权限拦截正常：非 admin 访问 /audit/logs/:id 被 403")
        except AssertionError:
            _log.warning("⚠ 权限漏洞：非 admin 可访问 /audit/logs/%s", first_id)
            vulnerabilities.append("非 admin 可访问审计详情")

        # 切回 admin
        _log.info("切回 admin 身份")
        auth_login_flow.run(client, admin_user, admin_pwd)

        # ========== 漏洞汇总 ==========
        if vulnerabilities:
            _log.warning("本次发现 %d 个潜在漏洞：", len(vulnerabilities))
            for v in vulnerabilities:
                _log.warning("  - %s", v)
        else:
            _log.info("未发现已知类型漏洞")

    finally:
        # ---- 兜底清理 ----
        cleanup_ok = True
        try:
            if SESSION.role_code != "admin":
                _log.info("finally 中切回 admin 以清理资源")
                auth_login_flow.run(client, admin_user, admin_pwd)
        except APIError as e:
            _log.warning("finally 中 admin 重新登录失败（跳过清理）：%s", e.message)
            cleanup_ok = False

        if cleanup_ok:
            _safe_delete_user(client, user_id)
