"""盘点单流程（PRD 第 7 章 - 盘点，三阶段状态机）。

预期步骤：
    1) POST /documents 创建 doc_type=stocktake 进入 recording 状态
    2) POST /documents/:id/confirm 确认盘点（录入实际数量）→ confirmed
    3) POST /documents/:id/settle 结算差异（admin）→ settled
    4) GET /transactions 校验差异流水写入
    5) 非 admin 调 settle 应 403
"""

from __future__ import annotations

from core.client import APIClient
from flows._placeholder import not_implemented


# TODO: 实现盘点三步流程 + 权限校验
def run(client: APIClient) -> None:
    not_implemented("document.stocktake")
