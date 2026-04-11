"""仓库切换流程（PRD 第 4 章 - 多仓库切换）。

预期步骤：
    1) GET /users/me/warehouses 查看可访问仓库
    2) PUT /users/me/warehouses/default 设置默认仓库
    3) PUT /users/me/warehouses/current 切换当前仓库
    4) 切换后使用新 X-Warehouse-ID 调用 /inventory 验证生效
"""

from __future__ import annotations

from core.client import APIClient
from flows._placeholder import not_implemented


# TODO: 实现默认仓库设置与切换
def run(client: APIClient) -> None:
    not_implemented("warehouse.switch_warehouse")
