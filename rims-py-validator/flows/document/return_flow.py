"""退货单流程（PRD 第 7 章 - 退货）。

预期步骤：
    1) 前置：已完成一张销售单
    2) POST /documents 创建 doc_type=return 关联原销售单
    3) POST /documents/:id/complete 完成退货
    4) GET /inventory 验证库存回补
    5) GET /transactions 验证负向流水
"""

from __future__ import annotations

from core.client import APIClient
from flows._placeholder import not_implemented


# TODO: 实现退货单全流程
def run(client: APIClient) -> None:
    not_implemented("document.return_flow")
