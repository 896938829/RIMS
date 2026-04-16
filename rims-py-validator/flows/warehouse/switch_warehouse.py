"""仓库切换流程（PRD 第 4 章 - 多仓库切换）。

预期步骤：
     1) 准备：建 wh_a / wh_b 两个新仓，绑 admin 到两者（admin 可多仓）
     2) GET /users/me/warehouses 验证 admin 三项绑定（seed WH001 + wh_a + wh_b）
     3) PUT /users/me/warehouses/default 设 wh_a 为默认 → 回查 isDefault 翻转
     4) PUT /users/me/warehouses/default 再设 wh_b 为默认 → 回查
     5) PUT /users/me/warehouses/current 切到 wh_a → 响应 warehouse.id=wh_a
     6) PUT /users/me/warehouses/current 切到 wh_b → 响应 warehouse.id=wh_b
     7) 创建普通用户（只绑 wh_a），登录为他
     8) 普通用户 GET /users/me/warehouses 验证单项绑定 + isDefault=true
     9) 普通用户 PUT /default wh_a / PUT /current wh_a → OK
    10) 错误路径：
        - PUT /default wh_b → 403（未绑）
        - PUT /current wh_b → 403
        - PUT /current 999999 → 403
        - PUT /default 空 body → 400
    11) 切回 admin；临时禁用 wh_b；PUT /current wh_b → 20002(invalid_state)；恢复
    12) 清理：默认仓恢复到 WH001；删 wh_a / wh_b（级联解绑）；删普通用户

测试原则：模拟用户操作，寻找后端漏洞。
"""

from __future__ import annotations

import time
from typing import Callable, List, Optional

from core.assertions import assert_eq, assert_in, expect_error
from core.client import APIClient
from core.config import CONFIG
from core.logger import get_logger
from core.session import SESSION
from flows.auth import login as auth_login_flow

_log = get_logger("flow.warehouse.switch")


# --------------- 工具函数 ---------------


def _unique_code() -> str:
    """生成唯一仓库编码。"""
    return f"sw_{int(time.time() * 1000)}"


def _unique_name() -> str:
    """生成唯一仓库名。"""
    return f"切换测试仓_{int(time.time() * 1000)}"


def _unique_username() -> str:
    """生成唯一测试用户名。"""
    return f"sw_test_user_{int(time.time() * 1000)}"


def _get_role_id_by_code(client: APIClient, code: str) -> int:
    """通过角色编码获取角色 ID。"""
    roles = client.get("/roles", scoped=False)
    for r in roles:
        if r.get("code") == code:
            return r["id"]
    raise AssertionError(f"未找到 code={code!r} 的角色")


def _find_seed_warehouse_id(client: APIClient, code: str) -> int:
    """按 code 查找 seed 仓库（WH001）的 ID。"""
    page = client.get("/warehouses", params={
        "page": 1, "pageSize": 50, "keyword": code,
    }, scoped=False)
    for w in page.get("list") or []:
        if w.get("code") == code:
            return w["id"]
    raise AssertionError(f"未找到 seed 仓库 code={code!r}")


def _find_binding(bindings: list, warehouse_id: int) -> Optional[dict]:
    """在 /users/me/warehouses 返回的绑定数组中找指定 warehouseId。"""
    for b in bindings or []:
        if b.get("warehouseId") == warehouse_id:
            return b
    return None


def _probe_permission(label: str, fn: Callable[[], object]) -> bool:
    """探测非 admin 被拦截（期望 HTTP 403）。"""
    try:
        expect_error(fn, http_status=403)
        _log.info("权限拦截正常：%s", label)
        return True
    except AssertionError:
        _log.warning("⚠ 权限漏洞: %s — 期望 HTTP 403", label)
        return False


# --------------- 主流程 ---------------


