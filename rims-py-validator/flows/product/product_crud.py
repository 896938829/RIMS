"""商品 CRUD 流程（PRD 第 5 章 - 商品档案管理）。

预期步骤：
    1) POST /products 创建商品（普通用户可创建，但成本价字段 admin-only）
    2) GET /products 列表分页
    3) GET /products/:id 详情
    4) PUT /products/:id 修改
    5) DELETE /products/:id 软删
    6) 非 admin 读取 costPrice 应被后端置空
"""

from __future__ import annotations

from core.client import APIClient
from flows._placeholder import not_implemented


# TODO: 实现商品 CRUD 与 admin 字段屏蔽校验
def run(client: APIClient) -> None:
    not_implemented("product.product_crud")
