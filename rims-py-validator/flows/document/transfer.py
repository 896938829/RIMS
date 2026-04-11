"""调拨/转仓单流程（PRD 第 7 章 - 跨仓调拨，admin-only）。

预期步骤：
    1) 前置：源仓库有库存
    2) POST /documents 创建 doc_type=transfer 指定目标仓库
    3) POST /documents/:id/complete 完成调拨
    4) 切换 X-Warehouse-ID 到目标仓库，GET /inventory 验证收货
    5) 切回源仓库验证出库
"""

from __future__ import annotations

from core.client import APIClient
from flows._placeholder import not_implemented


# TODO: 实现跨仓调拨 + 双仓校验
def run(client: APIClient) -> None:
    not_implemented("document.transfer")
