"""仓库 CRUD 流程（PRD 第 4 章 - 仓库管理）。

预期步骤：
    1) POST /warehouses 创建（admin）
    2) GET /warehouses 列表
    3) PUT /warehouses/:id 修改
    4) POST /warehouses/:id/users 绑定用户
    5) DELETE /warehouses/:id/users/:userId 解绑
    6) DELETE /warehouses/:id 删除
"""

from __future__ import annotations

from core.client import APIClient
from flows._placeholder import not_implemented


# TODO: 实现仓库 CRUD 与用户绑定
def run(client: APIClient) -> None:
    not_implemented("warehouse.warehouse_crud")
