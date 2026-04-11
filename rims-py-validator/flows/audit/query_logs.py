"""审计日志查询流程（PRD 第 10 章 - 操作审计，admin-only）。

预期步骤：
    1) 前置：触发一次登录/单据完成，确保有新的审计记录
    2) GET /audit/logs 按 resource/action/time 过滤
    3) GET /audit/logs/:id 校验 before/after 快照字段存在
    4) 非 admin 调用应 403
    5) 时间窗口 > 366 天应返回业务错误
"""

from __future__ import annotations

from core.client import APIClient
from flows._placeholder import not_implemented


# TODO: 实现审计日志列表 + 详情 + 权限校验
def run(client: APIClient) -> None:
    not_implemented("audit.query_logs")
