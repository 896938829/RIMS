"""入库单流程（PRD 第 7 章 - 入库）。

预期步骤：
    1) POST /documents 创建 doc_type=inbound 的单据 + 明细行
    2) GET /documents/:id 确认状态为 draft
    3) POST /documents/:id/complete 提交入库，后端事务性地写 inventory_transactions
    4) GET /inventory 验证库存数量增加
    5) GET /transactions 验证流水记录
"""

from __future__ import annotations

from core.client import APIClient
from flows._placeholder import not_implemented


# TODO: 实现入库单创建 → 完成 → 库存校验
def run(client: APIClient) -> None:
    not_implemented("document.inbound")
