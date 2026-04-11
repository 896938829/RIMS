"""库存总览/周转/滞销流程（PRD 第 8 章 - 库存分析）。

预期步骤：
    1) GET /reports/inventory/overview 当前库存快照
    2) GET /reports/inventory/turnover 周转率
    3) GET /reports/inventory/slow-moving 滞销预警
    4) 校验 stockValue 字段仅对 admin 返回
"""

from __future__ import annotations

from core.client import APIClient
from flows._placeholder import not_implemented


# TODO: 实现库存三张报表
def run(client: APIClient) -> None:
    not_implemented("report.inventory_overview")