def run(client: APIClient) -> None:  # noqa: C901
    """执行仓库切换全流程验证。

    前置条件：调用方已以 admin 身份登录。
    """
    vulnerabilities: List[str] = []

    # --- 准备：admin 身份、seed 仓库 ID、普通用户角色 ID ---
    admin_id = SESSION.user.get("id")
    if admin_id is None:
        raise AssertionError("SESSION.user.id 为空")
    _log.info("当前 admin：id=%s", admin_id)

    _log.info("定位 seed 仓库 WH001")
    seed_wh_id = _find_seed_warehouse_id(client, "WH001")
    _log.info("seed 仓库 WH001.id=%s", seed_wh_id)

    user_role_id = _get_role_id_by_code(client, "user")

    # 创建 wh_a / wh_b 两个新仓库
    code_a, code_b = _unique_code(), _unique_code() + "_b"
    _log.info("创建仓库 A：code=%s", code_a)
    wh_a = client.post("/warehouses", json={
        "code": code_a, "name": _unique_name(),
    }, scoped=False)
    wh_a_id = wh_a["id"]
    _log.info("创建仓库 B：code=%s", code_b)
    wh_b = client.post("/warehouses", json={
        "code": code_b, "name": _unique_name(),
    }, scoped=False)
    wh_b_id = wh_b["id"]
    _log.info("仓库就绪：wh_a=%s wh_b=%s", wh_a_id, wh_b_id)

    # 绑 admin 到 wh_a / wh_b（admin 可多仓）
    _log.info("绑定 admin 到 wh_a")
    client.post(f"/warehouses/{wh_a_id}/users",
                json={"userIds": [admin_id]}, scoped=False)
    _log.info("绑定 admin 到 wh_b")
    client.post(f"/warehouses/{wh_b_id}/users",
                json={"userIds": [admin_id]}, scoped=False)

    # ========== 第 2 步：GET /users/me/warehouses ==========
    _log.info("查询 admin 的绑定仓库列表")
    my_whs = client.get("/users/me/warehouses", scoped=False)
    if not isinstance(my_whs, list) or len(my_whs) < 3:
        raise AssertionError(
            f"admin 绑定至少 3 项（seed+a+b），实际 {len(my_whs) if isinstance(my_whs, list) else my_whs!r}"
        )
    for wid in (seed_wh_id, wh_a_id, wh_b_id):
        if _find_binding(my_whs, wid) is None:
            raise AssertionError(f"admin 绑定列表缺少 warehouseId={wid}")
    _log.info("admin 绑定校验通过：共 %d 项", len(my_whs))

    # ========== 第 3 步：设 wh_a 为默认 ==========
    _log.info("设 wh_a 为默认仓库")
    client.put("/users/me/warehouses/default",
               json={"warehouseId": wh_a_id}, scoped=False)
    after1 = client.get("/users/me/warehouses", scoped=False)
    ba = _find_binding(after1, wh_a_id)
    if ba is None:
        raise AssertionError("wh_a 绑定丢失")
    assert_eq(ba["isDefault"], True, "wh_a.isDefault 设置后")
    # 其余仓应不再是 default
    for wid in (seed_wh_id, wh_b_id):
        b = _find_binding(after1, wid)
        if b is not None:
            assert_eq(b["isDefault"], False, f"warehouseId={wid}.isDefault 应为 false")
    _log.info("默认仓切至 wh_a 验证通过")

    # ========== 第 4 步：再切 wh_b 为默认 ==========
    _log.info("再把默认仓切到 wh_b")
    client.put("/users/me/warehouses/default",
               json={"warehouseId": wh_b_id}, scoped=False)
    after2 = client.get("/users/me/warehouses", scoped=False)
    bb = _find_binding(after2, wh_b_id)
    if bb is None:
        raise AssertionError("wh_b 绑定丢失")
    assert_eq(bb["isDefault"], True, "wh_b.isDefault")
    ba2 = _find_binding(after2, wh_a_id)
    if ba2 is not None:
        assert_eq(ba2["isDefault"], False, "wh_a.isDefault 被切走后应 false")
    _log.info("默认仓切至 wh_b 验证通过")

    # ========== 第 5-6 步：PUT /current 切换 ==========
    _log.info("切当前仓库到 wh_a")
    cur_a = client.put("/users/me/warehouses/current",
                       json={"warehouseId": wh_a_id}, scoped=False)
    assert_in("id", cur_a, "切当前仓响应")
    assert_eq(cur_a["id"], wh_a_id, "current.id")
    _log.info("切当前仓 wh_a 响应正确")

    _log.info("切当前仓库到 wh_b")
    cur_b = client.put("/users/me/warehouses/current",
                       json={"warehouseId": wh_b_id}, scoped=False)
    assert_eq(cur_b["id"], wh_b_id, "current.id")
    _log.info("切当前仓 wh_b 响应正确")

    # ========== 第 7 步：普通用户场景 ==========
    normal_username = _unique_username()
    normal_password = "SwTest@12345"
    _log.info("创建普通测试用户：%s", normal_username)
    normal_user = client.post("/users", json={
        "username": normal_username,
        "password": normal_password,
        "realName": "切换测试用户",
        "roleId": user_role_id,
    }, scoped=False)
    normal_user_id = normal_user["id"]

    _log.info("绑普通用户到 wh_a（单仓）")
    client.post(f"/warehouses/{wh_a_id}/users",
                json={"userIds": [normal_user_id]}, scoped=False)

    _log.info("切换登录为普通用户")
    auth_login_flow.run(client, normal_username, normal_password)
    assert_eq(SESSION.role_code, "user", "角色应为 user")

    # ========== 第 8 步：普通用户查绑定列表 ==========
    my_nu = client.get("/users/me/warehouses", scoped=False)
    if not isinstance(my_nu, list) or len(my_nu) != 1:
        raise AssertionError(f"普通用户期望 1 项绑定，实际 {my_nu!r}")
    assert_eq(my_nu[0]["warehouseId"], wh_a_id, "普通用户.warehouseId")
    assert_eq(my_nu[0]["isDefault"], True, "普通用户.isDefault 首次绑定应 true")
    _log.info("普通用户绑定列表校验通过")

    # ========== 第 9 步：普通用户合法 default/current ==========
    _log.info("普通用户：PUT /default wh_a（应成功，幂等）")
    client.put("/users/me/warehouses/default",
               json={"warehouseId": wh_a_id}, scoped=False)
    _log.info("普通用户：PUT /current wh_a（应成功）")
    nu_cur = client.put("/users/me/warehouses/current",
                        json={"warehouseId": wh_a_id}, scoped=False)
    assert_eq(nu_cur["id"], wh_a_id, "普通用户.current.id")
    _log.info("普通用户合法切换通过")

    # ========== 第 10 步：普通用户错误路径 ==========
    _log.info("===== 普通用户错误路径 =====")

    # 10a 对未绑仓 wh_b 设默认 → 403
    _log.info("普通用户对未绑 wh_b 设默认，期望 403")
    if not _probe_permission(
        "普通用户 PUT /default 未绑仓",
        lambda: client.put("/users/me/warehouses/default",
                           json={"warehouseId": wh_b_id}, scoped=False),
    ):
        vulnerabilities.append("未绑用户可把未绑仓设为默认")

    # 10b 对未绑仓 wh_b 切 current → 403
    _log.info("普通用户对未绑 wh_b 切 current，期望 403")
    if not _probe_permission(
        "普通用户 PUT /current 未绑仓",
        lambda: client.put("/users/me/warehouses/current",
                           json={"warehouseId": wh_b_id}, scoped=False),
    ):
        vulnerabilities.append("未绑用户可切换到未绑仓")

    # 10c 对不存在的 id 切 current → 403（后端实现：查绑定找不到 → 403）
    _log.info("普通用户切到 warehouseId=999999，期望 403")
    expect_error(
        lambda: client.put("/users/me/warehouses/current",
                           json={"warehouseId": 999999}, scoped=False),
        http_status=403,
    )
    _log.info("不存在仓 切 current 403 验证通过")

    # 10d 缺必填字段 warehouseId → 400
    _log.info("普通用户 PUT /default 空 body，期望 400")
    expect_error(
        lambda: client.put("/users/me/warehouses/default",
                           json={}, scoped=False),
        http_status=400,
    )
    _log.info("缺必填拦截验证通过")

    _log.info("===== 普通用户错误路径结束 =====")

    # ========== 第 11 步：切回 admin + 禁用仓切 current ==========
    _log.info("切回 admin")
    auth_login_flow.run(client, CONFIG.admin_user, CONFIG.admin_password)

    _log.info("临时禁用 wh_b：status→0")
    client.put(f"/warehouses/{wh_b_id}", json={"status": 0}, scoped=False)

    _log.info("admin 切 current 到禁用的 wh_b，期望 code=20002")
    expect_error(
        lambda: client.put("/users/me/warehouses/current",
                           json={"warehouseId": wh_b_id}, scoped=False),
        code=20002,
    )
    _log.info("禁用仓切 current 拦截通过")

    _log.info("恢复 wh_b 状态")
    client.put(f"/warehouses/{wh_b_id}", json={"status": 1}, scoped=False)

    # ========== 第 12 步：清理 ==========
    _log.info("===== 清理 =====")

    # 把 admin 默认仓恢复回 seed WH001（避免测试后遗症）
    _log.info("恢复 admin 默认仓为 WH001(id=%s)", seed_wh_id)
    client.put("/users/me/warehouses/default",
               json={"warehouseId": seed_wh_id}, scoped=False)

    _log.info("删除 wh_a（级联解绑）：id=%s", wh_a_id)
    client.delete(f"/warehouses/{wh_a_id}", scoped=False)
    _log.info("删除 wh_b（级联解绑）：id=%s", wh_b_id)
    client.delete(f"/warehouses/{wh_b_id}", scoped=False)

    _log.info("删除普通测试用户：id=%s", normal_user_id)
    client.delete(f"/users/{normal_user_id}", scoped=False)

    if vulnerabilities:
        _log.warning("⚠ 发现 %d 个权限漏洞: %s",
                     len(vulnerabilities), ", ".join(vulnerabilities))
    else:
        _log.info("权限探测未发现漏洞")

    _log.info("仓库切换全流程验证完成")
