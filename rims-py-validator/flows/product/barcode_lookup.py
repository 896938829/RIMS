"""条码查询流程（PRD 第 5 章 - 条码扫描接入）。

预期步骤：
    1) POST /products 创建一个带条码的商品
    2) GET /products/barcode/:barcode 查询，断言返回同一条记录
    3) 查询不存在的条码应返回 404/业务错误
"""

from __future__ import annotations

from core.client import APIClient
from flows._placeholder import not_implemented


# TODO: 实现条码查询正/反向用例
def run(client: APIClient) -> None:
    not_implemented("product.barcode_lookup")
