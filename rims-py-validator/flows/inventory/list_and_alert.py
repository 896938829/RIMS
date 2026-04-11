"""库存列表与低库存告警流程（PRD 第 6 章 - 库存管理）。

预期步骤：
    1) GET /inventory 分页列出当前仓库库存
    2) PUT /inventory/:id 设置告警阈值
    3) GET /inventory/alerts 列出低于阈值的商品，断言包含上一步的商品
    4) 换一个仓库头（X-Warehouse-ID）验证仓库隔离
"""

from __future__ import annotations

from core.client import APIClient
from flows._placeholder import not_implemented


# TODO: 实现库存列表 + 告警阈值 + 仓库隔离校验
def run(client: APIClient) -> None:
    not_implemented("inventory.list_and_alert")
