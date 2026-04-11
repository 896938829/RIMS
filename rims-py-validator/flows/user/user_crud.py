"""用户 CRUD 流程（PRD 第 3 章 - 用户管理）。

预期步骤：
    1) 管理员创建新用户 POST /users
    2) 列表 GET /users 断言新建用户出现
    3) 详情 GET /users/:id
    4) 修改 PUT /users/:id
    5) 重置密码 PUT /users/:id/password
    6) 删除 DELETE /users/:id

注意：全部需要 admin 角色，非 admin 调用应预期 403（用 expect_error 覆盖）。
"""

from __future__ import annotations

from core.client import APIClient
from flows._placeholder import not_implemented


# TODO: 实现用户 CRUD 全流程
def run(client: APIClient) -> None:
    not_implemented("user.user_crud")
