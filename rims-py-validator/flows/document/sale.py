"""销售单流程（PRD 第 7 章 - 销售）。

预期步骤：
    1) 前置：确保有库存（否则先跑 inbound）
    2) POST /documents 创建 doc_type=sale 单据，带实际售价
    3) POST /documents/:id/complete 完成销售
    4) GET /inventory 验证库存减少
    5) GET /reports/sales/stats 验证销售金额包含本次
"""

from __future__ import annotations

from core.client import APIClient
from flows._placeholder import not_implemented


# TODO: 实现销售单完整流程 + 报表联动校验
def run(client: APIClient) -> None:
    not_implemented("document.sale")
