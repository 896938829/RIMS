"""销售统计/趋势/排行流程（PRD 第 8 章 - 销售分析）。

预期步骤：
    1) GET /reports/sales/stats 指定时间范围
    2) GET /reports/sales/trend 按 day/week/month 分桶
    3) GET /reports/sales/ranking 取 topN
    4) 非 admin 读取响应中不应包含 cost/profit 字段
    5) 超过 366 天范围应返回业务错误
"""

from __future__ import annotations

from core.client import APIClient
from flows._placeholder import not_implemented


# TODO: 实现销售三张报表 + admin 字段屏蔽校验
def run(client: APIClient) -> None:
    not_implemented("report.sales_stats")
